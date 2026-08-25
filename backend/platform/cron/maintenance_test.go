package cron

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"xivi/backend/platform/settings"
)

func TestPruneTemporaryFilesRemovesOnlyExpiredTempArtifacts(t *testing.T) {
	root := t.TempDir()
	originalM3U, originalEPG, originalLogo := settings.M3U_FILEPATH, settings.EPG_FILEPATH, settings.LOGO_FILEPATH
	settings.M3U_FILEPATH = filepath.Join(root, "m3u")
	settings.EPG_FILEPATH = filepath.Join(root, "epg")
	settings.LOGO_FILEPATH = filepath.Join(root, "logo")
	t.Cleanup(func() {
		settings.M3U_FILEPATH, settings.EPG_FILEPATH, settings.LOGO_FILEPATH = originalM3U, originalEPG, originalLogo
	})
	for _, directory := range []string{settings.M3U_FILEPATH, settings.EPG_FILEPATH, settings.LOGO_FILEPATH} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	oldTemp := filepath.Join(settings.M3U_FILEPATH, ".xivi-old.m3u.tmp")
	recentTemp := filepath.Join(settings.EPG_FILEPATH, ".xivi-recent.xml.tmp")
	publishedFile := filepath.Join(settings.LOGO_FILEPATH, "channel.png")
	for _, path := range []string{oldTemp, recentTemp, publishedFile} {
		if err := os.WriteFile(path, []byte("artifact"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldTemp, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	report, err := pruneTemporaryFiles(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if report.FilesRemoved != 1 || report.BytesReclaimed != int64(len("artifact")) {
		t.Fatalf("unexpected temporary cleanup report: %+v", report)
	}
	if _, err := os.Stat(oldTemp); !os.IsNotExist(err) {
		t.Fatalf("expired temporary file still exists: %v", err)
	}
	for _, path := range []string{recentTemp, publishedFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("maintenance removed a retained file %s: %v", path, err)
		}
	}
}
