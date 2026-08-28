package utils

import (
	"net/http"
	"net/http/httptest"
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
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(test.content)) }))
			defer server.Close()
			parser := M3uParser{httpClient: server.Client()}
			err := parser.ParseM3u(models.Playlist{ID: 1, Name: "Test", URL: server.URL})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
		})
	}
}

func TestParseM3uReportsIndeterminateAcquisition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte("#EXTM3U\n")) }))
	defer server.Close()

	type update struct {
		progress int
		message  string
	}
	updates := []update{}
	parser := M3uParser{httpClient: server.Client(), Progress: func(progress int, message string) {
		updates = append(updates, update{progress: progress, message: message})
	}}
	_ = parser.ParseM3u(models.Playlist{ID: 1, Name: "Test", URL: server.URL})

	if len(updates) < 2 {
		t.Fatalf("expected acquisition progress updates, got %#v", updates)
	}
	for _, item := range updates {
		if item.progress != 0 {
			t.Fatalf("unknown acquisition work must remain indeterminate, got %#v", updates)
		}
	}
	if !strings.Contains(updates[1].message, "Downloading playlist") {
		t.Fatalf("expected the active acquisition stage, got %#v", updates)
	}
}
