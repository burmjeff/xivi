package controllers

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

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
