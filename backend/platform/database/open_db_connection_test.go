package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/settings"
)

func TestEverySQLiteConnectionUsesSecurityPragmas(t *testing.T) {
	original := settings.CONFIG_PATH
	settings.CONFIG_PATH = filepath.Join(t.TempDir(), "config")
	t.Cleanup(func() { settings.CONFIG_PATH = original })
	if err := os.MkdirAll(settings.CONFIG_PATH, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := getDB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for name, want := range map[string]int{"trusted_schema": 0, "secure_delete": 2} {
		var value int
		if err := db.Get(&value, "PRAGMA "+name); err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if value != want {
			t.Fatalf("%s = %d, want %d", name, value, want)
		}
	}
}

func TestOpenDatabaseRemainsUsableAfterMigrations(t *testing.T) {
	originalPath := settings.CONFIG_PATH
	settings.CONFIG_PATH = filepath.Join(t.TempDir(), "config")
	t.Cleanup(func() { settings.CONFIG_PATH = originalPath })
	if err := os.MkdirAll(settings.CONFIG_PATH, 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(settings.CONFIG_PATH, "auth.key")
	if err := security.GenerateKeyFile(keyPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XIVI_AUTH_KEY_FILE", keyPath)
	t.Setenv("XIVI_PRODUCTION", "false")
	if err := security.InitializeKey(); err != nil {
		t.Fatal(err)
	}

	opened, err := OpenDBConnection()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDBConnection() })
	if _, err := opened.UserCount(context.Background()); err != nil {
		t.Fatalf("database pool was not usable after migrations: %v", err)
	}
}
