package middleware

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/app/queries"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestSessionCookieScopesAndLifetimes(t *testing.T) {
	app := fiber.New()
	app.Get("/:scope", func(c *fiber.Ctx) error {
		SetSessionCookie(c, c.Params("scope"), "opaque", time.Now().Add(time.Hour))
		return c.SendStatus(fiber.StatusNoContent)
	})
	app.Delete("/:scope", func(c *fiber.Ctx) error {
		ClearSessionCookie(c, c.Params("scope"))
		return c.SendStatus(fiber.StatusNoContent)
	})
	for _, test := range []struct {
		path       string
		name       string
		wantSecure bool
	}{
		{path: "/https", name: PublicSessionCookie, wantSecure: true},
		{path: "/lan_http", name: LANSessionCookie, wantSecure: false},
	} {
		response, err := app.Test(httptest.NewRequest(fiber.MethodGet, test.path, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		cookies := response.Cookies()
		if len(cookies) != 1 || cookies[0].Name != test.name || !cookies[0].HttpOnly || cookies[0].Secure != test.wantSecure || cookies[0].SameSite != 3 || cookies[0].Path != "/" || strings.Contains(strings.ToLower(response.Header.Get("Set-Cookie")), "domain=") {
			t.Fatalf("unexpected %s cookie: %#v", test.path, cookies)
		}
		_ = response.Body.Close()

		response, err = app.Test(httptest.NewRequest(fiber.MethodDelete, test.path, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		cookies = response.Cookies()
		if len(cookies) != 1 || cookies[0].Name != test.name || cookies[0].Value != "" || cookies[0].MaxAge >= 0 || cookies[0].Expires.After(time.Now()) {
			t.Fatalf("%s cookie was not explicitly expired: %#v", test.path, cookies)
		}
		_ = response.Body.Close()
	}

	if idle, absolute := SessionDurations(models.RoleAdmin, "https"); idle != 30*time.Minute || absolute != 12*time.Hour {
		t.Fatalf("unexpected public admin duration: %s/%s", idle, absolute)
	}
	if idle, absolute := SessionDurations(models.RoleViewer, "lan_http"); idle != 2*time.Hour || absolute != 24*time.Hour {
		t.Fatalf("unexpected LAN viewer duration: %s/%s", idle, absolute)
	}
}

func TestTrustedBrowserCookieScopes(t *testing.T) {
	app := fiber.New()
	app.Get("/:scope", func(c *fiber.Ctx) error {
		SetTrustedBrowserCookie(c, c.Params("scope"), "opaque", time.Now().Add(30*24*time.Hour))
		return c.SendStatus(fiber.StatusNoContent)
	})
	app.Delete("/:scope", func(c *fiber.Ctx) error {
		ClearTrustedBrowserCookie(c, c.Params("scope"))
		return c.SendStatus(fiber.StatusNoContent)
	})
	for _, test := range []struct {
		path       string
		name       string
		wantSecure bool
	}{
		{path: "/https", name: PublicTrustedBrowserCookie, wantSecure: true},
		{path: "/lan_http", name: LANTrustedBrowserCookie, wantSecure: false},
	} {
		response, err := app.Test(httptest.NewRequest(fiber.MethodGet, test.path, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		cookies := response.Cookies()
		if len(cookies) != 1 || cookies[0].Name != test.name || !cookies[0].HttpOnly || cookies[0].Secure != test.wantSecure || cookies[0].SameSite != 3 || cookies[0].Path != "/" {
			t.Fatalf("unexpected trusted-browser cookie: %#v", cookies)
		}
		_ = response.Body.Close()

		response, err = app.Test(httptest.NewRequest(fiber.MethodDelete, test.path, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		cookies = response.Cookies()
		if len(cookies) != 1 || cookies[0].Name != test.name || cookies[0].MaxAge >= 0 {
			t.Fatalf("trusted-browser cookie was not expired: %#v", cookies)
		}
		_ = response.Body.Close()
	}
}

func TestAnonymousSessionMiddlewareContinuesToTheRoute(t *testing.T) {
	app := fiber.New()
	app.Use(AuthenticateSession)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/health", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusOK || response.ContentLength == 0 {
		t.Fatalf("anonymous route returned status=%d content_length=%d", response.StatusCode, response.ContentLength)
	}
}

func TestSessionMiddlewareRejectsRevocationAndExpiry(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "auth.key")
	t.Setenv("XIVI_AUTH_KEY_FILE", keyPath)
	t.Setenv("XIVI_PRODUCTION", "false")
	if err := security.GenerateKeyFile(keyPath); err != nil {
		t.Fatal(err)
	}
	if err := security.InitializeKey(); err != nil {
		t.Fatal(err)
	}
	originalSecurity := settings.Current().Security
	settings.Current().Security.AllowLANHTTP = true
	settings.Current().Security.TrustedLANCIDRs = []string{"0.0.0.0/0"}
	settings.Current().Security.LocalBaseURL = "http://xivi.test"
	t.Cleanup(func() { settings.Current().Security = originalSecurity })

	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_txlock=immediate")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE app_user (
		id INTEGER PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL, must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
		initial_password BOOLEAN NOT NULL DEFAULT FALSE, auth_version INTEGER NOT NULL DEFAULT 1,
		disabled_at TIMESTAMP, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`)
	db.MustExec(`CREATE TABLE user_mfa (user_id INTEGER PRIMARY KEY)`)
	db.MustExec(`CREATE TABLE user_lineup (user_id INTEGER NOT NULL, lineup_id INTEGER NOT NULL)`)
	db.MustExec(`CREATE TABLE auth_session (
		id INTEGER PRIMARY KEY, token_hash BLOB NOT NULL UNIQUE, user_id INTEGER NOT NULL,
		auth_version INTEGER NOT NULL, transport_scope TEXT NOT NULL, mfa_verified BOOLEAN NOT NULL,
		reauthenticated_at TIMESTAMP NOT NULL, created_at TIMESTAMP NOT NULL,
		last_seen_at TIMESTAMP NOT NULL, idle_expires_at TIMESTAMP NOT NULL,
		absolute_expires_at TIMESTAMP NOT NULL, revoked_at TIMESTAMP NULL,
		client_ip TEXT NOT NULL, user_agent_hash BLOB)`)

	const token = "test-public-browser-session"
	tokenHash, err := security.HashToken("session", token)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	idleExpiry := now.Add(time.Hour)
	db.MustExec(`INSERT INTO app_user
		(id, username, password_hash, role, created_at, updated_at)
		VALUES (7, 'admin', 'unused', 'admin', ?, ?)`, now, now)
	db.MustExec(`INSERT INTO auth_session
		(id, token_hash, user_id, auth_version, transport_scope, mfa_verified,
		 reauthenticated_at, created_at, last_seen_at, idle_expires_at,
		 absolute_expires_at, client_ip)
		VALUES (19, ?, 7, 1, 'lan_http', TRUE, ?, ?, ?, ?, ?, '192.0.2.1')`,
		tokenHash, now, now, now.Add(-2*time.Minute), idleExpiry, now.Add(2*time.Hour))

	originalDB := database.Db
	database.Db = &database.Queries{SecurityQueries: queries.NewSecurityQueries(db)}
	t.Cleanup(func() { database.Db = originalDB })

	app := fiber.New()
	app.Use(AuthenticateSession)
	app.Get("/protected", RequireAuthenticated(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	request := func() *http.Request {
		req := httptest.NewRequest(fiber.MethodGet, "http://xivi.test/protected", nil)
		req.AddCookie(&http.Cookie{Name: LANSessionCookie, Value: token, Path: "/"})
		return req
	}

	passiveRequest := request()
	passiveRequest.Header.Set(passiveCheckHeader, "passive")
	response, err := app.Test(passiveRequest, -1)
	if err != nil || response.StatusCode != fiber.StatusNoContent {
		t.Fatalf("valid session returned status %d, err %v", response.StatusCode, err)
	}
	_ = response.Body.Close()
	var storedIdleExpiry time.Time
	if err := db.Get(&storedIdleExpiry, `SELECT idle_expires_at FROM auth_session WHERE id = 19`); err != nil || !storedIdleExpiry.Equal(idleExpiry) {
		t.Fatalf("passive validation extended idle expiry: got=%v want=%v err=%v", storedIdleExpiry, idleExpiry, err)
	}

	db.MustExec(`UPDATE auth_session SET revoked_at = ? WHERE id = 19`, now)
	response, err = app.Test(request(), -1)
	if err != nil || response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("revoked session returned status %d, err %v", response.StatusCode, err)
	}
	if cookies := response.Cookies(); len(cookies) != 1 || cookies[0].Name != LANSessionCookie || cookies[0].MaxAge >= 0 {
		t.Fatalf("revoked session cookie was not expired: %#v", cookies)
	}
	_ = response.Body.Close()

	db.MustExec(`UPDATE auth_session SET revoked_at = NULL, idle_expires_at = ? WHERE id = 19`, now.Add(-time.Second))
	response, err = app.Test(request(), -1)
	if err != nil || response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expired session returned status %d, err %v", response.StatusCode, err)
	}
	var revokedAt *time.Time
	if err := db.Get(&revokedAt, `SELECT revoked_at FROM auth_session WHERE id = 19`); err != nil || revokedAt == nil {
		t.Fatalf("expired session was not revoked: revoked_at=%v err=%v", revokedAt, err)
	}
	_ = response.Body.Close()
}

func TestMediaKeyNetworkScopes(t *testing.T) {
	original := settings.Current().Security.AllowLANHTTP
	settings.Current().Security.AllowLANHTTP = true
	t.Cleanup(func() { settings.Current().Security.AllowLANHTTP = original })
	https := security.RequestNetwork{Scheme: "https", IP: net.ParseIP("203.0.113.10")}
	trustedLAN := security.RequestNetwork{Scheme: "http", IP: net.ParseIP("192.168.1.20"), DirectTrustedLAN: true}
	trustedLANTLS := security.RequestNetwork{Scheme: "https", IP: net.ParseIP("192.168.1.20"), DirectTrustedLAN: true}
	untrustedHTTP := security.RequestNetwork{Scheme: "http", IP: net.ParseIP("203.0.113.10")}

	if !mediaKeyNetworkAllowed("public", https) || !mediaKeyNetworkAllowed("public", trustedLAN) {
		t.Fatal("public keys should work over HTTPS and direct trusted LAN requests")
	}
	if mediaKeyNetworkAllowed("public", untrustedHTTP) {
		t.Fatal("public keys must not work over untrusted cleartext HTTP")
	}
	if !mediaKeyNetworkAllowed("lan", trustedLAN) || mediaKeyNetworkAllowed("lan", https) {
		t.Fatal("LAN keys must be restricted to direct trusted LAN requests")
	}
	if mediaKeyNetworkAllowed("unknown", trustedLAN) {
		t.Fatal("unknown key scopes must fail closed")
	}
	settings.Current().Security.AllowLANHTTP = false
	if mediaKeyNetworkAllowed("lan", trustedLAN) || mediaKeyNetworkAllowed("public", trustedLAN) {
		t.Fatal("cleartext LAN keys must honor the disabled LAN HTTP exception")
	}
	if !mediaKeyNetworkAllowed("lan", trustedLANTLS) || !mediaKeyNetworkAllowed("public", trustedLANTLS) {
		t.Fatal("direct trusted-LAN HTTPS should remain available")
	}
}

func TestShortMediaOutputAliasAuthenticatesOneLineupAndHonorsRevocation(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "auth.key")
	t.Setenv("XIVI_AUTH_KEY_FILE", keyPath)
	t.Setenv("XIVI_PRODUCTION", "false")
	if err := security.GenerateKeyFile(keyPath); err != nil {
		t.Fatal(err)
	}
	if err := security.InitializeKey(); err != nil {
		t.Fatal(err)
	}
	security.SetInitialSetupRequired(false)
	originalSecurity := settings.Current().Security
	settings.Current().Security.AllowLANHTTP = true
	settings.Current().Security.TrustedLANCIDRs = []string{"0.0.0.0/0", "::/0"}
	settings.Current().Security.LocalBaseURL = "http://xivi.test"
	t.Cleanup(func() { settings.Current().Security = originalSecurity })

	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_txlock=immediate")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE app_user (
		id INTEGER PRIMARY KEY, username TEXT NOT NULL, role TEXT NOT NULL, disabled_at TIMESTAMP NULL)`)
	db.MustExec(`CREATE TABLE user_lineup (user_id INTEGER NOT NULL, lineup_id INTEGER NOT NULL)`)
	db.MustExec(`CREATE TABLE media_access_key (
		id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, name TEXT NOT NULL,
		token_prefix TEXT NOT NULL UNIQUE, token_hash BLOB NOT NULL UNIQUE, token_cipher BLOB,
		network_scope TEXT NOT NULL, created_at TIMESTAMP NOT NULL, expires_at TIMESTAMP NULL,
		last_used_at TIMESTAMP NULL, last_used_ip TEXT NOT NULL DEFAULT '', revoked_at TIMESTAMP NULL)`)
	db.MustExec(`CREATE TABLE media_key_lineup (media_key_id INTEGER NOT NULL, lineup_id INTEGER NOT NULL)`)
	db.MustExec(`CREATE TABLE media_output_alias (
		id INTEGER PRIMARY KEY, media_key_id INTEGER NOT NULL, lineup_id INTEGER NOT NULL,
		code_hash BLOB NOT NULL UNIQUE, code_cipher BLOB NOT NULL, created_at TIMESTAMP NOT NULL)`)

	const token = "xmk_internal.full-secret"
	tokenHash, err := security.HashToken("media", token)
	if err != nil {
		t.Fatal(err)
	}
	tokenCipher, err := security.EncryptSecret([]byte(token))
	if err != nil {
		t.Fatal(err)
	}
	compact, ok := security.NormalizeMediaOutputCode("ABCD-EFGH-JKMN-PQRS")
	if !ok {
		t.Fatal("test output code is invalid")
	}
	aliasHash, err := security.HashToken("media-output-alias", compact)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	db.MustExec(`INSERT INTO app_user(id, username, role) VALUES (1, 'viewer', 'viewer')`)
	db.MustExec(`INSERT INTO user_lineup(user_id, lineup_id) VALUES (1, 7)`)
	db.MustExec(`INSERT INTO media_access_key
		(id, user_id, name, token_prefix, token_hash, token_cipher, network_scope, created_at)
		VALUES (2, 1, 'Living room', 'xmk_internal', ?, ?, 'public', ?)`, tokenHash, tokenCipher, now)
	db.MustExec(`INSERT INTO media_key_lineup(media_key_id, lineup_id) VALUES (2, 7)`)
	db.MustExec(`INSERT INTO media_output_alias
		(id, media_key_id, lineup_id, code_hash, code_cipher, created_at)
		VALUES (3, 2, 7, ?, X'01', ?)`, aliasHash, now)

	originalDB := database.Db
	database.Db = &database.Queries{SecurityQueries: queries.NewSecurityQueries(db)}
	t.Cleanup(func() { database.Db = originalDB })
	app := fiber.New()
	app.Get("/m/:code", RequireMediaOutputAlias(), func(c *fiber.Ctx) error {
		credential, found := CurrentMediaCredential(c)
		if !found || credential.LineupID != 7 || credential.Token != token || credential.OutputCode != "ABCD-EFGH-JKMN-PQRS" {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	for _, path := range []string{"/m/ABCD-EFGH-JKMN-PQRS", "/m/abcdefghjkmnpqrs"} {
		request := httptest.NewRequest(fiber.MethodGet, "https://xivi.test"+path, nil)
		request.TLS = &tls.ConnectionState{}
		response, err := app.Test(request, -1)
		if err != nil || response.StatusCode != fiber.StatusNoContent {
			t.Fatalf("short alias %s returned status %d, err %v", path, response.StatusCode, err)
		}
		_ = response.Body.Close()
	}
	db.MustExec(`UPDATE media_access_key SET revoked_at = ? WHERE id = 2`, now)
	request := httptest.NewRequest(fiber.MethodGet, "https://xivi.test/m/ABCD-EFGH-JKMN-PQRS", nil)
	request.TLS = &tls.ConnectionState{}
	response, err := app.Test(request, -1)
	if err != nil || response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("revoked short alias returned status %d, err %v", response.StatusCode, err)
	}
	_ = response.Body.Close()
}

func TestCSRFRequiresSessionTokenAndSameOrigin(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "auth.key")
	t.Setenv("XIVI_AUTH_KEY_FILE", keyPath)
	t.Setenv("XIVI_PRODUCTION", "false")
	if err := security.GenerateKeyFile(keyPath); err != nil {
		t.Fatal(err)
	}
	if err := security.InitializeKey(); err != nil {
		t.Fatal(err)
	}
	original := settings.Current().Security
	settings.Current().Security.LocalBaseURL = "http://xivi.test"
	t.Cleanup(func() { settings.Current().Security = original })

	const sessionToken = "test-session-token"
	csrf, err := security.CSRFToken(sessionToken)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Post("/change", func(c *fiber.Ctx) error {
		c.Locals(principalLocal, models.SessionPrincipal{Role: models.RoleViewer})
		c.Locals(sessionTokenLocal, sessionToken)
		return c.Next()
	}, CSRFProtected(), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	request := httptest.NewRequest(fiber.MethodPost, "http://xivi.test/change", nil)
	request.Header.Set("Origin", "http://xivi.test")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := app.Test(request, -1)
	if err != nil || response.StatusCode != fiber.StatusNoContent {
		t.Fatalf("valid CSRF request returned status %d, err %v", response.StatusCode, err)
	}
	_ = response.Body.Close()

	mobileApp := fiber.New()
	mobileApp.Post("/change", func(c *fiber.Ctx) error {
		c.Locals(principalLocal, models.SessionPrincipal{Role: models.RoleViewer})
		c.Locals(mobileSessionLocal, &models.MobileSession{ID: 22, UserID: 7})
		return c.Next()
	}, CSRFProtected(), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	mobileResponse, mobileErr := mobileApp.Test(httptest.NewRequest(fiber.MethodPost, "https://xivi.test/change", nil), -1)
	if mobileErr != nil || mobileResponse.StatusCode != fiber.StatusNoContent {
		t.Fatalf("authenticated mobile bearer mutation returned status %d, err %v", mobileResponse.StatusCode, mobileErr)
	}
	_ = mobileResponse.Body.Close()

	for _, candidate := range []struct {
		name   string
		origin string
		csrf   string
		site   string
	}{
		{name: "missing token", origin: "http://xivi.test", site: "same-origin"},
		{name: "foreign origin", origin: "https://attacker.example", csrf: csrf, site: "cross-site"},
	} {
		request = httptest.NewRequest(fiber.MethodPost, "http://xivi.test/change", nil)
		request.Header.Set("Origin", candidate.origin)
		request.Header.Set("Sec-Fetch-Site", candidate.site)
		request.Header.Set("X-CSRF-Token", candidate.csrf)
		response, err = app.Test(request, -1)
		if err != nil || response.StatusCode != fiber.StatusForbidden {
			t.Fatalf("%s returned status %d, err %v", candidate.name, response.StatusCode, err)
		}
		_ = response.Body.Close()
	}

	settings.Current().Security.LocalBaseURL = "http://127.0.0.1:5173"
	settings.Current().Security.TrustedLANCIDRs = []string{"0.0.0.0/0", "::/0"}
	request = httptest.NewRequest(fiber.MethodPost, "http://localhost:5173/change", nil)
	request.RemoteAddr = "127.0.0.1:41000"
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = app.Test(request, -1)
	if err != nil || response.StatusCode != fiber.StatusNoContent {
		t.Fatalf("same-origin loopback alias returned status %d, err %v", response.StatusCode, err)
	}
	_ = response.Body.Close()

	request = httptest.NewRequest(fiber.MethodPost, "http://attacker.example/change", nil)
	request.RemoteAddr = "127.0.0.1:41000"
	request.Header.Set("Origin", "http://attacker.example")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = app.Test(request, -1)
	if err != nil || response.StatusCode != fiber.StatusForbidden {
		t.Fatalf("unconfigured direct hostname returned status %d, err %v", response.StatusCode, err)
	}
	_ = response.Body.Close()

	settings.Current().Security.PublicBaseURL = "https://tv.example.com"
	settings.Current().Security.TrustedProxyCIDRs = []string{"0.0.0.0/0", "::/0"}
	request = httptest.NewRequest(fiber.MethodPost, "http://backend.internal/change", nil)
	request.Header.Set("Origin", "https://tv.example.com")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = app.Test(request, -1)
	if err != nil || response.StatusCode != fiber.StatusNoContent {
		t.Fatalf("configured public proxy origin returned status %d, err %v", response.StatusCode, err)
	}
	_ = response.Body.Close()

	request = httptest.NewRequest(fiber.MethodPost, "http://backend.internal/change", nil)
	request.Header.Set("Origin", "https://other.example.com")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err = app.Test(request, -1)
	if err != nil || response.StatusCode != fiber.StatusForbidden {
		t.Fatalf("unconfigured public proxy origin returned status %d, err %v", response.StatusCode, err)
	}
	_ = response.Body.Close()
}

func TestMobileBearerSurfaceAllowlist(t *testing.T) {
	for _, path := range []string{
		"/api/v2/mobile/auth/logout",
		"/api/v2/auth/session",
		"/api/v2/watch/lineups",
		"/api/v2/search",
		"/api/v2/stream/telemetry",
		"/stream/hls-audio/channel",
		"/images/channel.png",
	} {
		if !mobileBearerPathAllowed(path) {
			t.Errorf("expected mobile bearer access to %s", path)
		}
	}
	for _, path := range []string{
		"/api/v2/studio/overview",
		"/api/v2/account/media-keys",
		"/api/v2/auth/mfa/enroll",
		"/docs/openapi.yaml",
	} {
		if mobileBearerPathAllowed(path) {
			t.Errorf("mobile bearer unexpectedly allowed on %s", path)
		}
	}
}

func TestRoleAndPasswordGuards(t *testing.T) {
	app := fiber.New()
	app.Get("/admin", func(c *fiber.Ctx) error {
		c.Locals(principalLocal, models.SessionPrincipal{Role: models.RoleViewer})
		return c.Next()
	}, RequireAdmin(), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Get("/temporary", func(c *fiber.Ctx) error {
		c.Locals(principalLocal, models.SessionPrincipal{Role: models.RoleViewer, MustChangePassword: true})
		return c.Next()
	}, RequirePasswordChanged(), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Get("/unverified-mfa", func(c *fiber.Ctx) error {
		c.Locals(principalLocal, models.SessionPrincipal{Role: models.RoleViewer, MFARequired: true})
		return c.Next()
	}, RequireAuthenticated(), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Get("/stream/:stream_id", func(c *fiber.Ctx) error {
		c.Locals(principalLocal, models.SessionPrincipal{Role: models.RoleAdmin, MustChangePassword: true})
		return c.Next()
	}, RequirePlayback(), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Get("/images/:asset", func(c *fiber.Ctx) error {
		c.Locals(principalLocal, models.SessionPrincipal{Role: models.RoleAdmin, MustChangePassword: true})
		return c.Next()
	}, RequireImageAccess(), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	for path, want := range map[string]int{
		"/admin":           fiber.StatusForbidden,
		"/temporary":       fiber.StatusForbidden,
		"/unverified-mfa":  fiber.StatusUnauthorized,
		"/stream/channel":  fiber.StatusForbidden,
		"/images/logo.png": fiber.StatusForbidden,
	} {
		response, err := app.Test(httptest.NewRequest(fiber.MethodGet, path, nil), -1)
		if err != nil || response.StatusCode != want {
			t.Fatalf("%s returned status %d, err %v; want %d", path, response.StatusCode, err, want)
		}
		_ = response.Body.Close()
	}
}

func TestMediaCredentialsRemainLockedDuringInitialPasswordChange(t *testing.T) {
	security.SetInitialSetupRequired(true)
	t.Cleanup(func() { security.SetInitialSetupRequired(false) })

	app := fiber.New()
	app.Get("/media", func(c *fiber.Ctx) error {
		_, err := authenticateMediaToken(c)
		if err != nil {
			return securityError(c, fiber.StatusUnauthorized, "invalid_media_key", "The media credential was invalid.")
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/media?access_token=xmk_known.value", nil), -1)
	if err != nil || response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("initial setup media request returned status %d, err %v", response.StatusCode, err)
	}
	_ = response.Body.Close()
}
