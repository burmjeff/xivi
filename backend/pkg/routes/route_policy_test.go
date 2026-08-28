package routes

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"xivi/backend/pkg/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

var routeParameter = regexp.MustCompile(`:[A-Za-z0-9_]+`)

func concreteTestPath(path string) string {
	path = routeParameter.ReplaceAllString(path, "1")
	path = strings.ReplaceAll(path, "*", "index.html")
	return path
}

func TestEveryRegisteredRouteDeclaresAnAuthorizationPolicy(t *testing.T) {
	app := fiber.New()
	app.Use(recover.New())
	SvelteRoutes(app)
	SwaggerRoutes(app)
	V2Routes(app)
	PublicRoutes(app)
	MediaRoutes(app)
	NotFoundRoute(app)

	seen := map[string]bool{}
	for _, route := range app.GetRoutes(true) {
		if route.Method == fiber.MethodHead || route.Method == fiber.MethodOptions || route.Method == "USE" {
			continue
		}
		key := route.Method + " " + route.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		request := httptest.NewRequest(route.Method, concreteTestPath(route.Path), strings.NewReader(`{}`))
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		response, err := app.Test(request, -1)
		if err != nil {
			t.Errorf("%s could not be exercised: %v", key, err)
			continue
		}
		if policy := response.Header.Get(middleware.RoutePolicyHeader); policy == "" {
			t.Errorf("%s has no declared authorization policy", key)
		}
		_ = response.Body.Close()
	}
}

func TestAnonymousRequestsCannotReachProtectedSurfaces(t *testing.T) {
	app := fiber.New()
	app.Use(recover.New())
	V2Routes(app)
	PublicRoutes(app)
	MediaRoutes(app)

	cases := []struct {
		method string
		path   string
		want   int
	}{
		{fiber.MethodGet, "/api/playlists", fiber.StatusUnauthorized},
		{fiber.MethodGet, "/api/v2/watch/lineups", fiber.StatusUnauthorized},
		{fiber.MethodGet, "/api/v2/studio/overview", fiber.StatusUnauthorized},
		{fiber.MethodGet, "/api/v2/events", fiber.StatusUnauthorized},
		{fiber.MethodGet, "/stream/channel-uuid", fiber.StatusUnauthorized},
		{fiber.MethodGet, "/media/v1/lineups/1/playlist.m3u", fiber.StatusUnauthorized},
		{fiber.MethodGet, "/images/channel.png?lineup_id=1", fiber.StatusUnauthorized},
	}
	for _, test := range cases {
		request := httptest.NewRequest(test.method, test.path, nil)
		response, err := app.Test(request, -1)
		if err != nil {
			t.Errorf("%s %s failed: %v", test.method, test.path, err)
			continue
		}
		if response.StatusCode != test.want {
			t.Errorf("%s %s returned %d, want %d", test.method, test.path, response.StatusCode, test.want)
		}
		_ = response.Body.Close()
	}
}

func TestBootstrapStatusIsNotCapturedByLegacyAdminPrefix(t *testing.T) {
	app := fiber.New()
	app.Use(recover.New())
	V2Routes(app)
	PublicRoutes(app)

	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/api/v2/auth/bootstrap-status", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode == fiber.StatusUnauthorized || response.Header.Get(middleware.RoutePolicyHeader) != "anonymous" {
		t.Fatalf("bootstrap route was captured by another policy: status=%d policy=%q", response.StatusCode, response.Header.Get(middleware.RoutePolicyHeader))
	}
	var payload struct {
		BootstrapRequired             bool `json:"bootstrap_required"`
		InitialPasswordChangeRequired bool `json:"initial_password_change_required"`
		InitialLoginAllowed           bool `json:"initial_login_allowed"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.BootstrapRequired || payload.InitialPasswordChangeRequired || payload.InitialLoginAllowed {
		t.Fatalf("public bootstrap response disclosed setup state: %+v", payload)
	}
}

func TestNotFoundResponseHasExplicitPolicyAndSafeEnvelope(t *testing.T) {
	app := fiber.New()
	NotFoundRoute(app)
	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/definitely-not-a-route", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusNotFound || response.Header.Get(middleware.RoutePolicyHeader) != "anonymous-not-found" {
		t.Fatalf("unexpected not-found boundary: status=%d policy=%q", response.StatusCode, response.Header.Get(middleware.RoutePolicyHeader))
	}
}
