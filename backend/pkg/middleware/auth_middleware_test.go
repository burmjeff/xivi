package middleware

import (
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
	originalSecurity := settings.APP_SETTINGS.Security
	settings.APP_SETTINGS.Security.AllowLANHTTP = true
	settings.APP_SETTINGS.Security.TrustedLANCIDRs = []string{"0.0.0.0/0"}
	settings.APP_SETTINGS.Security.LocalBaseURL = "http://xivi.test"
	t.Cleanup(func() { settings.APP_SETTINGS.Security = originalSecurity })

	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_txlock=immediate")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE app_user (
		id INTEGER PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
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
	original := settings.APP_SETTINGS.Security.AllowLANHTTP
	settings.APP_SETTINGS.Security.AllowLANHTTP = true
	t.Cleanup(func() { settings.APP_SETTINGS.Security.AllowLANHTTP = original })
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
	settings.APP_SETTINGS.Security.AllowLANHTTP = false
	if mediaKeyNetworkAllowed("lan", trustedLAN) || mediaKeyNetworkAllowed("public", trustedLAN) {
		t.Fatal("cleartext LAN keys must honor the disabled LAN HTTP exception")
	}
	if !mediaKeyNetworkAllowed("lan", trustedLANTLS) || !mediaKeyNetworkAllowed("public", trustedLANTLS) {
		t.Fatal("direct trusted-LAN HTTPS should remain available")
	}
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
	original := settings.APP_SETTINGS.Security
	settings.APP_SETTINGS.Security.LocalBaseURL = "http://xivi.test"
	t.Cleanup(func() { settings.APP_SETTINGS.Security = original })

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

	settings.APP_SETTINGS.Security.LocalBaseURL = "http://127.0.0.1:5173"
	settings.APP_SETTINGS.Security.TrustedLANCIDRs = []string{"0.0.0.0/0", "::/0"}
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

	settings.APP_SETTINGS.Security.PublicBaseURL = "https://tv.example.com"
	settings.APP_SETTINGS.Security.TrustedProxyCIDRs = []string{"0.0.0.0/0", "::/0"}
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
