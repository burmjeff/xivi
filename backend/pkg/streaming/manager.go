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
	"sort"
	"strings"
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
		c.HLSPlaylistLength = 10
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

type ProducerInspector interface {
	MediaTracks() []string
}

type Manager struct {
	mu                sync.RWMutex
	sessions          map[string]*Session
	factory           ProducerFactory
	config            func() Config
	cleanupStop       chan struct{}
	cleanupDone       chan struct{}
	closeOnce         sync.Once
	totalReconnect    uint64
	completedBytes    uint64
	completedDrops    uint64
	completedDelivery uint64
	observer          Observer
}

type Session struct {
	id                 string
	incidentID         string
	mu                 sync.RWMutex
	sources            []string
	config             Config
	factory            ProducerFactory
	hub                *Hub
	state              State
	lastError          string
	lastAccess         time.Time
	startedAt          time.Time
	current            Producer
	sourceIndex        int
	generation         uint64
	reconnects         uint64
	everReady          bool
	initialErr         error
	initialDone        chan struct{}
	initialOnce        sync.Once
	ctx                context.Context
	cancel             context.CancelFunc
	done               chan struct{}
	stopOnce           sync.Once
	forceNext          chan error
	clients            map[string]*sessionClient
	samples            []MetricSample
	lastSampleAt       time.Time
	lastSampleIn       uint64
	lastSampleOut      uint64
	retiredClientBytes uint64
	firstMediaAt       *time.Time
	restartSource      bool
	observer           func(Observation)
	onReconnect        func()
	onDone             func(*Session)
}

type sessionClient struct {
	metadata        ClientMetadata
	startedAt       time.Time
	lastSeenAt      time.Time
	bytesDelivered  uint64
	lastSampleBytes uint64
	bitrateBPS      uint64
	endReason       string
	cancel          func()
}

type SessionSnapshot struct {
	ID                string           `json:"id"`
	IncidentID        string           `json:"incident_id"`
	State             State            `json:"state"`
	SourcePosition    int              `json:"source_position"`
	SourceCount       int              `json:"source_count"`
	Clients           int              `json:"clients"`
	Reconnects        uint64           `json:"reconnects"`
	BytesPublished    uint64           `json:"bytes_published"`
	SlowClientDrops   uint64           `json:"slow_client_drops"`
	LastError         string           `json:"last_error,omitempty"`
	StartedAt         time.Time        `json:"started_at"`
	LastAccess        time.Time        `json:"last_access"`
	LastMediaAt       time.Time        `json:"last_media_at,omitempty"`
	BytesDelivered    uint64           `json:"bytes_delivered"`
	IngressBitrateBPS uint64           `json:"ingress_bitrate_bps"`
	EgressBitrateBPS  uint64           `json:"egress_bitrate_bps"`
	ClientDetails     []ClientSnapshot `json:"connections"`
	Samples           []MetricSample   `json:"samples"`
	MediaTracks       []string         `json:"media_tracks,omitempty"`
}

type Summary struct {
	Sessions          int    `json:"sessions"`
	Clients           int    `json:"clients"`
	Reconnects        uint64 `json:"reconnects"`
	BytesPublished    uint64 `json:"bytes_published"`
	SlowClientDrops   uint64 `json:"slow_client_drops"`
	BytesDelivered    uint64 `json:"bytes_delivered"`
	IngressBitrateBPS uint64 `json:"ingress_bitrate_bps"`
	EgressBitrateBPS  uint64 `json:"egress_bitrate_bps"`
}

type FileCleanupReport struct {
	DirectoriesRemoved int
	BytesReclaimed     int64
}

const (
	endedClientRetention   = 5 * time.Minute
	maximumEndedClientRows = 1000
)

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

func (m *Manager) SetObserver(observer Observer) {
	m.mu.Lock()
	m.observer = observer
	m.mu.Unlock()
}

func (m *Manager) observe(observation Observation) {
	m.mu.RLock()
	observer := m.observer
	m.mu.RUnlock()
	if observer != nil {
		observer.Observe(observation)
	}
}

func (m *Manager) Acquire(ctx context.Context, id string, sources []string) (*Session, error) {
	cleanSources, err := validateSources(id, sources)
	if err != nil {
		return nil, err
	}
	// Fiber/fasthttp exposes zero-copy parameter strings. A stream outlives the
	// request that created it, so retaining that storage would let later
	// requests mutate map keys and diagnostic IDs underneath the manager.
	id = strings.Clone(id)

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
		id:           id,
		incidentID:   randomIdentifier("str_"),
		sources:      cleanSources,
		config:       config,
		factory:      m.factory,
		hub:          NewHub(config.ClientBufferBytes),
		state:        StateStarting,
		lastAccess:   time.Now(),
		startedAt:    time.Now(),
		sourceIndex:  -1,
		initialDone:  make(chan struct{}),
		ctx:          sessionContext,
		cancel:       cancel,
		done:         make(chan struct{}),
		forceNext:    make(chan error, 1),
		clients:      make(map[string]*sessionClient),
		lastSampleAt: time.Now(),
		observer:     m.observe,
		onReconnect: func() {
			m.mu.Lock()
			m.totalReconnect++
			m.mu.Unlock()
		},
	}
	session.onDone = func(completed *Session) {
		completed.observeCompleted()
		bytes, drops := completed.hub.Metrics()
		delivered := completed.Snapshot().BytesDelivered
		m.mu.Lock()
		if m.sessions[id] == completed {
			m.completedBytes += bytes
			m.completedDrops += drops
			m.completedDelivery += delivered
			delete(m.sessions, id)
		}
		m.mu.Unlock()
	}
	m.sessions[id] = session
	m.mu.Unlock()
	session.observeSession(nil, "")
	session.emitEvent("info", "session_started", "Shared stream session started.", "")
	go session.run()

	if err := session.WaitReady(ctx); err != nil {
		return session, err
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
		s.emitEvent("info", "source_connecting", fmt.Sprintf("Connecting to ordered source %d of %d.", nextSource+1, len(sources)), "")
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
		s.mu.Lock()
		restartSource := s.restartSource
		s.restartSource = false
		s.mu.Unlock()
		if !restartSource {
			nextSource = (nextSource + 1) % len(sources)
		}
		s.setState(StateReconnecting, err)
		code, _ := ClassifyError(err)
		severity := "error"
		if code == "manual_failover" {
			severity = "warning"
		}
		s.emitEvent(severity, code, err.Error(), "")
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
	case err := <-s.forceNext:
		return false, err
	}

	s.setState(StateRunning, nil)
	s.emitEvent("info", "media_ready", "Valid media is flowing from the selected source.", "")
	s.observeSession(nil, "")
	select {
	case err := <-producer.Errors():
		return true, err
	case <-s.ctx.Done():
		return true, context.Canceled
	case err := <-s.forceNext:
		return true, err
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
	s.lastError = SanitizeDiagnostic(err.Error())
	s.state = StateFailed
	s.mu.Unlock()
	code, _ := ClassifyError(err)
	s.emitEvent("error", code, err.Error(), "")
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
		s.lastError = SanitizeDiagnostic(err.Error())
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

func (s *Session) IncidentID() string { return s.incidentID }

func (s *Session) RegisterClient(metadata ClientMetadata, cancel func()) (string, bool) {
	metadata.ID = strings.Clone(metadata.ID)
	metadata.Protocol = strings.Clone(metadata.Protocol)
	metadata.RemoteIP = strings.Clone(metadata.RemoteIP)
	metadata.Method = strings.Clone(metadata.Method)
	metadata.UserAgent = strings.Clone(metadata.UserAgent)
	now := time.Now()
	s.mu.Lock()
	if metadata.ID != "" {
		if existing := s.clients[metadata.ID]; existing != nil {
			if existing.endReason != "" {
				if existing.endReason == "terminated_by_user" {
					s.mu.Unlock()
					return metadata.ID, false
				}
				existing.endReason = ""
				existing.lastSeenAt = now
				existing.cancel = cancel
				s.lastAccess = now
				s.mu.Unlock()
				s.observeConnection(existing, nil)
				s.emitEventForClient(metadata.ID, "info", "viewer_reconnected", fmt.Sprintf("%s viewer resumed.", strings.ToUpper(metadata.Protocol)), "")
				return metadata.ID, true
			}
			existing.lastSeenAt = now
			if cancel != nil {
				existing.cancel = cancel
			}
			s.lastAccess = now
			s.mu.Unlock()
			return metadata.ID, true
		}
	}
	if metadata.ID == "" {
		metadata.ID = randomIdentifier("con_")
	}
	client := &sessionClient{metadata: metadata, startedAt: now, lastSeenAt: now, cancel: cancel}
	s.clients[metadata.ID] = client
	s.lastAccess = now
	s.mu.Unlock()
	s.observeConnection(client, nil)
	s.emitEventForClient(metadata.ID, "info", "viewer_connected", fmt.Sprintf("%s viewer connected.", strings.ToUpper(metadata.Protocol)), "")
	return metadata.ID, true
}

func (s *Session) AddClientBytes(id string, bytes int) bool {
	if bytes <= 0 {
		return true
	}
	s.mu.Lock()
	client := s.clients[id]
	if client == nil || client.endReason != "" {
		s.mu.Unlock()
		return false
	}
	client.bytesDelivered += uint64(bytes)
	client.lastSeenAt = time.Now()
	s.lastAccess = client.lastSeenAt
	s.mu.Unlock()
	return true
}

func (s *Session) CloseClient(id, reason string) {
	s.mu.Lock()
	client := s.clients[id]
	if client == nil || client.endReason != "" {
		s.mu.Unlock()
		return
	}
	client.endReason = reason
	client.lastSeenAt = time.Now()
	s.mu.Unlock()
	ended := client.lastSeenAt
	s.observeConnection(client, &ended)
	s.emitEventForClient(id, "info", "viewer_disconnected", "Viewer disconnected: "+reason+".", "")
}

func (s *Session) DisconnectClient(id string) bool {
	s.mu.Lock()
	client := s.clients[id]
	if client == nil || client.endReason != "" {
		s.mu.Unlock()
		return false
	}
	cancel := client.cancel
	s.mu.Unlock()
	s.CloseClient(id, "terminated_by_user")
	if cancel != nil {
		cancel()
	}
	return true
}

func (s *Session) ForceNextSource() bool {
	err := errors.New("manual source failover requested")
	select {
	case s.forceNext <- err:
		s.emitEvent("warning", "manual_failover", "A user requested the next ordered source.", "")
		return true
	default:
		return false
	}
}

func (s *Session) RestartSource() bool {
	s.mu.Lock()
	s.restartSource = true
	s.mu.Unlock()
	err := errors.New("manual source restart requested")
	select {
	case s.forceNext <- err:
		s.emitEvent("warning", "manual_restart", "A user requested a restart of the active source.", "")
		return true
	default:
		s.mu.Lock()
		s.restartSource = false
		s.mu.Unlock()
		return false
	}
}

func (s *Session) RecordClientEvent(clientID, severity, code, message, details string) {
	if severity != "error" && severity != "warning" {
		severity = "info"
	}
	if code == "" {
		code = "client_event"
	}
	if message == "" {
		message = "The player reported a stream event."
	}
	s.emitEventForClient(clientID, severity, code, message, details)
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

func (s *Session) sample(now time.Time) {
	bytesIn, _ := s.hub.Metrics()
	timedOut := []ConnectionRecord{}
	s.mu.Lock()
	if elapsed := now.Sub(s.lastSampleAt); elapsed > 0 {
		type endedClient struct {
			id       string
			lastSeen time.Time
		}
		endedClients := make([]endedClient, 0)
		for id, client := range s.clients {
			if client.endReason != "" {
				endedClients = append(endedClients, endedClient{id: id, lastSeen: client.lastSeenAt})
			}
		}
		sort.Slice(endedClients, func(i, j int) bool {
			return endedClients[i].lastSeen.Before(endedClients[j].lastSeen)
		})
		mustRemove := len(endedClients) - maximumEndedClientRows
		for _, ended := range endedClients {
			if mustRemove <= 0 && now.Sub(ended.lastSeen) <= endedClientRetention {
				continue
			}
			client := s.clients[ended.id]
			if client == nil || client.endReason == "" {
				continue
			}
			s.retiredClientBytes += client.bytesDelivered
			delete(s.clients, ended.id)
			if mustRemove > 0 {
				mustRemove--
			}
		}

		bytesOut := s.retiredClientBytes
		active := 0
		for _, client := range s.clients {
			if client.endReason != "" {
				bytesOut += client.bytesDelivered
				continue
			}
			if client.metadata.Protocol == "hls" && now.Sub(client.lastSeenAt) > 35*time.Second {
				client.endReason = "viewer_timeout"
				endedAt := now
				timedOut = append(timedOut, ConnectionRecord{ID: client.metadata.ID, IncidentID: s.incidentID,
					StreamID: s.id, Protocol: client.metadata.Protocol, RemoteIP: client.metadata.RemoteIP,
					Method: client.metadata.Method, UserAgent: client.metadata.UserAgent, StartedAt: client.startedAt,
					LastSeenAt: client.lastSeenAt, EndedAt: &endedAt, BytesDelivered: client.bytesDelivered,
					EndReason: client.endReason})
				continue
			}
			active++
			bytesOut += client.bytesDelivered
			client.bitrateBPS = uint64(float64((client.bytesDelivered-client.lastSampleBytes)*8) / elapsed.Seconds())
			client.lastSampleBytes = client.bytesDelivered
		}
		inRate := uint64(float64((bytesIn-s.lastSampleIn)*8) / elapsed.Seconds())
		outRate := uint64(float64((bytesOut-s.lastSampleOut)*8) / elapsed.Seconds())
		s.samples = append(s.samples, MetricSample{At: now, IngressBPS: inRate, EgressBPS: outRate, ActiveClients: active})
		if len(s.samples) > 900 {
			s.samples = append([]MetricSample(nil), s.samples[len(s.samples)-900:]...)
		}
		s.lastSampleAt, s.lastSampleIn, s.lastSampleOut = now, bytesIn, bytesOut
	}
	producer := s.current
	if producer != nil && s.firstMediaAt == nil {
		last := producer.LastDataAt()
		if !last.IsZero() {
			s.firstMediaAt = &last
		}
	}
	s.mu.Unlock()
	if s.observer != nil {
		for index := range timedOut {
			record := timedOut[index]
			s.observer(Observation{Connection: &record})
		}
	}
}

func (s *Session) Snapshot() SessionSnapshot {
	s.mu.RLock()
	producer := s.current
	snapshot := SessionSnapshot{
		ID:             s.id,
		IncidentID:     s.incidentID,
		State:          s.state,
		SourcePosition: s.sourceIndex + 1,
		SourceCount:    len(s.sources),
		Clients:        0,
		Reconnects:     s.reconnects,
		LastError:      s.lastError,
		StartedAt:      s.startedAt,
		LastAccess:     s.lastAccess,
		ClientDetails:  []ClientSnapshot{},
		Samples:        []MetricSample{},
	}
	bytesOut := s.retiredClientBytes
	for _, client := range s.clients {
		bytesOut += client.bytesDelivered
		if client.endReason != "" {
			continue
		}
		snapshot.Clients++
		snapshot.ClientDetails = append(snapshot.ClientDetails, ClientSnapshot{
			ID: client.metadata.ID, Protocol: client.metadata.Protocol, RemoteIP: client.metadata.RemoteIP,
			Method: client.metadata.Method, UserAgent: client.metadata.UserAgent, StartedAt: client.startedAt,
			LastSeenAt: client.lastSeenAt, BytesDelivered: client.bytesDelivered, BitrateBPS: client.bitrateBPS,
		})
	}
	snapshot.BytesDelivered = bytesOut
	snapshot.Samples = append([]MetricSample(nil), s.samples...)
	if len(snapshot.Samples) > 0 {
		last := snapshot.Samples[len(snapshot.Samples)-1]
		snapshot.IngressBitrateBPS, snapshot.EgressBitrateBPS = last.IngressBPS, last.EgressBPS
	}
	s.mu.RUnlock()
	if producer != nil {
		snapshot.LastMediaAt = producer.LastDataAt()
		if inspector, ok := producer.(ProducerInspector); ok {
			snapshot.MediaTracks = inspector.MediaTracks()
		}
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

func (m *Manager) StopManual(id string) bool {
	if session, ok := m.Get(id); ok {
		session.emitEvent("warning", "manual_stop", "A user stopped the shared stream and its viewers.", "")
		session.Stop()
		return true
	}
	return false
}

func (m *Manager) ForceNextSource(id string) bool {
	session, ok := m.Get(id)
	return ok && session.ForceNextSource()
}

func (m *Manager) RestartSource(id string) bool {
	session, ok := m.Get(id)
	return ok && session.RestartSource()
}

func (m *Manager) DisconnectClient(id, clientID string) bool {
	session, ok := m.Get(id)
	return ok && session.DisconnectClient(clientID)
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
		BytesDelivered:  m.completedDelivery,
	}
	m.mu.RUnlock()
	for _, item := range items {
		summary.Clients += item.Clients
		summary.BytesPublished += item.BytesPublished
		summary.SlowClientDrops += item.SlowClientDrops
		summary.BytesDelivered += item.BytesDelivered
		summary.IngressBitrateBPS += item.IngressBitrateBPS
		summary.EgressBitrateBPS += item.EgressBitrateBPS
	}
	return summary
}

// PruneOrphanedStreamFiles removes HLS work directories left behind by a crash
// while holding the manager lock for each final active-session check. A stream
// cannot be acquired with the same ID during its directory removal.
func (m *Manager) PruneOrphanedStreamFiles() (FileCleanupReport, error) {
	report := FileCleanupReport{}
	root := m.config().StreamRoot
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return report, nil
	}
	if err != nil {
		return report, err
	}
	var cleanupErrors []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		m.mu.RLock()
		_, active := m.sessions[entry.Name()]
		m.mu.RUnlock()
		if active {
			continue
		}
		bytes, sizeErr := directoryBytes(path)
		if sizeErr != nil {
			cleanupErrors = append(cleanupErrors, sizeErr)
		}

		// Recheck under the write lock before removal. Acquire uses the same
		// lock, so a session cannot appear between this check and RemoveAll.
		m.mu.Lock()
		_, active = m.sessions[entry.Name()]
		if !active {
			if removeErr := os.RemoveAll(path); removeErr != nil {
				cleanupErrors = append(cleanupErrors, removeErr)
			} else {
				report.DirectoriesRemoved++
				report.BytesReclaimed += bytes
			}
		}
		m.mu.Unlock()
	}
	return report, errors.Join(cleanupErrors...)
}

func directoryBytes(root string) (int64, error) {
	var total int64
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer close(m.cleanupDone)
	for {
		select {
		case now := <-ticker.C:
			m.mu.RLock()
			sessions := make([]*Session, 0, len(m.sessions))
			for _, session := range m.sessions {
				sessions = append(sessions, session)
			}
			m.mu.RUnlock()
			for _, session := range sessions {
				session.sample(now)
			}
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

func (s *Session) emitEvent(severity, code, message, details string) {
	s.emitEventForClient("", severity, code, message, details)
}

func (s *Session) emitEventForClient(clientID, severity, code, message, details string) {
	if s.observer == nil {
		return
	}
	s.mu.RLock()
	position := s.sourceIndex + 1
	s.mu.RUnlock()
	s.observer(Observation{Event: &EventRecord{IncidentID: s.incidentID, ConnectionID: clientID,
		StreamID: s.id, Severity: severity, Code: code, Message: SanitizeDiagnostic(message),
		SourcePosition: position, Details: details, CreatedAt: time.Now()}})
}

func (s *Session) observeSession(endedAt *time.Time, reason string) {
	if s.observer == nil {
		return
	}
	snapshot := s.Snapshot()
	code := ""
	if snapshot.LastError != "" {
		code, _ = ClassifyError(errors.New(snapshot.LastError))
	}
	s.mu.RLock()
	firstMedia := s.firstMediaAt
	s.mu.RUnlock()
	s.observer(Observation{Session: &SessionRecord{IncidentID: s.incidentID, StreamID: s.id,
		State: snapshot.State, SourceCount: snapshot.SourceCount, Reconnects: snapshot.Reconnects,
		BytesIngested: snapshot.BytesPublished, BytesDelivered: snapshot.BytesDelivered,
		SlowClientDrops: snapshot.SlowClientDrops, StartedAt: snapshot.StartedAt,
		FirstMediaAt: firstMedia, EndedAt: endedAt, EndReason: reason,
		ErrorCode: code, LastError: snapshot.LastError}})
}

func (s *Session) observeCompleted() {
	now := time.Now()
	snapshot := s.Snapshot()
	reason := "stopped"
	if snapshot.State == StateFailed {
		reason = "failed"
	}
	s.mu.RLock()
	clientIDs := make([]string, 0, len(s.clients))
	for id := range s.clients {
		clientIDs = append(clientIDs, id)
	}
	s.mu.RUnlock()
	for _, id := range clientIDs {
		s.CloseClient(id, "session_ended")
	}
	s.observeSession(&now, reason)
}

func (s *Session) observeConnection(client *sessionClient, endedAt *time.Time) {
	if s.observer == nil || client == nil {
		return
	}
	record := &ConnectionRecord{ID: client.metadata.ID, IncidentID: s.incidentID, StreamID: s.id,
		Protocol: client.metadata.Protocol, RemoteIP: client.metadata.RemoteIP, Method: client.metadata.Method,
		UserAgent: client.metadata.UserAgent, StartedAt: client.startedAt, LastSeenAt: client.lastSeenAt,
		EndedAt: endedAt, BytesDelivered: client.bytesDelivered, EndReason: client.endReason}
	s.observer(Observation{Connection: record})
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
