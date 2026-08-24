package streaming

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadHLSPlaylistReturnsStableSnapshot(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"segment.1.00001.ts", "segment.1.00002.ts", "segment.1.00003.ts"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("media"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	playlist := "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:1.5,\n/stream/hls/channel/segment.1.00001.ts\n#EXTINF:2.0,\n/stream/hls/channel/segment.1.00002.ts\n#EXTINF:1.75,\n/stream/hls/channel/segment.1.00003.ts\n"
	path := filepath.Join(directory, "playlist.m3u8")
	if err := os.WriteFile(path, []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := ReadHLSPlaylist(path)
	if err != nil {
		t.Fatalf("read playlist: %v", err)
	}
	if len(snapshot.Segments) != 3 || snapshot.Duration != 5250*time.Millisecond {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if err := snapshot.ValidateSegments(directory); err != nil {
		t.Fatalf("validate advertised segments: %v", err)
	}
}

func TestReadHLSPlaylistRejectsPartialRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlist.m3u8")
	if err := os.WriteFile(path, []byte("#EXTM3U\n#EXTINF:2.0,\n/stream/hls/channel/segment.1.00001.ts"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHLSPlaylist(path); !errors.Is(err, ErrHLSPlaylistNotReady) {
		t.Fatalf("expected partial playlist to be rejected, got %v", err)
	}
}

func TestHLSPlaylistValidationRejectsMissingSegment(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "playlist.m3u8")
	playlist := "#EXTM3U\n#EXTINF:2.0,\n/stream/hls/channel/missing.ts\n"
	if err := os.WriteFile(path, []byte(playlist), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ReadHLSPlaylist(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.ValidateSegments(directory); !errors.Is(err, ErrHLSPlaylistNotReady) {
		t.Fatalf("expected missing segment to be rejected, got %v", err)
	}
}
