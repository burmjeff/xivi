package streaming

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
