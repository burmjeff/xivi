package middleware

import (
	"net/http/httptest"
	"testing"
	"xivi/backend/app/models"

	"github.com/gofiber/fiber/v2"
)

func TestTVViewerAllowlist(t *testing.T) {
	for _, path := range []string{"/api/v2/studio/users", "/api/v2/auth/sessions", "/api/v2/account/media-keys", "/api/v2/tv/auth/decision", "/api/v2/mobile/auth/password", "/api/v2/stream/prewarm/uuid", "/api/templates"} {
		for _, method := range []string{"GET", "POST", "PATCH", "DELETE"} {
			if tvPathAllowed(method, path) {
				t.Fatalf("TV administration allowed: %s %s", method, path)
			}
		}
	}
	for _, path := range []string{"/api/v2/watch/lineups", "/api/v2/watch/preferences", "/api/v2/watch/search", "/stream/hls/uuid", "/images/logo.png"} {
		if !tvPathAllowed("GET", path) {
			t.Fatalf("viewer route denied: %s", path)
		}
	}
}
func TestTVAdminPrincipalCannotUseStudioAndBrowserCSRFStillApplies(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(principalLocal, models.SessionPrincipal{UserID: 1, Role: models.RoleAdmin})
		if c.Get("Test-TV") == "yes" {
			c.Locals(tvDeviceLocal, &models.TVDevice{ID: 1})
		}
		return c.Next()
	})
	app.Get("/admin", RequireAdmin(), func(c *fiber.Ctx) error { return c.SendStatus(204) })
	app.Post("/mutation", CSRFProtected(), func(c *fiber.Ctx) error { return c.SendStatus(204) })
	tests := []struct {
		method, path, tv string
		want             int
	}{{"GET", "/admin", "yes", 403}, {"POST", "/mutation", "", 403}, {"POST", "/mutation", "yes", 204}}
	for _, test := range tests {
		req := httptest.NewRequest(test.method, test.path, nil)
		req.Header.Set("Test-TV", test.tv)
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != test.want {
			t.Fatalf("%s TV=%s: %d", test.path, test.tv, res.StatusCode)
		}
	}
}
