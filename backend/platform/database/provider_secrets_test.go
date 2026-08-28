package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"xivi/backend/pkg/security"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestEncryptExistingProviderURLsRemovesPlaintextProviderSecrets(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "auth.key")
	if err := security.GenerateKeyFile(keyPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XIVI_AUTH_KEY_FILE", keyPath)
	t.Setenv("XIVI_PRODUCTION", "false")
	if err := security.InitializeKey(); err != nil {
		t.Fatal(err)
	}

	db := sqlx.MustOpen("sqlite3", "file:provider-secrets?mode=memory&cache=shared")
	t.Cleanup(func() { _ = db.Close() })
	for _, statement := range []string{
		`CREATE TABLE playlist (id INTEGER PRIMARY KEY, url TEXT, url_cipher BLOB)`,
		`CREATE TABLE epg (id INTEGER PRIMARY KEY, url TEXT, url_cipher BLOB)`,
		`CREATE TABLE channelurl (id INTEGER PRIMARY KEY, url TEXT, url_cipher BLOB)`,
		`CREATE TABLE playlistchannel (id INTEGER PRIMARY KEY, tvg_logo TEXT)`,
		`CREATE TABLE epgchannel (id INTEGER PRIMARY KEY, "icon.src" TEXT)`,
		`CREATE TABLE epgprogramme (id INTEGER PRIMARY KEY, "icon.src" TEXT)`,
		`CREATE TABLE user_mfa (user_id INTEGER PRIMARY KEY, encrypted_secret BLOB)`,
	} {
		db.MustExec(statement)
	}
	db.MustExec(`INSERT INTO playlist VALUES (1, 'https://provider.example/list.m3u?token=secret', NULL)`)
	db.MustExec(`INSERT INTO epg VALUES (2, 'https://provider.example/guide.xml?token=secret', NULL)`)
	db.MustExec(`INSERT INTO channelurl VALUES (3, 'https://provider.example/live/3?token=secret', NULL)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES (4, 'https://provider.example/logo.png?token=secret')`)
	db.MustExec(`INSERT INTO epgchannel VALUES (5, 'https://provider.example/channel.png?token=secret')`)
	db.MustExec(`INSERT INTO epgprogramme VALUES (6, 'https://provider.example/programme.png?token=secret')`)

	if err := encryptExistingProviderURLs(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"playlist", "epg", "channelurl"} {
		var row encryptedProviderURLRow
		if err := db.Get(&row, `SELECT id, url, url_cipher FROM `+table); err != nil {
			t.Fatal(err)
		}
		if row.URL != "encrypted" || len(row.Cipher) == 0 {
			t.Fatalf("%s provider URL was not converted to encrypted storage", table)
		}
		plaintext, err := security.DecryptSecret(row.Cipher)
		if err != nil || !strings.Contains(string(plaintext), "token=secret") {
			t.Fatalf("%s encrypted value did not round-trip: %q, %v", table, plaintext, err)
		}
	}

	var protectedLogo string
	if err := db.Get(&protectedLogo, `SELECT tvg_logo FROM playlistchannel WHERE id = 4`); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(protectedLogo, "provider.example") {
		t.Fatal("provider logo remained plaintext")
	}
	revealedLogo, err := security.RevealString(protectedLogo)
	if err != nil || revealedLogo != "https://provider.example/logo.png?token=secret" {
		t.Fatalf("protected logo did not round-trip: %q, %v", revealedLogo, err)
	}

	for _, table := range []string{"epgchannel", "epgprogramme"} {
		var icon string
		if err := db.Get(&icon, `SELECT "icon.src" FROM `+table); err != nil {
			t.Fatal(err)
		}
		if icon != "" {
			t.Fatalf("%s remote artwork was retained", table)
		}
	}

	// Re-running startup verification is idempotent and validates ciphertext.
	if err := encryptExistingProviderURLs(context.Background(), db); err != nil {
		t.Fatalf("second provider-secret pass failed: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("authentication key disappeared: %v", err)
	}
}
