package streaming

import (
	"context"
	"encoding/json"
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
	"xivi/backend/pkg/outbound"
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
	StartupHedge      time.Duration
	StallTimeout      time.Duration
	HedgeTimeout      time.Duration
	IdleTimeout       time.Duration
	CleanupTimeout    time.Duration
	RetryLimit        int
	RetryBackoff      time.Duration
	HLSSegmentSeconds int
	HLSPlaylistLength int
	HLSCompatibility  bool
	ClientBufferBytes int
	PrewarmChannels   int
	TLSVerify         bool
	UserAgent         string
	StreamRoot        string
}

func CurrentConfig() Config {
	configured := settings.Current().Streaming
	return Config{
		IngestBuffer:      time.Duration(configured.IngestBufferMS) * time.Millisecond,
		StartupTimeout:    time.Duration(configured.StartupTimeoutSeconds) * time.Second,
		StartupHedge:      time.Duration(configured.StartupHedgeMS) * time.Millisecond,
		StallTimeout:      time.Duration(configured.StallTimeoutSeconds) * time.Second,
		HedgeTimeout:      time.Duration(configured.HedgeTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(configured.IdleTimeoutSeconds) * time.Second,
		RetryLimit:        configured.RetryLimit,
		RetryBackoff:      time.Duration(configured.RetryBackoffMS) * time.Millisecond,
		HLSSegmentSeconds: configured.HLSSegmentSeconds,
		HLSPlaylistLength: configured.HLSPlaylistLength,
		HLSCompatibility:  configured.HLSCompatibilityMode,
		ClientBufferBytes: configured.ClientBufferMB * 1024 * 1024,
		PrewarmChannels:   configured.PrewarmChannels,
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
	if c.StartupHedge < 0 || c.StartupHedge >= c.StartupTimeout {
		c.StartupHedge = 750 * time.Millisecond
	}
	if c.StallTimeout < 3*time.Second {
		c.StallTimeout = 10 * time.Second
	}
	if c.HedgeTimeout < time.Second || c.HedgeTimeout >= c.StallTimeout {
		c.HedgeTimeout = max(time.Second, c.StallTimeout-2*time.Second)
	}
	if c.IdleTimeout < 10*time.Second {
		c.IdleTimeout = 45 * time.Second
	}
	if c.CleanupTimeout <= 0 {
		c.CleanupTimeout = 6 * time.Second
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
	if c.PrewarmChannels < 0 || c.PrewarmChannels > 8 {
		c.PrewarmChannels = 2
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

type HLSCompatibilityInspector interface {
	HLSCompatibilityActions() []string
}

type HLSProducer interface {
	HLSGeneration() uint64
	HLSPlaylistSnapshot() (*HLSPlaylistSnapshot, error)
}

type Manager struct {
	mu                sync.RWMutex
	sessions          map[string]*Session
	playbackMu        sync.Mutex
	playbacks         map[string]playbackLease
	playbackStripes   [64]sync.Mutex
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
	connections       *connectionCoordinator
	sourceHealth      map[string]*sourceHealthState
	sourceValidator   func(context.Context, Source) error
	sourceRelay       func(Source) (*outbound.Relay, error)
}

type Session struct {
	id                 string
	incidentID         string
	mu                 sync.RWMutex
	sources            []Source
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
	prewarm            bool
	observer           func(Observation)
	connections        *connectionCoordinator
	hls                hlsTimeline
	sourceAllowed      func(Source, int) bool
	recordSourceResult func(Source, time.Duration, error)
	validateSource     func(context.Context, Source) error
	relaySource        func(Source) (*outbound.Relay, error)
	hlsReadyOnce       sync.Once
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

type managedProducer struct {
	producer     Producer
	hub          *Hub
	lease        *connectionLease
	sourceIndex  int
	generation   uint64
	bridgeCancel context.CancelFunc
	bridgeDone   chan struct{}
	hedged       bool
	startedAt    time.Time
	readyAt      time.Time
	cleanupOnce  sync.Once
	cleanupAlert sync.Once
	cleanupDone  chan struct{}
	relay        *outbound.Relay
}

type sourceHealthState struct {
	failures      int
	retryAfter    time.Time
	averageStart  time.Duration
	lastSucceeded time.Time
	updatedAt     time.Time
}

type producerStartResult struct {
	managed *managedProducer
	err     error
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
	endedClientRetention    = 5 * time.Minute
	maximumEndedClientRows  = 1000
	minimumClientTimeout    = 35 * time.Second
	minimumHLSClientTimeout = 10 * time.Second
)

var validSessionID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

var (
	ErrSessionStopping        = errors.New("stream session is stopping")
	ErrProducerCleanupTimeout = errors.New("stream producer cleanup timed out")
)

var DefaultManager = NewManager(nil, nil)

func NewManager(factory ProducerFactory, configProvider func() Config) *Manager {
	var sourceValidator func(context.Context, Source) error
	var sourceRelay func(Source) (*outbound.Relay, error)
	if factory == nil {
		factory = newGSTProducer
		sourceValidator = func(ctx context.Context, source Source) error {
			_, err := outbound.Validate(ctx, source.URL, true)
			return err
		}
		sourceRelay = func(source Source) (*outbound.Relay, error) {
			return outbound.StartRelay(source.URL, outbound.Policy{
				AllowPrivate: true,
				Timeout:      20 * time.Second,
				MaxRedirects: 3,
			})
		}
	}
	if configProvider == nil {
		configProvider = CurrentConfig
	}
	manager := &Manager{
		sessions:        make(map[string]*Session),
		playbacks:       make(map[string]playbackLease),
		factory:         factory,
		config:          configProvider,
		cleanupStop:     make(chan struct{}),
		cleanupDone:     make(chan struct{}),
		connections:     newConnectionCoordinator(),
		sourceHealth:    make(map[string]*sourceHealthState),
		sourceValidator: sourceValidator,
		sourceRelay:     sourceRelay,
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
	variants := make([]Source, 0, len(sources))
	for _, source := range sources {
		variants = append(variants, Source{URL: source})
	}
	return m.AcquireSources(ctx, id, variants)
}

func (m *Manager) AcquireSources(ctx context.Context, id string, sources []Source) (*Session, error) {
	return m.acquireSources(ctx, id, sources, false, true)
}

// StartSources creates or reuses a session without holding the downstream
// MPEG-TS request until upstream validation completes. Subscribers may attach
// immediately and receive the decoder-safe warm bootstrap when it is ready.
func (m *Manager) StartSources(ctx context.Context, id string, sources []Source) (*Session, error) {
	return m.acquireSources(ctx, id, sources, false, false)
}

func (m *Manager) PrewarmSources(ctx context.Context, id string, sources []Source) (*Session, error) {
	return m.acquireSources(ctx, id, sources, true, true)
}

func (m *Manager) acquireSources(ctx context.Context, id string, sources []Source, prewarm, waitReady bool) (*Session, error) {
	cleanSources, err := validateSources(id, sources)
	if err != nil {
		return nil, err
	}
	// Fiber/fasthttp exposes zero-copy parameter strings. A stream outlives the
	// request that created it, so retaining that storage would let later
	// requests mutate map keys and diagnostic IDs underneath the manager.
	id = strings.Clone(id)
	if !prewarm {
		m.reclaimPrewarms(cleanSources, id)
		if _, reusable := m.Get(id); !reusable {
			m.ensureSourceCapacity(ctx, cleanSources, id)
		}
	}

	m.mu.Lock()
	if existing := m.sessions[id]; existing != nil {
		existing.mu.RLock()
		stopping := existing.state == StateStopping || existing.state == StateStopped
		existing.mu.RUnlock()
		if stopping {
			m.mu.Unlock()
			return existing, ErrSessionStopping
		}
		existing.touch()
		existing.updateSources(cleanSources)
		if !prewarm {
			existing.mu.Lock()
			existing.prewarm = false
			existing.mu.Unlock()
		}
		m.mu.Unlock()
		if waitReady {
			if err := existing.WaitReady(ctx); err != nil {
				return nil, err
			}
		}
		return existing, nil
	}

	config := m.config().normalized()
	if prewarm {
		if config.PrewarmChannels == 0 {
			m.mu.Unlock()
			return nil, errors.New("stream prewarming is disabled")
		}
		prewarmCount := 0
		for _, candidate := range m.sessions {
			candidate.mu.RLock()
			if candidate.prewarm {
				prewarmCount++
			}
			candidate.mu.RUnlock()
		}
		if prewarmCount >= config.PrewarmChannels {
			m.mu.Unlock()
			return nil, errors.New("stream prewarm pool is full")
		}
		config.RetryLimit = min(config.RetryLimit, max(len(cleanSources), 1))
		config.IdleTimeout = min(config.IdleTimeout, 20*time.Second)
	}
	sessionContext, cancel := context.WithCancel(context.Background())
	session := &Session{
		id:                 id,
		incidentID:         randomIdentifier("str_"),
		sources:            cleanSources,
		config:             config,
		factory:            m.factory,
		hub:                NewHub(config.ClientBufferBytes),
		state:              StateStarting,
		lastAccess:         time.Now(),
		startedAt:          time.Now(),
		sourceIndex:        -1,
		initialDone:        make(chan struct{}),
		ctx:                sessionContext,
		cancel:             cancel,
		done:               make(chan struct{}),
		forceNext:          make(chan error, 1),
		clients:            make(map[string]*sessionClient),
		lastSampleAt:       time.Now(),
		observer:           m.observe,
		connections:        m.connections,
		prewarm:            prewarm,
		sourceAllowed:      m.sourceIsAllowed,
		recordSourceResult: m.recordSourceResult,
		validateSource:     m.sourceValidator,
		relaySource:        m.sourceRelay,
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

	if waitReady {
		if err := session.WaitReady(ctx); err != nil {
			return session, err
		}
	}
	return session, nil
}

func (m *Manager) reclaimPrewarms(sources []Source, exceptID string) {
	pools := make(map[int64]bool)
	for _, source := range sources {
		if source.PoolID != 0 {
			pools[source.PoolID] = true
		}
	}
	if len(pools) == 0 {
		return
	}
	m.mu.RLock()
	candidates := make([]*Session, 0)
	for id, session := range m.sessions {
		if id == exceptID {
			continue
		}
		session.mu.RLock()
		prewarm := session.prewarm
		position := session.sourceIndex
		poolID := int64(0)
		if position >= 0 && position < len(session.sources) {
			poolID = session.sources[position].PoolID
		}
		session.mu.RUnlock()
		if prewarm && pools[poolID] {
			candidates = append(candidates, session)
		}
	}
	m.mu.RUnlock()
	for _, session := range candidates {
		session.emitEvent("info", "prewarm_reclaimed", "Prewarmed capacity was released for an active viewer.", "")
		session.Stop()
	}
	// Stop is asynchronous because the GStreamer pipeline must first transition
	// to NULL. Give the reclaimed leases a short, bounded window to return so a
	// real viewer does not lose the first startup attempt to speculative work.
	deadline := time.NewTimer(350 * time.Millisecond)
	defer deadline.Stop()
	for _, session := range candidates {
		select {
		case <-session.done:
		case <-deadline.C:
			return
		}
	}
}

func (m *Manager) sourceIsAllowed(source Source, sourceCount int) bool {
	if sourceCount < 2 {
		return true
	}
	m.mu.RLock()
	health := m.sourceHealth[source.URL]
	allowed := health == nil || time.Now().After(health.retryAfter)
	m.mu.RUnlock()
	return allowed
}

func (m *Manager) recordSourceResult(source Source, duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	health := m.sourceHealth[source.URL]
	if health == nil {
		health = &sourceHealthState{}
		m.sourceHealth[source.URL] = health
	}
	health.updatedAt = time.Now()
	if len(m.sourceHealth) > 5000 {
		var oldestURL string
		var oldest time.Time
		for url, candidate := range m.sourceHealth {
			if url == source.URL {
				continue
			}
			if oldestURL == "" || candidate.updatedAt.Before(oldest) {
				oldestURL, oldest = url, candidate.updatedAt
			}
		}
		delete(m.sourceHealth, oldestURL)
	}
	if err == nil {
		health.failures = 0
		health.retryAfter = time.Time{}
		health.lastSucceeded = time.Now()
		if health.averageStart == 0 {
			health.averageStart = duration
		} else {
			health.averageStart = (health.averageStart*3 + duration) / 4
		}
		return
	}
	health.failures++
	cooldown := time.Duration(1<<min(health.failures-1, 5)) * time.Second
	health.retryAfter = time.Now().Add(cooldown)
}

func validateSources(id string, sources []Source) ([]Source, error) {
	if !validSessionID.MatchString(id) {
		return nil, fmt.Errorf("invalid stream id")
	}
	clean := make([]Source, 0, len(sources))
	seen := make(map[string]bool)
	for _, source := range sources {
		parsed, err := url.Parse(source.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			continue
		}
		if !seen[source.URL] {
			seen[source.URL] = true
			if source.ConnectionLimit < 1 && source.PoolID != 0 {
				source.ConnectionLimit = 1
			}
			source.URL = strings.Clone(source.URL)
			source.PoolName = strings.Clone(source.PoolName)
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
	var active *managedProducer
	for {
		sources := s.sourceList()
		if len(sources) == 0 {
			s.failInitial(errors.New("no source variants are available"))
			return
		}
		if active == nil {
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
			var candidate *managedProducer
			var err error
			if !s.isReady() && len(sources) > 1 && s.config.StartupHedge > 0 {
				candidate, err = s.startColdProducerRace(sources, nextSource)
			} else {
				candidate, err = s.startManagedProducer(sources[nextSource], nextSource, len(sources))
			}
			if err != nil {
				if errors.Is(err, context.Canceled) || s.ctx.Err() != nil {
					if errors.Is(err, ErrProducerCleanupTimeout) {
						s.finish(StateFailed, err)
					} else {
						s.finish(StateStopped, nil)
					}
					return
				}
				attempt++
				s.reportSourceFailure(err, attempt)
				nextSource = (nextSource + 1) % len(sources)
				if nextSource == 0 && !s.wait(backoff(s.config.RetryBackoff, max(attempt-len(sources), 0))) {
					s.finish(StateStopped, nil)
					return
				}
				continue
			}
			s.promoteProducer(candidate, nil)
			active = candidate
			attempt = 0
		}

		reason, hedge := s.monitorProducer(active)
		if errors.Is(reason, context.Canceled) || s.ctx.Err() != nil {
			if cleanupErr := s.stopManagedProducer(active); cleanupErr != nil {
				s.finish(StateFailed, cleanupErr)
			} else {
				s.finish(StateStopped, nil)
			}
			return
		}

		s.mu.Lock()
		restartSource := s.restartSource
		s.restartSource = false
		s.mu.Unlock()
		target := active.sourceIndex
		if !restartSource {
			target = (target + 1) % len(sources)
		}

		// A suspected stall or manual operation first tries to prepare a
		// replacement while the current producer remains connected. The source
		// coordinator refuses this when it would exceed a playlist budget.
		if hedge || strings.Contains(reason.Error(), "manual") {
			candidate, candidateErr := s.startManagedProducer(sources[target], target, len(sources))
			if candidateErr == nil && s.hasActiveHLSClient() {
				candidateErr = s.waitManagedHLS(candidate)
				if candidateErr != nil {
					s.stopManagedProducer(candidate)
					candidate = nil
				}
			}
			if candidateErr == nil {
				s.promoteProducer(candidate, active)
				active = candidate
				attempt = 0
				continue
			}
			if hedge && active.producer.LastDataAt().After(time.Now().Add(-s.config.HedgeTimeout)) {
				s.emitEvent("warning", "hedged_recovery_cancelled", "The original source recovered before its replacement was ready.", "")
				active.hedged = false
				s.setState(StateRunning, nil)
				continue
			}
			if hedge {
				active.hedged = true
				continue
			}
			if code, _ := ClassifyError(candidateErr); code != "source_connection_limit" {
				s.setState(StateRunning, nil)
				s.emitEvent("warning", "replacement_failed", "The requested replacement failed to become ready; the current source was retained.", "")
				continue
			}
			// With a one-connection source budget, make-before-break is
			// intentionally impossible. Release the old lease before retrying.
			s.emitEvent("warning", "break_before_make", candidateErr.Error(), "")
		}

		s.stopManagedProducer(active)
		active = nil
		nextSource = target
		attempt++
		s.reportSourceFailure(reason, attempt)
	}
}

func (s *Session) hasActiveHLSClient() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, client := range s.clients {
		if client.endReason == "" && client.metadata.Protocol == "hls" {
			return true
		}
	}
	return false
}

func (s *Session) waitManagedHLS(managed *managedProducer) error {
	timer := time.NewTimer(s.config.StartupTimeout)
	defer timer.Stop()
	select {
	case <-managed.producer.HLSReady():
		return nil
	case err := <-managed.producer.Errors():
		return err
	case <-timer.C:
		return fmt.Errorf("replacement HLS segment was not ready within %s", s.config.StartupTimeout)
	case <-s.ctx.Done():
		return context.Canceled
	}
}

func (s *Session) startColdProducerRace(sources []Source, primaryIndex int) (*managedProducer, error) {
	results := make(chan producerStartResult, 2)
	start := func(index int) {
		managed, err := s.startManagedProducer(sources[index], index, len(sources))
		results <- producerStartResult{managed: managed, err: err}
	}
	go start(primaryIndex)
	timer := time.NewTimer(s.config.StartupHedge)
	defer timer.Stop()
	select {
	case result := <-results:
		if result.err == nil {
			return result.managed, nil
		}
		backupIndex := (primaryIndex + 1) % len(sources)
		return s.startManagedProducer(sources[backupIndex], backupIndex, len(sources))
	case <-timer.C:
	}

	backupIndex := (primaryIndex + 1) % len(sources)
	go start(backupIndex)
	var failures []error
	for completed := 0; completed < 2; completed++ {
		result := <-results
		if result.err != nil {
			failures = append(failures, result.err)
			continue
		}
		// Whichever ordered source becomes valid first wins. The other
		// candidate is stopped as soon as its startup attempt resolves.
		if completed == 0 {
			go func() {
				remaining := <-results
				if remaining.managed != nil {
					s.stopManagedProducer(remaining.managed)
				}
			}()
		}
		return result.managed, nil
	}
	return nil, errors.Join(failures...)
}

func (s *Session) startManagedProducer(source Source, sourceIndex, sourceCount int) (*managedProducer, error) {
	if s.validateSource != nil {
		validationTimeout := min(s.config.StartupTimeout, 5*time.Second)
		validationContext, cancel := context.WithTimeout(s.ctx, validationTimeout)
		err := s.validateSource(validationContext, source)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("outbound source rejected before connection: %w", err)
		}
	}
	if s.sourceAllowed != nil && !s.sourceAllowed(source, sourceCount) {
		return nil, errors.New("source is in a short health cooldown after recent failures")
	}
	lease, err := s.connections.acquire(source)
	if err != nil {
		return nil, err
	}
	producerSource := source.URL
	var relay *outbound.Relay
	if s.relaySource != nil {
		relay, err = s.relaySource(source)
		if err != nil {
			lease.Release()
			return nil, fmt.Errorf("create protected upstream relay: %w", err)
		}
		producerSource = relay.URL()
	}
	s.mu.Lock()
	s.generation++
	generation := s.generation
	starting := !s.everReady
	if starting {
		s.state = StateStarting
	} else {
		s.state = StateReconnecting
	}
	s.mu.Unlock()
	s.emitEvent("info", "source_connecting", fmt.Sprintf("Connecting to ordered source %d of %d.", sourceIndex+1, sourceCount), "")
	localHub := NewHub(s.config.ClientBufferBytes)
	producer := s.factory(s.id, producerSource, generation, s.config, localHub)
	managed := &managedProducer{producer: producer, hub: localHub, lease: lease,
		sourceIndex: sourceIndex, generation: generation, startedAt: time.Now(), relay: relay}
	startupDeadline := time.Now().Add(s.config.StartupTimeout)
	startResult := make(chan error, 1)
	go func() { startResult <- producer.Start() }()
	startTimer := time.NewTimer(time.Until(startupDeadline))
	var startErr error
	select {
	case startErr = <-startResult:
		if !startTimer.Stop() {
			select {
			case <-startTimer.C:
			default:
			}
		}
	case <-startTimer.C:
		startErr = fmt.Errorf("source pipeline construction exceeded %s: %w", s.config.StartupTimeout, context.DeadlineExceeded)
	case <-s.ctx.Done():
		if !startTimer.Stop() {
			select {
			case <-startTimer.C:
			default:
			}
		}
		startErr = context.Canceled
	}
	if startErr != nil {
		if s.recordSourceResult != nil {
			s.recordSourceResult(source, time.Since(managed.startedAt), startErr)
		}
		cleanupErr := s.stopManagedProducer(managed)
		return nil, errors.Join(startErr, cleanupErr)
	}
	remaining := time.Until(startupDeadline)
	if remaining < 0 {
		remaining = 0
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-producer.Ready():
		managed.readyAt = time.Now()
		if inspector, ok := producer.(HLSCompatibilityInspector); ok {
			for _, action := range inspector.HLSCompatibilityActions() {
				s.emitEvent("info", "hls_compatibility_transcode", action, "")
			}
		}
		if s.recordSourceResult != nil {
			s.recordSourceResult(source, managed.readyAt.Sub(managed.startedAt), nil)
		}
		return managed, nil
	case err := <-producer.Errors():
		if s.recordSourceResult != nil {
			s.recordSourceResult(source, time.Since(managed.startedAt), err)
		}
		cleanupErr := s.stopManagedProducer(managed)
		return nil, errors.Join(err, cleanupErr)
	case <-timer.C:
		if s.recordSourceResult != nil {
			s.recordSourceResult(source, time.Since(managed.startedAt), context.DeadlineExceeded)
		}
		status := localHub.BootstrapStatus()
		missing := "decoder bootstrap"
		if len(status.Missing) > 0 {
			missing = strings.Join(status.Missing, ", ")
		}
		cleanupErr := s.stopManagedProducer(managed)
		return nil, errors.Join(
			fmt.Errorf("source produced no decoder-ready MPEG-TS within %s (missing: %s)", s.config.StartupTimeout, missing),
			cleanupErr,
		)
	case <-s.ctx.Done():
		cleanupErr := s.stopManagedProducer(managed)
		return nil, errors.Join(context.Canceled, cleanupErr)
	}
}

func (s *Session) promoteProducer(next, previous *managedProducer) {
	if previous != nil {
		s.stopManagedBridge(previous)
	}
	s.hub.ResetForDiscontinuity()
	s.startManagedBridge(next)
	s.mu.Lock()
	wasReady := s.everReady
	s.current = next.producer
	s.sourceIndex = next.sourceIndex
	s.everReady = true
	s.state = StateRunning
	s.lastError = ""
	if wasReady {
		s.reconnects++
	}
	s.mu.Unlock()
	if wasReady && s.onReconnect != nil {
		s.onReconnect()
	}
	s.initialOnce.Do(func() { close(s.initialDone) })
	details, err := json.Marshal(struct {
		Generation uint64          `json:"generation"`
		StartupMS  int64           `json:"startup_ms"`
		Bootstrap  BootstrapStatus `json:"bootstrap"`
	}{
		Generation: next.generation,
		StartupMS:  next.readyAt.Sub(next.startedAt).Milliseconds(),
		Bootstrap:  next.hub.BootstrapStatus(),
	})
	if err != nil {
		details = []byte(fmt.Sprintf("{\"generation\":%d,\"startup_ms\":%d}", next.generation, next.readyAt.Sub(next.startedAt).Milliseconds()))
	}
	s.emitEvent("info", "media_ready", "Valid media is flowing from the selected source.", string(details))
	s.observeSession(nil, "")
	if previous != nil {
		s.stopManagedProducer(previous)
	}
}

func (s *Session) startManagedBridge(managed *managedProducer) {
	bridgeContext, cancel := context.WithCancel(s.ctx)
	managed.bridgeCancel = cancel
	managed.bridgeDone = make(chan struct{})
	subscription := managed.hub.Subscribe()
	go func() {
		defer close(managed.bridgeDone)
		defer subscription.Close()
		for {
			select {
			case <-bridgeContext.Done():
				return
			default:
			}
			chunk, ok := subscription.Next()
			if !ok {
				return
			}
			s.hub.PublishOwned(chunk)
		}
	}()
}

func (s *Session) stopManagedBridge(managed *managedProducer) {
	if managed == nil || managed.bridgeCancel == nil {
		return
	}
	managed.bridgeCancel()
	managed.hub.Close(context.Canceled)
	<-managed.bridgeDone
	managed.bridgeCancel = nil
}

func (s *Session) stopManagedProducer(managed *managedProducer) error {
	if managed == nil {
		return nil
	}
	s.stopManagedBridge(managed)
	managed.hub.Close(context.Canceled)
	managed.cleanupOnce.Do(func() {
		managed.cleanupDone = make(chan struct{})
		go func() {
			defer close(managed.cleanupDone)
			if managed.relay != nil {
				_ = managed.relay.Close()
			}
			managed.producer.Stop()
			managed.lease.Release()
			s.clearProducer(managed.producer)
		}()
	})
	timer := time.NewTimer(s.config.CleanupTimeout)
	defer timer.Stop()
	select {
	case <-managed.cleanupDone:
		return nil
	case <-timer.C:
		err := fmt.Errorf("%w after %s", ErrProducerCleanupTimeout, s.config.CleanupTimeout)
		s.clearProducer(managed.producer)
		managed.cleanupAlert.Do(func() {
			s.emitEvent("error", "producer_cleanup_timeout",
				"The upstream pipeline did not stop within its cleanup deadline; its source capacity remains reserved.",
				fmt.Sprintf("{\"timeout_ms\":%d}", s.config.CleanupTimeout.Milliseconds()))
		})
		return err
	}
}

func (s *Session) monitorProducer(active *managedProducer) (error, bool) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	hedgeAttempted := false
	for {
		select {
		case err := <-active.producer.Errors():
			return err, false
		case <-s.ctx.Done():
			return context.Canceled, false
		case err := <-s.forceNext:
			return err, false
		case <-ticker.C:
			last := active.producer.LastDataAt()
			if active.hedged && !last.IsZero() && time.Since(last) < s.config.HedgeTimeout/2 {
				active.hedged = false
			}
			if !hedgeAttempted && !active.hedged && !last.IsZero() && time.Since(last) >= s.config.HedgeTimeout {
				hedgeAttempted = true
				return fmt.Errorf("source delivery is delayed by %s", time.Since(last).Round(100*time.Millisecond)), true
			}
		}
	}
}

func (s *Session) reportSourceFailure(err error, attempt int) {
	s.setState(StateReconnecting, err)
	code, _ := ClassifyError(err)
	severity := "error"
	if strings.Contains(code, "manual") || code == "source_connection_limit" {
		severity = "warning"
	}
	s.emitEvent(severity, code, err.Error(), "")
	log.Warn().Str("error", SanitizeDiagnostic(err.Error())).Str("stream_id", s.id).Int("source_position", s.sourcePosition()).Int("attempt", attempt).Msg("Streaming source failed; trying the next ordered variant")
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

func (s *Session) clearProducer(producer Producer) {
	s.mu.Lock()
	if s.current == producer {
		s.current = nil
	}
	s.mu.Unlock()
}

func (s *Session) sourceList() []Source {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Source(nil), s.sources...)
}

func (s *Session) updateSources(sources []Source) {
	s.mu.Lock()
	s.sources = append([]Source(nil), sources...)
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
			s.hlsReadyOnce.Do(func() {
				s.emitEvent("info", "hls_ready", "The first independently playable HLS segment is ready.", "")
			})
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

func (s *Session) ActiveClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	active := 0
	for _, client := range s.clients {
		if client.endReason == "" {
			active++
		}
	}
	return active
}

func (s *Session) HasActiveClient(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	client := s.clients[id]
	return client != nil && client.endReason == ""
}

func (s *Session) RegisterClient(metadata ClientMetadata, cancel func()) (string, bool) {
	metadata.ID = strings.Clone(metadata.ID)
	metadata.Protocol = strings.Clone(metadata.Protocol)
	metadata.RemoteIP = strings.Clone(metadata.RemoteIP)
	metadata.Method = strings.Clone(metadata.Method)
	metadata.UserAgent = strings.Clone(metadata.UserAgent)
	now := time.Now()
	s.mu.Lock()
	if s.state == StateStopping || s.state == StateStopped {
		s.mu.Unlock()
		return metadata.ID, false
	}
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

func (s *Session) ReleaseClient(id, reason string) bool {
	s.mu.Lock()
	client := s.clients[id]
	if client == nil || client.endReason != "" {
		s.mu.Unlock()
		return false
	}
	cancel := client.cancel
	s.mu.Unlock()
	s.CloseClient(id, reason)
	if cancel != nil {
		cancel()
	}
	return true
}

func (s *Session) DisconnectClient(id string) bool {
	return s.ReleaseClient(id, "terminated_by_user")
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

func (s *Session) requestStop(manual bool) bool {
	requested := false
	s.stopOnce.Do(func() {
		requested = true
		if manual {
			s.emitEvent("warning", "manual_stop", "A user stopped the shared stream and its viewers.", "")
		}
		s.setState(StateStopping, nil)
		s.cancel()
		// Closing the public hub first wakes MPEG-TS response writers even when
		// the upstream GStreamer graph takes longer to transition to NULL.
		s.hub.Close(context.Canceled)
		s.closeAllClients("session_stopped")
	})
	return requested
}

func (s *Session) Stop() {
	s.requestStop(false)
}

func (s *Session) StopManual() bool {
	return s.requestStop(true)
}

func (s *Session) closeAllClients(reason string) {
	type endedClient struct {
		id     string
		client *sessionClient
		cancel func()
		ended  time.Time
	}
	ended := make([]endedClient, 0)
	now := time.Now()
	s.mu.Lock()
	for id, client := range s.clients {
		if client.endReason != "" {
			continue
		}
		client.endReason = reason
		client.lastSeenAt = now
		ended = append(ended, endedClient{id: id, client: client, cancel: client.cancel, ended: now})
	}
	s.mu.Unlock()
	for _, item := range ended {
		s.observeConnection(item.client, &item.ended)
		s.emitEventForClient(item.id, "info", "viewer_disconnected", "Viewer disconnected: "+reason+".", "")
		if item.cancel != nil {
			item.cancel()
		}
	}
}

func (s *Session) sample(now time.Time) {
	bytesIn, _ := s.hub.Metrics()
	type timedOutClient struct {
		record ConnectionRecord
		cancel func()
	}
	timedOut := []timedOutClient{}
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
			timeout := max(minimumHLSClientTimeout, time.Duration(3*s.config.HLSSegmentSeconds)*time.Second)
			if client.metadata.Protocol == "mpegts" {
				timeout = max(minimumClientTimeout, 2*s.config.StallTimeout)
			}
			if now.Sub(client.lastSeenAt) > timeout {
				client.endReason = "viewer_timeout"
				endedAt := now
				timedOut = append(timedOut, timedOutClient{record: ConnectionRecord{ID: client.metadata.ID,
					IncidentID: s.incidentID, StreamID: s.id, Protocol: client.metadata.Protocol,
					RemoteIP: client.metadata.RemoteIP, Method: client.metadata.Method,
					UserAgent: client.metadata.UserAgent, StartedAt: client.startedAt,
					LastSeenAt: client.lastSeenAt, EndedAt: &endedAt, BytesDelivered: client.bytesDelivered,
					EndReason: client.endReason}, cancel: client.cancel})
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
			item := timedOut[index]
			s.observer(Observation{Connection: &item.record})
			if item.cancel != nil {
				item.cancel()
			}
		}
	} else {
		for _, item := range timedOut {
			if item.cancel != nil {
				item.cancel()
			}
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
			AuthKind: client.metadata.AuthKind,
		})
	}
	snapshot.BytesDelivered = bytesOut
	snapshot.Samples = append([]MetricSample(nil), s.samples...)
	if len(snapshot.Samples) > 0 {
		last := snapshot.Samples[len(snapshot.Samples)-1]
		snapshot.IngressBitrateBPS, snapshot.EgressBitrateBPS = last.IngressBPS, last.EgressBPS
	}
	s.mu.RUnlock()
	sort.Slice(snapshot.ClientDetails, func(i, j int) bool {
		if snapshot.ClientDetails[i].StartedAt.Equal(snapshot.ClientDetails[j].StartedAt) {
			return snapshot.ClientDetails[i].ID < snapshot.ClientDetails[j].ID
		}
		return snapshot.ClientDetails[i].StartedAt.Before(snapshot.ClientDetails[j].StartedAt)
	})
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
	if !ok {
		return false
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.state == StateStopping || session.state == StateStopped {
		return false
	}
	session.lastAccess = time.Now()
	return true
}

func (m *Manager) Stop(id string) {
	if session, ok := m.Get(id); ok {
		session.Stop()
	}
}

func (m *Manager) StopManual(id string) bool {
	if session, ok := m.Get(id); ok {
		session.StopManual()
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

// DisconnectAuthorizedClients terminates downstream viewers whose credential
// or owning account was revoked. Shared producers stay alive for other users.
func (m *Manager) DisconnectAuthorizedClients(authKind string, authID, ownerUserID int64) int {
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()
	disconnected := 0
	for _, session := range sessions {
		session.mu.RLock()
		ids := make([]string, 0)
		for id, client := range session.clients {
			credentialMatch := authKind != "" && client.metadata.AuthKind == authKind && client.metadata.AuthID == authID
			ownerMatch := ownerUserID > 0 && client.metadata.OwnerUserID == ownerUserID
			if client.endReason == "" && (credentialMatch || ownerMatch) {
				ids = append(ids, id)
			}
		}
		session.mu.RUnlock()
		for _, id := range ids {
			if session.ReleaseClient(id, "authorization_revoked") {
				disconnected++
			}
		}
	}
	return disconnected
}

// DisconnectUserSessionClients ends browser-session playback after a username
// change without interrupting the same user's independently revocable media-key
// players. The shared upstream remains available to every other downstream.
func (m *Manager) DisconnectUserSessionClients(ownerUserID int64) int {
	if ownerUserID < 1 {
		return 0
	}
	m.mu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}
	m.mu.RUnlock()
	disconnected := 0
	for _, session := range sessions {
		session.mu.RLock()
		ids := make([]string, 0)
		for id, client := range session.clients {
			if client.endReason == "" && client.metadata.AuthKind == "session" && client.metadata.OwnerUserID == ownerUserID {
				ids = append(ids, id)
			}
		}
		session.mu.RUnlock()
		for _, id := range ids {
			if session.ReleaseClient(id, "authorization_rotated") {
				disconnected++
			}
		}
	}
	return disconnected
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
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartedAt.Equal(items[j].StartedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].StartedAt.After(items[j].StartedAt)
	})
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

func (m *Manager) ConnectionUsage() []ConnectionUsage { return m.connections.usage() }

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
			m.prunePlaybacks(now)
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
