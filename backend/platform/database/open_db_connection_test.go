package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
	"xivi/backend/app/models"
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
	var aliasTable string
	if err := opened.SecurityQueries.GetContext(context.Background(), &aliasTable,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'media_output_alias'`); err != nil || aliasTable != "media_output_alias" {
		t.Fatalf("short media output alias migration was not applied: table=%q err=%v", aliasTable, err)
	}
	ctx := context.Background()
	userID, err := opened.CreateUser(ctx, "alias-migration-owner", "", "hash", "viewer", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.SecurityQueries.ExecContext(ctx, `INSERT INTO template(id, name) VALUES (700, 'Migration lineup')`); err != nil {
		t.Fatal(err)
	}
	key := &models.MediaAccessKey{UserID: userID, Name: "Migration device", TokenPrefix: "migration-prefix",
		TokenHash: []byte("migration-hash"), TokenCipher: []byte("migration-cipher"), NetworkScope: "lan",
		CreatedAt: time.Now().UTC(), OutputAliases: []models.MediaOutputAlias{{LineupID: 700,
			CodeHash: []byte("migration-alias-hash"), CodeCipher: []byte("migration-alias-cipher"), CreatedAt: time.Now().UTC()}}}
	if err := opened.CreateMediaKey(ctx, key, []int64{700}); err != nil {
		t.Fatalf("short alias could not be inserted into migrated schema: %v", err)
	}
	aliases, err := opened.ListMediaOutputAliases(ctx, key.ID)
	if err != nil || len(aliases) != 1 || aliases[0].LineupID != 700 {
		t.Fatalf("short alias could not be read from migrated schema: aliases=%+v err=%v", aliases, err)
	}
}
