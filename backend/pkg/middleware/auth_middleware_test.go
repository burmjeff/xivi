package middleware

import (
	"net"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

func TestSessionCookieScopesAndLifetimes(t *testing.T) {
	app := fiber.New()
	app.Get("/:scope", func(c *fiber.Ctx) error {
		SetSessionCookie(c, c.Params("scope"), "opaque", time.Now().Add(time.Hour))
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
