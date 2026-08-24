package streaming

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

type State string

const (
	StateStarting     State = "starting"
	StateRunning      State = "running"
	StateReconnecting State = "reconnecting"
	StateFailed       State = "failed"
	StateStopping     State = "stopping"
	StateStopped      State = "stopped"
)

type Config struct {
	IngestBuffer      time.Duration
	StartupTimeout    time.Duration
	StallTimeout      time.Duration
	IdleTimeout       time.Duration
	RetryLimit        int
	RetryBackoff      time.Duration
	HLSSegmentSeconds int
	HLSPlaylistLength int
	ClientBufferBytes int
	TLSVerify         bool
	UserAgent         string
	StreamRoot        string
}

func CurrentConfig() Config {
	configured := settings.APP_SETTINGS.Streaming
	return Config{
		IngestBuffer:      time.Duration(configured.IngestBufferMS) * time.Millisecond,
		StartupTimeout:    time.Duration(configured.StartupTimeoutSeconds) * time.Second,
		StallTimeout:      time.Duration(configured.StallTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(configured.IdleTimeoutSeconds) * time.Second,
		RetryLimit:        configured.RetryLimit,
		RetryBackoff:      time.Duration(configured.RetryBackoffMS) * time.Millisecond,
		HLSSegmentSeconds: configured.HLSSegmentSeconds,
		HLSPlaylistLength: configured.HLSPlaylistLength,
		ClientBufferBytes: configured.ClientBufferMB * 1024 * 1024,
		TLSVerify:         configured.TLSVerify,
		UserAgent:         configured.UserAgent,
		StreamRoot:        settings.STREAM_FILEPATH,
	}.normalized()
}

func (c Config) normalized() Config {
	if c.IngestBuffer < 0 {
		c.IngestBuffer = 0
	}
	if c.StartupTimeout < 3*time.Second {
		c.StartupTimeout = 12 * time.Second
	}
	if c.StallTimeout < 3*time.Second {
		c.StallTimeout = 10 * time.Second
	}
	if c.IdleTimeout < 10*time.Second {
		c.IdleTimeout = 45 * time.Second
	}
	if c.RetryLimit < 1 {
		c.RetryLimit = 6
	}
	if c.RetryBackoff < 100*time.Millisecond {
		c.RetryBackoff = 500 * time.Millisecond
	}
	if c.HLSSegmentSeconds < 1 {
		c.HLSSegmentSeconds = 2
	}
	if c.HLSPlaylistLength < 3 {
		c.HLSPlaylistLength = 6
	}
	if c.ClientBufferBytes < 1024*1024 {
		c.ClientBufferBytes = 2 * 1024 * 1024
	}
	if c.UserAgent == "" {
		c.UserAgent = "Xivi 1.0"
	}
	if c.StreamRoot == "" {
		c.StreamRoot = "./serve/stream"
	}
	return c
}

type Producer interface {
	Start() error
	Ready() <-chan struct{}
	HLSReady() <-chan struct{}
	Errors() <-chan error
	Stop()
	LastDataAt() time.Time
}

type ProducerFactory func(id, source string, generation uint64, config Config, hub *Hub) Producer

type Manager struct {
	mu             sync.RWMutex
	sessions       map[string]*Session
	factory        ProducerFactory
	config         func() Config
	cleanupStop    chan struct{}
	cleanupDone    chan struct{}
	closeOnce      sync.Once
	totalReconnect uint64
	completedBytes uint64
	completedDrops uint64
}

type Session struct {
	id          string
	mu          sync.RWMutex
	sources     []string
	config      Config
	factory     ProducerFactory
	hub         *Hub
	state       State
	lastError   string
	lastAccess  time.Time
	startedAt   time.Time
	current     Producer
	sourceIndex int
	generation  uint64
	reconnects  uint64
	everReady   bool
	initialErr  error
	initialDone chan struct{}
	initialOnce sync.Once
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	stopOnce    sync.Once
	onReconnect func()
	onDone      func(*Session)
}

type SessionSnapshot struct {
	ID              string    `json:"id"`
	State           State     `json:"state"`
	SourcePosition  int       `json:"source_position"`
	SourceCount     int       `json:"source_count"`
	Clients         int       `json:"clients"`
	Reconnects      uint64    `json:"reconnects"`
	BytesPublished  uint64    `json:"bytes_published"`
	SlowClientDrops uint64    `json:"slow_client_drops"`
	LastError       string    `json:"last_error,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	LastAccess      time.Time `json:"last_access"`
	LastMediaAt     time.Time `json:"last_media_at,omitempty"`
}

type Summary struct {
	Sessions        int    `json:"sessions"`
	Clients         int    `json:"clients"`
	Reconnects      uint64 `json:"reconnects"`
	BytesPublished  uint64 `json:"bytes_published"`
	SlowClientDrops uint64 `json:"slow_client_drops"`
}

var validSessionID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

var DefaultManager = NewManager(nil, nil)

func NewManager(factory ProducerFactory, configProvider func() Config) *Manager {
	if factory == nil {
		factory = newGSTProducer
	}
	if configProvider == nil {
		configProvider = CurrentConfig
	}
	manager := &Manager{
		sessions:    make(map[string]*Session),
		factory:     factory,
		config:      configProvider,
		cleanupStop: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}
	go manager.cleanupLoop()
	return manager
}

func (m *Manager) Acquire(ctx context.Context, id string, sources []string) (*Session, error) {
	cleanSources, err := validateSources(id, sources)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if existing := m.sessions[id]; existing != nil {
		existing.touch()
		existing.updateSources(cleanSources)
		m.mu.Unlock()
		if err := existing.WaitReady(ctx); err != nil {
			return nil, err
		}
		return existing, nil
	}

	config := m.config().normalized()
	sessionContext, cancel := context.WithCancel(context.Background())
	session := &Session{
		id:          id,
		sources:     cleanSources,
		config:      config,
		factory:     m.factory,
		hub:         NewHub(config.ClientBufferBytes),
		state:       StateStarting,
		lastAccess:  time.Now(),
		startedAt:   time.Now(),
		sourceIndex: -1,
		initialDone: make(chan struct{}),
		ctx:         sessionContext,
		cancel:      cancel,
		done:        make(chan struct{}),
		onReconnect: func() {
			m.mu.Lock()
			m.totalReconnect++
			m.mu.Unlock()
		},
	}
	session.onDone = func(completed *Session) {
		bytes, drops := completed.hub.Metrics()
		m.mu.Lock()
		if m.sessions[id] == completed {
			m.completedBytes += bytes
			m.completedDrops += drops
			delete(m.sessions, id)
		}
		m.mu.Unlock()
	}
	m.sessions[id] = session
	m.mu.Unlock()
	go session.run()

	if err := session.WaitReady(ctx); err != nil {
		return nil, err
	}
	return session, nil
}

func validateSources(id string, sources []string) ([]string, error) {
	if !validSessionID.MatchString(id) {
		return nil, fmt.Errorf("invalid stream id")
	}
	clean := make([]string, 0, len(sources))
	seen := make(map[string]bool)
	for _, source := range sources {
		parsed, err := url.Parse(source)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			continue
		}
		if !seen[source] {
			seen[source] = true
			clean = append(clean, source)
		}
	}
	if len(clean) == 0 {
		return nil, errors.New("no valid HTTP source variants are available")
	}
	return clean, nil
}

func (s *Session) run() {
	defer close(s.done)
	defer func() {
		if s.onDone != nil {
			s.onDone(s)
		}
	}()

	attempt := 0
	nextSource := 0
	for {
		select {
		case <-s.ctx.Done():
			s.finish(StateStopped, nil)
			return
		default:
		}

		sources := s.sourceList()
		if len(sources) == 0 {
			s.failInitial(errors.New("no source variants are available"))
			return
		}
		if nextSource >= len(sources) {
			nextSource = 0
		}
		if attempt >= s.config.RetryLimit {
			err := fmt.Errorf("all ordered source variants failed after %d attempts", attempt)
			if !s.isReady() {
				s.failInitial(err)
				return
			}
			s.setState(StateFailed, err)
			if !s.wait(backoff(s.config.RetryBackoff, attempt)) {
				s.finish(StateStopped, nil)
				return
			}
			attempt = 0
		}

		s.mu.Lock()
		s.generation++
		generation := s.generation
		s.sourceIndex = nextSource
		if s.everReady {
			s.state = StateReconnecting
			s.reconnects++
		} else {
			s.state = StateStarting
		}
		shouldReportReconnect := s.everReady
		s.mu.Unlock()
		if shouldReportReconnect && s.onReconnect != nil {
			s.onReconnect()
		}

		s.hub.ResetWarmBuffer()
		producer := s.factory(s.id, sources[nextSource], generation, s.config, s.hub)
		s.setProducer(producer)
		err := producer.Start()
		becameReady := false
		if err == nil {
			becameReady, err = s.waitForProducer(producer)
		}
		producer.Stop()
		s.clearProducer(producer)
		if errors.Is(err, context.Canceled) || s.ctx.Err() != nil {
			s.finish(StateStopped, nil)
			return
		}

		if becameReady {
			attempt = 0
		}
		attempt++
		nextSource = (nextSource + 1) % len(sources)
		s.setState(StateReconnecting, err)
		log.Warn().Err(err).Str("stream_id", s.id).Int("source_position", s.sourcePosition()).Int("attempt", attempt).Msg("Streaming source failed; trying the next ordered variant")
		if !s.wait(backoff(s.config.RetryBackoff, attempt-1)) {
			s.finish(StateStopped, nil)
			return
		}
	}
}

func (s *Session) waitForProducer(producer Producer) (bool, error) {
	timer := time.NewTimer(s.config.StartupTimeout)
	defer timer.Stop()
	select {
	case <-producer.Ready():
		s.markReady()
	case err := <-producer.Errors():
		return false, err
	case <-timer.C:
		return false, fmt.Errorf("source produced no valid MPEG-TS programme within %s", s.config.StartupTimeout)
	case <-s.ctx.Done():
		return false, context.Canceled
	}

	s.setState(StateRunning, nil)
	select {
	case err := <-producer.Errors():
		return true, err
	case <-s.ctx.Done():
		return true, context.Canceled
	}
}

func backoff(base time.Duration, exponent int) time.Duration {
	if exponent > 6 {
		exponent = 6
	}
	delay := base * time.Duration(1<<exponent)
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	jitter := time.Duration(rand.Int64N(max(int64(delay/4), 1)))
	return delay + jitter
}

func (s *Session) wait(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *Session) markReady() {
	s.mu.Lock()
	s.everReady = true
	s.lastError = ""
	s.mu.Unlock()
	s.initialOnce.Do(func() { close(s.initialDone) })
}

func (s *Session) failInitial(err error) {
	s.mu.Lock()
	s.initialErr = err
	s.lastError = err.Error()
	s.state = StateFailed
	s.mu.Unlock()
	s.initialOnce.Do(func() { close(s.initialDone) })
	s.hub.Close(err)
	_ = os.RemoveAll(filepath.Join(s.config.StreamRoot, s.id))
}

func (s *Session) finish(state State, err error) {
	s.setState(state, err)
	s.initialOnce.Do(func() {
		s.mu.Lock()
		if s.initialErr == nil {
			s.initialErr = context.Canceled
		}
		s.mu.Unlock()
		close(s.initialDone)
	})
	s.hub.Close(err)
	_ = os.RemoveAll(filepath.Join(s.config.StreamRoot, s.id))
}

func (s *Session) setState(state State, err error) {
	s.mu.Lock()
	s.state = state
	if err != nil {
		s.lastError = err.Error()
	} else if state == StateRunning {
		s.lastError = ""
	}
	s.mu.Unlock()
}

func (s *Session) setProducer(producer Producer) {
	s.mu.Lock()
	s.current = producer
	s.mu.Unlock()
}

func (s *Session) clearProducer(producer Producer) {
	s.mu.Lock()
	if s.current == producer {
		s.current = nil
	}
	s.mu.Unlock()
}

func (s *Session) sourceList() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.sources...)
}

func (s *Session) updateSources(sources []string) {
	s.mu.Lock()
	s.sources = append([]string(nil), sources...)
	s.mu.Unlock()
}

func (s *Session) sourcePosition() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sourceIndex + 1
}

func (s *Session) isReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.everReady
}

func (s *Session) WaitReady(ctx context.Context) error {
	select {
	case <-s.initialDone:
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.initialErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Session) WaitHLS(ctx context.Context) error {
	for {
		s.mu.RLock()
		producer := s.current
		s.mu.RUnlock()
		if producer == nil {
			select {
			case <-time.After(50 * time.Millisecond):
				continue
			case <-ctx.Done():
				return ctx.Err()
			case <-s.done:
				return errors.New("stream session stopped before HLS became ready")
			}
		}
		select {
		case <-producer.HLSReady():
			s.touch()
			return nil
		case <-time.After(100 * time.Millisecond):
			// Re-read the current producer so the supervisor remains the sole
			// consumer of its failure channel during ordered failover.
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return errors.New("stream session stopped before HLS became ready")
		}
	}
}

func (s *Session) Subscribe() *Subscription {
	s.touch()
	return s.hub.Subscribe()
}

func (s *Session) touch() {
	s.mu.Lock()
	s.lastAccess = time.Now()
	s.mu.Unlock()
}

func (s *Session) Stop() {
	s.stopOnce.Do(func() {
		s.setState(StateStopping, nil)
		s.cancel()
		s.mu.RLock()
		producer := s.current
		s.mu.RUnlock()
		if producer != nil {
			producer.Stop()
		}
	})
}

func (s *Session) Snapshot() SessionSnapshot {
	s.mu.RLock()
	producer := s.current
	snapshot := SessionSnapshot{
		ID:             s.id,
		State:          s.state,
		SourcePosition: s.sourceIndex + 1,
		SourceCount:    len(s.sources),
		Clients:        s.hub.SubscriberCount(),
		Reconnects:     s.reconnects,
		LastError:      s.lastError,
		StartedAt:      s.startedAt,
		LastAccess:     s.lastAccess,
	}
	s.mu.RUnlock()
	if producer != nil {
		snapshot.LastMediaAt = producer.LastDataAt()
	}
	snapshot.BytesPublished, snapshot.SlowClientDrops = s.hub.Metrics()
	return snapshot
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[id]
	return session, ok
}

func (m *Manager) Touch(id string) bool {
	session, ok := m.Get(id)
	if ok {
		session.touch()
	}
	return ok
}

func (m *Manager) Stop(id string) {
	if session, ok := m.Get(id); ok {
		session.Stop()
	}
}

func (m *Manager) Snapshots() []SessionSnapshot {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()
	items := make([]SessionSnapshot, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, session.Snapshot())
	}
	return items
}

func (m *Manager) Summary() Summary {
	items := m.Snapshots()
	m.mu.RLock()
	summary := Summary{
		Sessions:        len(items),
		Reconnects:      m.totalReconnect,
		BytesPublished:  m.completedBytes,
		SlowClientDrops: m.completedDrops,
	}
	m.mu.RUnlock()
	for _, item := range items {
		summary.Clients += item.Clients
		summary.BytesPublished += item.BytesPublished
		summary.SlowClientDrops += item.SlowClientDrops
	}
	return summary
}

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	defer close(m.cleanupDone)
	for {
		select {
		case now := <-ticker.C:
			for _, snapshot := range m.Snapshots() {
				if snapshot.Clients == 0 {
					session, ok := m.Get(snapshot.ID)
					if ok && now.Sub(snapshot.LastAccess) > session.config.IdleTimeout {
						log.Info().Str("stream_id", snapshot.ID).Dur("idle", now.Sub(snapshot.LastAccess)).Msg("Stopping idle shared stream")
						session.Stop()
					}
				}
			}
		case <-m.cleanupStop:
			return
		}
	}
}

func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		close(m.cleanupStop)
		m.mu.RLock()
		sessions := make([]*Session, 0, len(m.sessions))
		for _, session := range m.sessions {
			sessions = append(sessions, session)
		}
		m.mu.RUnlock()
		for _, session := range sessions {
			session.Stop()
		}
		for _, session := range sessions {
			<-session.done
		}
		<-m.cleanupDone
	})
}
