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
	if _, err := manager.AcquireSources(context.Background(), "limited-one", []Source{source}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := manager.AcquireSources(ctx, "limited-two", []Source{source}); err == nil {
		t.Fatal("a second channel exceeded the playlist connection limit")
	}
	usage := manager.ConnectionUsage()
	if len(usage) != 1 || usage[0].Active != 1 || usage[0].Limit != 1 {
		t.Fatalf("unexpected connection usage: %#v", usage)
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
