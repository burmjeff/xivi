package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"xivi/backend/app/models"
)

func TestParseM3uRejectsInvalidSnapshotsBeforeCleanup(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty", content: "", want: "playlist is empty"},
		{name: "no channels", content: "#EXTM3U\n# This is not a channel", want: "no EXTINF"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "playlist.m3u")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			parser := M3uParser{}
			err := parser.ParseM3u(models.Playlist{ID: 1, Name: "Test", URL: path})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
	}
}

func TestParseM3uReportsIndeterminateAcquisition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playlist.m3u")
	if err := os.WriteFile(path, []byte("#EXTM3U\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	type update struct {
		progress int
		message  string
	}
	updates := []update{}
	parser := M3uParser{Progress: func(progress int, message string) {
		updates = append(updates, update{progress: progress, message: message})
	}}
	_ = parser.ParseM3u(models.Playlist{ID: 1, Name: "Test", URL: path})

	if len(updates) < 2 {
		t.Fatalf("expected acquisition progress updates, got %#v", updates)
	}
	for _, item := range updates {
		if item.progress != 0 {
			t.Fatalf("unknown acquisition work must remain indeterminate, got %#v", updates)
		}
	}
	if !strings.Contains(updates[1].message, "Reading playlist") {
		t.Fatalf("expected the active acquisition stage, got %#v", updates)
	}
}
