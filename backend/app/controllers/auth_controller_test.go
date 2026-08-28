package controllers

import (
	"bytes"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/app/queries"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestMediaKeyOutputLinksRemainAvailableWithoutStandaloneKey(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "auth.key")
	t.Setenv("XIVI_AUTH_KEY_FILE", keyPath)
	t.Setenv("XIVI_PRODUCTION", "false")
	if err := security.GenerateKeyFile(keyPath); err != nil {
		t.Fatal(err)
	}
	if err := security.InitializeKey(); err != nil {
		t.Fatal(err)
	}

	originalSecurity := settings.APP_SETTINGS.Security
	settings.APP_SETTINGS.Security.PublicBaseURL = "https://tv.example.com"
	settings.APP_SETTINGS.Security.LocalBaseURL = "http://192.168.1.10:3000"
	settings.APP_SETTINGS.Security.AllowLANHTTP = true
	t.Cleanup(func() { settings.APP_SETTINGS.Security = originalSecurity })

	token := "xmk_persistent.example-secret"
	hash, err := security.HashToken("media", token)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := security.EncryptSecret([]byte(token))
	if err != nil {
		t.Fatal(err)
	}
	key := &models.MediaAccessKey{
		TokenPrefix: "xmk_persistent", TokenHash: hash, TokenCipher: ciphertext,
		NetworkScope: "public", LineupIDs: []int64{42},
	}
	links, err := mediaKeyOutputLinks(key)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"public_m3u", "public_xmltv", "local_m3u", "local_xmltv"} {
		if !strings.Contains(links["42"][name], "access_token="+token) {
			t.Fatalf("%s was not reconstructed with the encrypted credential: %q", name, links["42"][name])
		}
	}
	if !strings.HasPrefix(links["42"]["public_m3u"], "https://tv.example.com/") ||
		!strings.HasPrefix(links["42"]["public_xmltv"], "https://tv.example.com/") ||
		!strings.HasPrefix(links["42"]["local_m3u"], "http://192.168.1.10:3000/") ||
		!strings.HasPrefix(links["42"]["local_xmltv"], "http://192.168.1.10:3000/") {
		t.Fatalf("public and local output links did not use their configured endpoints: %#v", links["42"])
	}
	lanKey := *key
	lanKey.NetworkScope = "lan"
	lanLinks, err := mediaKeyOutputLinks(&lanKey)
	if err != nil {
		t.Fatal(err)
	}
	if lanLinks["42"]["local_m3u"] == "" || lanLinks["42"]["local_xmltv"] == "" {
		t.Fatal("LAN-scoped key did not receive local output links")
	}
	if lanLinks["42"]["public_m3u"] != "" || lanLinks["42"]["public_xmltv"] != "" {
		t.Fatal("LAN-scoped key received public links that its network policy would reject")
	}
	legacyLinks, err := mediaKeyOutputLinks(&models.MediaAccessKey{TokenPrefix: "xmk_legacy"})
	if err != nil || legacyLinks != nil {
		t.Fatalf("legacy hash-only key returned links=%v err=%v", legacyLinks, err)
	}
}

func TestReauthenticationUsesPasswordOnlyForMFAAccount(t *testing.T) {
	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_txlock=immediate")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE app_user (
		id INTEGER PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
		role TEXT NOT NULL, must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
		initial_password BOOLEAN NOT NULL DEFAULT FALSE, auth_version INTEGER NOT NULL DEFAULT 1,
		disabled_at TIMESTAMP, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	db.MustExec(`CREATE TABLE user_lineup (user_id INTEGER NOT NULL, lineup_id INTEGER NOT NULL)`)
	db.MustExec(`CREATE TABLE user_mfa (user_id INTEGER PRIMARY KEY)`)
	db.MustExec(`CREATE TABLE auth_session (
		id INTEGER PRIMARY KEY, reauthenticated_at TIMESTAMP NOT NULL,
		mfa_verified BOOLEAN NOT NULL, revoked_at TIMESTAMP)`)
	db.MustExec(`CREATE TABLE security_audit_event (
		id INTEGER PRIMARY KEY, actor_user_id INTEGER, actor_username TEXT NOT NULL DEFAULT '',
		target_user_id INTEGER, target_username TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL, outcome TEXT NOT NULL, resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL, client_ip TEXT NOT NULL, detail TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`)

	hash, err := security.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	db.MustExec(`INSERT INTO app_user (id, username, password_hash, role) VALUES (1, 'viewer', ?, 'viewer')`, hash)
	db.MustExec(`INSERT INTO user_mfa (user_id) VALUES (1)`)
	db.MustExec(`INSERT INTO auth_session (id, reauthenticated_at, mfa_verified) VALUES (7, ?, TRUE)`, time.Now().Add(-time.Hour))

	original := database.Db
	database.Db = &database.Queries{SecurityQueries: queries.NewSecurityQueries(db)}
	t.Cleanup(func() { database.Db = original })

	app := fiber.New()
	app.Post("/reauth", func(c *fiber.Ctx) error {
		c.Locals("xivi_principal", models.SessionPrincipal{UserID: 1, Username: "viewer", Role: models.RoleViewer, MFAEnabled: true})
		c.Locals("xivi_session", &models.AuthSession{ID: 7, UserID: 1, MFAVerified: true})
		return V2Reauthenticate(c)
	})
	request := httptest.NewRequest(fiber.MethodPost, "/reauth", bytes.NewBufferString(`{"password":"correct horse battery staple","mfa_code":"invalid-and-ignored"}`))
	request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	response, err := app.Test(request, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusNoContent {
		t.Fatalf("password-only reauthentication returned %d", response.StatusCode)
	}
	var audit struct {
		ActorUsername  string `db:"actor_username"`
		TargetUsername string `db:"target_username"`
	}
	if err := db.Get(&audit, `SELECT actor_username, target_username FROM security_audit_event
		WHERE action = 'reauthenticate' AND outcome = 'success'`); err != nil {
		t.Fatalf("successful reauthentication was not audited: %v", err)
	}
	if audit.ActorUsername != "viewer" || audit.TargetUsername != "viewer" {
		t.Fatalf("reauthentication audit did not identify the user: %+v", audit)
	}
}

func TestLogoutRevokesServerSessionAndExpiresBrowserCookie(t *testing.T) {
	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_txlock=immediate")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE auth_session (
		id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, revoked_at TIMESTAMP NULL)`)
	db.MustExec(`CREATE TABLE security_audit_event (
		id INTEGER PRIMARY KEY, actor_user_id INTEGER, actor_username TEXT NOT NULL DEFAULT '',
		target_user_id INTEGER, target_username TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL, outcome TEXT NOT NULL, resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL, client_ip TEXT NOT NULL, detail TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO auth_session (id, user_id) VALUES (19, 7)`)

	original := database.Db
	database.Db = &database.Queries{SecurityQueries: queries.NewSecurityQueries(db)}
	t.Cleanup(func() { database.Db = original })

	app := fiber.New()
	app.Post("/logout", func(c *fiber.Ctx) error {
		c.Locals("xivi_principal", models.SessionPrincipal{UserID: 7, Username: "viewer", Role: models.RoleViewer})
		c.Locals("xivi_session", &models.AuthSession{ID: 19, UserID: 7, TransportScope: "lan_http"})
		return V2Logout(c)
	})
	response, err := app.Test(httptest.NewRequest(fiber.MethodPost, "/logout", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusNoContent {
		t.Fatalf("logout returned %d", response.StatusCode)
	}
	var revokedAt *time.Time
	if err := db.Get(&revokedAt, `SELECT revoked_at FROM auth_session WHERE id = 19`); err != nil || revokedAt == nil {
		t.Fatalf("server session was not revoked: revoked_at=%v err=%v", revokedAt, err)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != middleware.LANSessionCookie || cookies[0].Value != "" || cookies[0].MaxAge >= 0 {
		t.Fatalf("browser session cookie was not expired: %#v", cookies)
	}
}
