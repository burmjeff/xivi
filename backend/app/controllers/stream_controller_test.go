package controllers

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xivi/backend/pkg/streaming"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

type blockingControllerProducer struct {
	ready    chan struct{}
	hlsReady chan struct{}
	errors   chan error
	release  chan struct{}
	once     sync.Once
}

func newBlockingControllerProducer() *blockingControllerProducer {
	ready := make(chan struct{})
	close(ready)
	hlsReady := make(chan struct{})
	close(hlsReady)
	return &blockingControllerProducer{ready: ready, hlsReady: hlsReady, errors: make(chan error, 1), release: make(chan struct{})}
}

func (p *blockingControllerProducer) Start() error              { return nil }
func (p *blockingControllerProducer) Ready() <-chan struct{}    { return p.ready }
func (p *blockingControllerProducer) HLSReady() <-chan struct{} { return p.hlsReady }
func (p *blockingControllerProducer) Errors() <-chan error      { return p.errors }
func (p *blockingControllerProducer) LastDataAt() time.Time     { return time.Now() }
func (p *blockingControllerProducer) Stop()                     { <-p.release }

func TestV2StopStreamAcknowledgesBeforeProducerCleanupCompletes(t *testing.T) {
	producer := newBlockingControllerProducer()
	streamRoot := t.TempDir()
	manager := streaming.NewManager(func(id, source string, generation uint64, config streaming.Config, hub *streaming.Hub) streaming.Producer {
		return producer
	}, func() streaming.Config {
		return streaming.Config{StreamRoot: streamRoot, StartupTimeout: 3 * time.Second, CleanupTimeout: time.Second}
	})
	previousManager := streaming.DefaultManager
	streaming.DefaultManager = manager
	t.Cleanup(func() {
		producer.once.Do(func() { close(producer.release) })
		manager.Close()
		streaming.DefaultManager = previousManager
	})
	if _, err := manager.Acquire(context.Background(), "controller-stop", []string{"https://example.test/live.ts"}); err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	app.Delete("/api/v2/studio/streams/:stream_id", V2StopStream)
	started := time.Now()
	response, err := app.Test(httptest.NewRequest("DELETE", "/api/v2/studio/streams/controller-stop", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("stop endpoint waited for producer cleanup for %s", elapsed)
	}
	if response.StatusCode != fiber.StatusAccepted {
		t.Fatalf("stop endpoint status = %d, want %d", response.StatusCode, fiber.StatusAccepted)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"status":"stop_requested"`) {
		t.Fatalf("unexpected stop response: %s", body)
	}
	producer.once.Do(func() { close(producer.release) })
}

func TestSendHLSPlaylistUsesUncachedStableSnapshot(t *testing.T) {
	root := t.TempDir()
	previousRoot := settings.STREAM_FILEPATH
	settings.STREAM_FILEPATH = root
	t.Cleanup(func() { settings.STREAM_FILEPATH = previousRoot })

	streamID := "channel-1"
	directory := filepath.Join(root, streamID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "segment.1.00001.ts"), []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	playlist := "#EXTM3U\n#EXTINF:2.0,\n/stream/hls/channel-1/segment.1.00001.ts\n"
	if err := os.WriteFile(filepath.Join(directory, "playlist.m3u8"), []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	app.Get("/playlist.m3u8", func(c *fiber.Ctx) error {
		return sendHLSFile(c, streamID, "playlist.m3u8")
	})
	response, err := app.Test(httptest.NewRequest("GET", "/playlist.m3u8", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || string(body) != playlist {
		t.Fatalf("unexpected playlist response: status=%d body=%q", response.StatusCode, body)
	}
	if response.Header.Get("Last-Modified") != "" {
		t.Fatalf("live playlist included a cache validator: %q", response.Header.Get("Last-Modified"))
	}
	if cacheControl := response.Header.Get("Cache-Control"); !strings.Contains(cacheControl, "no-store") {
		t.Fatalf("live playlist cache policy = %q", cacheControl)
	}
}

func TestSendHLSPlaylistCarriesLogicalViewerIntoSegments(t *testing.T) {
	root := t.TempDir()
	previousRoot := settings.STREAM_FILEPATH
	settings.STREAM_FILEPATH = root
	t.Cleanup(func() { settings.STREAM_FILEPATH = previousRoot })
	directory := filepath.Join(root, "channel-1")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	playlist := "#EXTM3U\n#EXTINF:2.0,\n/stream/hls/channel-1/segment.1.ts\n"
	if err := os.WriteFile(filepath.Join(directory, "playlist.m3u8"), []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/playlist.m3u8", func(c *fiber.Ctx) error { return sendHLSFile(c, "channel-1", "playlist.m3u8") })
	response, err := app.Test(httptest.NewRequest("GET", "/playlist.m3u8?viewer_id=browser-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "/stream/hls/channel-1/segment.1.ts?viewer_id=browser-1") {
		t.Fatalf("viewer id did not propagate to segment requests: %q", body)
	}
}

func TestSendHLSPlaylistScopesRelativeSegmentsToStream(t *testing.T) {
	root := t.TempDir()
	previousRoot := settings.STREAM_FILEPATH
	settings.STREAM_FILEPATH = root
	t.Cleanup(func() { settings.STREAM_FILEPATH = previousRoot })
	directory := filepath.Join(root, "channel-1")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	playlist := "#EXTM3U\n#EXTINF:2.0,\nsegment.1.ts\n"
	if err := os.WriteFile(filepath.Join(directory, "playlist.m3u8"), []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/stream/hls/channel-1", func(c *fiber.Ctx) error {
		return sendHLSFile(c, "channel-1", "playlist.m3u8")
	})
	response, err := app.Test(httptest.NewRequest("GET", "/stream/hls/channel-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "/stream/hls/channel-1/segment.1.ts") {
		t.Fatalf("relative segment was not scoped to its stream: %q", body)
	}
}
