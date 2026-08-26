package streaming

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeProducer struct {
	startErr error
	ready    chan struct{}
	hlsReady chan struct{}
	errors   chan error
	stops    atomic.Int32
}

func newReadyFake() *fakeProducer {
	ready := make(chan struct{})
	close(ready)
	hlsReady := make(chan struct{})
	close(hlsReady)
	return &fakeProducer{ready: ready, hlsReady: hlsReady, errors: make(chan error, 1)}
}

func (p *fakeProducer) Start() error              { return p.startErr }
func (p *fakeProducer) Ready() <-chan struct{}    { return p.ready }
func (p *fakeProducer) HLSReady() <-chan struct{} { return p.hlsReady }
func (p *fakeProducer) Errors() <-chan error      { return p.errors }
func (p *fakeProducer) Stop()                     { p.stops.Add(1) }
func (p *fakeProducer) LastDataAt() time.Time     { return time.Now() }

type blockingStopProducer struct {
	ready       chan struct{}
	hlsReady    chan struct{}
	errors      chan error
	stopStarted chan struct{}
	releaseStop chan struct{}
	stopOnce    sync.Once
}

type blockingLifecycleProducer struct {
	ready       chan struct{}
	hlsReady    chan struct{}
	errors      chan error
	startCalled chan struct{}
	stopCalled  chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	stopOnce    sync.Once
}

func newBlockingLifecycleProducer() *blockingLifecycleProducer {
	return &blockingLifecycleProducer{ready: make(chan struct{}), hlsReady: make(chan struct{}),
		errors: make(chan error, 1), startCalled: make(chan struct{}), stopCalled: make(chan struct{}),
		release: make(chan struct{})}
}

func (p *blockingLifecycleProducer) Start() error {
	p.startOnce.Do(func() { close(p.startCalled) })
	<-p.release
	return nil
}
func (p *blockingLifecycleProducer) Ready() <-chan struct{}    { return p.ready }
func (p *blockingLifecycleProducer) HLSReady() <-chan struct{} { return p.hlsReady }
func (p *blockingLifecycleProducer) Errors() <-chan error      { return p.errors }
func (p *blockingLifecycleProducer) LastDataAt() time.Time     { return time.Time{} }
func (p *blockingLifecycleProducer) Stop() {
	p.stopOnce.Do(func() { close(p.stopCalled) })
	<-p.release
}

func newBlockingStopProducer() *blockingStopProducer {
	ready := make(chan struct{})
	close(ready)
	hlsReady := make(chan struct{})
	close(hlsReady)
	return &blockingStopProducer{ready: ready, hlsReady: hlsReady, errors: make(chan error, 1),
		stopStarted: make(chan struct{}), releaseStop: make(chan struct{})}
}

func (p *blockingStopProducer) Start() error              { return nil }
func (p *blockingStopProducer) Ready() <-chan struct{}    { return p.ready }
func (p *blockingStopProducer) HLSReady() <-chan struct{} { return p.hlsReady }
func (p *blockingStopProducer) Errors() <-chan error      { return p.errors }
func (p *blockingStopProducer) LastDataAt() time.Time     { return time.Now() }
func (p *blockingStopProducer) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopStarted)
		<-p.releaseStop
	})
}

func testConfig(root string) Config {
	return Config{
		StartupTimeout:    3 * time.Second,
		StallTimeout:      3 * time.Second,
		IdleTimeout:       10 * time.Second,
		RetryLimit:        4,
		RetryBackoff:      100 * time.Millisecond,
		HLSSegmentSeconds: 1,
		HLSPlaylistLength: 3,
		ClientBufferBytes: 1024 * 1024,
		StreamRoot:        root,
	}
}

func TestManagerDeduplicatesConcurrentStarts(t *testing.T) {
	var starts atomic.Int32
	factory := func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		starts.Add(1)
		return newReadyFake()
	}
	config := testConfig(t.TempDir())
	manager := NewManager(factory, func() Config { return config })
	t.Cleanup(manager.Close)

	var wait sync.WaitGroup
	errorsFound := make(chan error, 20)
	for range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := manager.Acquire(context.Background(), "channel-1", []string{"https://example.com/live.ts"})
			if err != nil {
				errorsFound <- err
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent acquire failed: %v", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("producer starts = %d, want 1", starts.Load())
	}
}

func TestManagerStartSourcesReturnsBeforeProducerReady(t *testing.T) {
	producer := &fakeProducer{ready: make(chan struct{}), hlsReady: make(chan struct{}), errors: make(chan error, 1)}
	config := testConfig(t.TempDir())
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return producer
	}, func() Config { return config })
	t.Cleanup(manager.Close)

	started := time.Now()
	session, err := manager.StartSources(context.Background(), "cold-channel", []Source{{URL: "https://example.com/live.ts"}})
	if err != nil {
		t.Fatalf("non-blocking start failed: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("non-blocking start waited %s for producer readiness", elapsed)
	}
	if state := session.Snapshot().State; state != StateStarting {
		t.Fatalf("new session state = %s, want %s before producer readiness", state, StateStarting)
	}

	close(producer.ready)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.WaitReady(ctx); err != nil {
		t.Fatalf("session did not become ready after producer validation: %v", err)
	}
}

func TestManagerFailsOverAndKeepsSession(t *testing.T) {
	var mu sync.Mutex
	created := []*fakeProducer{}
	factory := func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		producer := newReadyFake()
		if source == "https://primary.example/live.ts" {
			producer.startErr = errors.New("primary offline")
		}
		mu.Lock()
		created = append(created, producer)
		mu.Unlock()
		return producer
	}
	config := testConfig(t.TempDir())
	manager := NewManager(factory, func() Config { return config })
	t.Cleanup(manager.Close)

	session, err := manager.Acquire(context.Background(), "channel-2", []string{
		"https://primary.example/live.ts",
		"https://backup.example/live.ts",
	})
	if err != nil {
		t.Fatalf("backup did not start: %v", err)
	}
	if got := session.Snapshot().SourcePosition; got != 2 {
		t.Fatalf("source position = %d, want ordered backup position 2", got)
	}

	mu.Lock()
	backup := created[len(created)-1]
	mu.Unlock()
	backup.errors <- errors.New("backup stalled")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if session.Snapshot().Reconnects >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("long-running producer failure did not trigger failover")
}

func TestManagerReturnsFailureAfterBoundedAttempts(t *testing.T) {
	factory := func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		producer := newReadyFake()
		producer.startErr = errors.New("source offline")
		return producer
	}
	config := testConfig(t.TempDir())
	config.RetryLimit = 2
	manager := NewManager(factory, func() Config { return config })
	t.Cleanup(manager.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := manager.Acquire(ctx, "channel-3", []string{"https://example.com/live.ts"}); err == nil {
		t.Fatal("unavailable source unexpectedly started")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if manager.Summary().Sessions == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("failed session was not removed from manager")
}

func TestManagerStopRemovesSessionFilesAndProducer(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "channel-cleanup")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "segment.ts"), []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	var producer *fakeProducer
	factory := func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		producer = newReadyFake()
		return producer
	}
	config := testConfig(root)
	manager := NewManager(factory, func() Config { return config })
	defer manager.Close()
	if _, err := manager.Acquire(context.Background(), "channel-cleanup", []string{"https://example.com/live.ts"}); err != nil {
		t.Fatal(err)
	}
	manager.Stop("channel-cleanup")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if manager.Summary().Sessions == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if manager.Summary().Sessions != 0 {
		t.Fatal("stopped session remained registered")
	}
	if producer == nil || producer.stops.Load() == 0 {
		t.Fatal("producer was not stopped")
	}
	if _, err := os.Stat(filepath.Join(root, "channel-cleanup")); !os.IsNotExist(err) {
		t.Fatalf("session files remained after cleanup: %v", err)
	}
}

func TestManualStopImmediatelyClosesViewersAndRejectsLateClients(t *testing.T) {
	config := testConfig(t.TempDir())
	config.CleanupTimeout = time.Second
	producer := newBlockingStopProducer()
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return producer
	}, func() Config { return config })
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(producer.releaseStop) })
		manager.Close()
	})

	session, err := manager.Acquire(context.Background(), "manual-stop", []string{"https://example.com/live.ts"})
	if err != nil {
		t.Fatal(err)
	}
	subscription := session.Subscribe()
	if _, allowed := session.RegisterClient(ClientMetadata{ID: "viewer", Protocol: "mpegts"}, subscription.Close); !allowed {
		t.Fatal("initial viewer was rejected")
	}

	started := time.Now()
	if !manager.StopManual("manual-stop") {
		t.Fatal("manual stop was not accepted")
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("manual stop blocked on producer cleanup for %s", elapsed)
	}
	if snapshot := session.Snapshot(); snapshot.State != StateStopping || snapshot.Clients != 0 {
		t.Fatalf("manual stop did not immediately close viewers: %#v", snapshot)
	}
	if _, ok := subscription.Next(); ok {
		t.Fatal("downstream subscription remained open during producer cleanup")
	}
	if _, allowed := session.RegisterClient(ClientMetadata{ID: "late-viewer", Protocol: "mpegts"}, nil); allowed {
		t.Fatal("stopping session accepted a late viewer")
	}
	if _, err := manager.Acquire(context.Background(), "manual-stop", []string{"https://example.com/live.ts"}); !errors.Is(err, ErrSessionStopping) {
		t.Fatalf("stopping session was reused: %v", err)
	}

	select {
	case <-producer.stopStarted:
	case <-time.After(time.Second):
		t.Fatal("producer cleanup did not begin")
	}
	releaseOnce.Do(func() { close(producer.releaseStop) })
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && manager.Summary().Sessions != 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if manager.Summary().Sessions != 0 {
		t.Fatal("stopped session remained registered after producer cleanup")
	}
}

func TestProducerCleanupWatchdogRemovesSessionButReservesSourceCapacity(t *testing.T) {
	config := testConfig(t.TempDir())
	config.CleanupTimeout = 40 * time.Millisecond
	producer := newBlockingStopProducer()
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return producer
	}, func() Config { return config })
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(producer.releaseStop) })
		manager.Close()
	})
	source := Source{URL: "https://provider.example/live.ts", PoolID: 22, PoolName: "Provider", ConnectionLimit: 1}
	if _, err := manager.AcquireSources(context.Background(), "cleanup-watchdog", []Source{source}); err != nil {
		t.Fatal(err)
	}
	if !manager.StopManual("cleanup-watchdog") {
		t.Fatal("manual stop was not accepted")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && manager.Summary().Sessions != 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if manager.Summary().Sessions != 0 {
		t.Fatal("cleanup watchdog left the session stuck in stopping")
	}
	usage := manager.ConnectionUsage()
	if len(usage) != 1 || usage[0].Active != 1 {
		t.Fatalf("blocked producer released source capacity prematurely: %#v", usage)
	}

	releaseOnce.Do(func() { close(producer.releaseStop) })
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) && len(manager.ConnectionUsage()) != 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if usage := manager.ConnectionUsage(); len(usage) != 0 {
		t.Fatalf("source capacity was not released after delayed cleanup: %#v", usage)
	}
}

func TestManualStopCanCancelBlockedProducerStartup(t *testing.T) {
	config := testConfig(t.TempDir())
	config.CleanupTimeout = 40 * time.Millisecond
	producer := newBlockingLifecycleProducer()
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return producer
	}, func() Config { return config })
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(producer.release) })
		manager.Close()
	})
	source := Source{URL: "https://provider.example/start.ts", PoolID: 23, PoolName: "Provider", ConnectionLimit: 1}
	acquireDone := make(chan error, 1)
	go func() {
		_, err := manager.AcquireSources(context.Background(), "blocked-startup", []Source{source})
		acquireDone <- err
	}()
	select {
	case <-producer.startCalled:
	case <-time.After(time.Second):
		t.Fatal("producer startup did not begin")
	}
	if !manager.StopManual("blocked-startup") {
		t.Fatal("manual stop was not accepted during startup")
	}
	select {
	case <-producer.stopCalled:
	case <-time.After(time.Second):
		t.Fatal("producer cleanup did not begin after startup cancellation")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && manager.Summary().Sessions != 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if manager.Summary().Sessions != 0 {
		t.Fatal("blocked startup left the session stuck in stopping")
	}
	if usage := manager.ConnectionUsage(); len(usage) != 1 || usage[0].Active != 1 {
		t.Fatalf("blocked startup released source capacity prematurely: %#v", usage)
	}
	select {
	case err := <-acquireDone:
		if err == nil {
			t.Fatal("cancelled startup unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("acquire request remained blocked after cleanup watchdog")
	}

	releaseOnce.Do(func() { close(producer.release) })
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) && len(manager.ConnectionUsage()) != 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if usage := manager.ConnectionUsage(); len(usage) != 0 {
		t.Fatalf("startup lease was not released after delayed cleanup: %#v", usage)
	}
}

func TestMPEGTSViewerExpiresWhenNoMediaCanDetectDisconnect(t *testing.T) {
	config := testConfig(t.TempDir())
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return newReadyFake()
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	session, err := manager.Acquire(context.Background(), "viewer-timeout", []string{"https://example.com/live.ts"})
	if err != nil {
		t.Fatal(err)
	}
	subscription := session.Subscribe()
	if _, allowed := session.RegisterClient(ClientMetadata{ID: "stale-mpegts", Protocol: "mpegts"}, subscription.Close); !allowed {
		t.Fatal("viewer registration failed")
	}
	now := time.Now()
	session.mu.Lock()
	session.clients["stale-mpegts"].lastSeenAt = now.Add(-minimumClientTimeout - time.Second)
	session.mu.Unlock()
	session.sample(now)
	if snapshot := session.Snapshot(); snapshot.Clients != 0 {
		t.Fatalf("stale MPEG-TS viewer remained active: %#v", snapshot.ClientDetails)
	}
	if _, ok := subscription.Next(); ok {
		t.Fatal("stale MPEG-TS subscription was not cancelled")
	}
}

func TestManagerPrunesCrashLeftStreamFilesButPreservesActiveDirectory(t *testing.T) {
	root := t.TempDir()
	staleDirectory := filepath.Join(root, "stale-channel")
	if err := os.MkdirAll(staleDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDirectory, "segment.ts"), []byte("stale media"), 0o644); err != nil {
		t.Fatal(err)
	}

	config := testConfig(root)
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return newReadyFake()
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	if _, err := manager.Acquire(context.Background(), "active-channel", []string{"https://example.com/live.ts"}); err != nil {
		t.Fatal(err)
	}
	activeDirectory := filepath.Join(root, "active-channel")
	if err := os.MkdirAll(activeDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeDirectory, "segment.ts"), []byte("active media"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := manager.PruneOrphanedStreamFiles()
	if err != nil {
		t.Fatal(err)
	}
	if report.DirectoriesRemoved != 1 || report.BytesReclaimed != int64(len("stale media")) {
		t.Fatalf("unexpected file cleanup report: %+v", report)
	}
	if _, err := os.Stat(staleDirectory); !os.IsNotExist(err) {
		t.Fatalf("stale stream directory was not removed: %v", err)
	}
	if _, err := os.Stat(activeDirectory); err != nil {
		t.Fatalf("active stream directory was removed: %v", err)
	}
}

func TestSessionBoundsEndedClientsWithoutLosingDeliveryTotals(t *testing.T) {
	config := testConfig(t.TempDir())
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return newReadyFake()
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	session, err := manager.Acquire(context.Background(), "client-retention", []string{"https://example.com/live.ts"})
	if err != nil {
		t.Fatal(err)
	}

	const clients = maximumEndedClientRows + 2
	for index := 0; index < clients; index++ {
		id := fmt.Sprintf("client-%d", index)
		if _, accepted := session.RegisterClient(ClientMetadata{ID: id, Protocol: "hls"}, nil); !accepted {
			t.Fatalf("client %s was rejected", id)
		}
		session.AddClientBytes(id, 10)
		session.CloseClient(id, "test_complete")
	}
	session.sample(time.Now().Add(time.Second))

	session.mu.RLock()
	retained := len(session.clients)
	session.mu.RUnlock()
	if retained != maximumEndedClientRows {
		t.Fatalf("retained %d ended clients, want %d", retained, maximumEndedClientRows)
	}
	if delivered := session.Snapshot().BytesDelivered; delivered != clients*10 {
		t.Fatalf("delivery total changed after client cleanup: got %d, want %d", delivered, clients*10)
	}
}

func TestManagerEnforcesPlaylistConnectionLimitAcrossChannels(t *testing.T) {
	config := testConfig(t.TempDir())
	config.RetryLimit = 1
	config.StartupHedge = 0
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return newReadyFake()
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	source := Source{URL: "https://provider.example/live.ts", PoolID: 9, PoolName: "Provider", ConnectionLimit: 1}
	first, err := manager.AcquireSources(context.Background(), "limited-one", []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if _, allowed := first.RegisterClient(ClientMetadata{ID: "active-viewer", Protocol: "mpegts"}, nil); !allowed {
		t.Fatal("active viewer registration failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := manager.AcquireSources(ctx, "limited-two", []Source{source}); err == nil {
		t.Fatal("a second channel exceeded the playlist connection limit")
	}
	usage := manager.ConnectionUsage()
	if len(usage) != 1 || usage[0].Active != 1 || usage[0].Limit != 1 {
		t.Fatalf("unexpected connection usage: %#v", usage)
	}
}

func TestManagerReclaimsIdleSessionUnderSourcePressure(t *testing.T) {
	config := testConfig(t.TempDir())
	config.RetryLimit = 1
	config.StartupHedge = 0
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return newReadyFake()
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	source := Source{URL: "https://provider.example/live.ts", PoolID: 10, PoolName: "Provider", ConnectionLimit: 1}
	first, err := manager.AcquireSources(context.Background(), "idle-one", []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if first.ActiveClientCount() != 0 {
		t.Fatal("new session unexpectedly has viewers")
	}
	if _, err := manager.AcquireSources(context.Background(), "idle-two", []Source{source}); err != nil {
		t.Fatalf("idle capacity was not reclaimed: %v", err)
	}
	if _, ok := manager.Get("idle-one"); ok {
		t.Fatal("reclaimed idle session remained registered")
	}
}

func TestManagerColdStartupRacesBackupWithIndependentCapacity(t *testing.T) {
	config := testConfig(t.TempDir())
	config.StartupHedge = 40 * time.Millisecond
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		producer := newReadyFake()
		if strings.Contains(source, "primary") {
			producer.ready = make(chan struct{})
			go func() {
				time.Sleep(400 * time.Millisecond)
				close(producer.ready)
			}()
		}
		return producer
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	started := time.Now()
	session, err := manager.AcquireSources(context.Background(), "raced-channel", []Source{
		{URL: "https://primary.example/live.ts", PoolID: 1, ConnectionLimit: 1},
		{URL: "https://backup.example/live.ts", PoolID: 2, ConnectionLimit: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("backup race did not reduce startup time: %s", elapsed)
	}
	if session.Snapshot().SourcePosition != 2 {
		t.Fatalf("source position = %d, want fast backup 2", session.Snapshot().SourcePosition)
	}
}

func TestManagerManualFailoverPreparesReplacementBeforeStoppingCurrent(t *testing.T) {
	config := testConfig(t.TempDir())
	config.StartupHedge = 0
	var mu sync.Mutex
	created := map[string]*fakeProducer{}
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		producer := newReadyFake()
		mu.Lock()
		created[source] = producer
		mu.Unlock()
		return producer
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	session, err := manager.AcquireSources(context.Background(), "manual-switch", []Source{
		{URL: "https://one.example/live.ts", PoolID: 1, ConnectionLimit: 1},
		{URL: "https://two.example/live.ts", PoolID: 2, ConnectionLimit: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !session.ForceNextSource() {
		t.Fatal("manual failover was not accepted")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if session.Snapshot().SourcePosition == 2 {
			mu.Lock()
			old := created["https://one.example/live.ts"]
			mu.Unlock()
			if old == nil || old.stops.Load() == 0 {
				t.Fatal("old producer was not stopped after replacement promotion")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("manual replacement was not promoted")
}
