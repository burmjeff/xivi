package routes

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"xivi/backend/app/models"

	"github.com/gofiber/fiber/v2"
)

func responseBody(t *testing.T, responseBody io.ReadCloser) string {
	t.Helper()
	defer responseBody.Close()
	content, err := io.ReadAll(responseBody)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestCurrentDocumentationServesEmbeddedV2Contract(t *testing.T) {
	app := fiber.New()
	app.Get("/openapi.yaml", serveCurrentOpenAPI)
	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/openapi.yaml", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response.Body)
	if response.StatusCode != fiber.StatusOK || !strings.Contains(response.Header.Get(fiber.HeaderContentType), "application/yaml") {
		t.Fatalf("unexpected contract response: status=%d content-type=%q", response.StatusCode, response.Header.Get(fiber.HeaderContentType))
	}
	if !strings.Contains(body, "openapi: 3.1.0") || !strings.Contains(body, "title: Xivi Interface API") {
		t.Fatal("current API contract was not served")
	}
}

func TestDocumentationPagesClearlySeparateCurrentAndLegacyContracts(t *testing.T) {
	app := fiber.New()
	app.Use(documentationSecurityHeaders)
	app.Get("/current", documentationPage(false))
	app.Get("/legacy", documentationPage(true))

	current, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/current", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	currentBody := responseBody(t, current.Body)
	if !strings.Contains(currentBody, "Current Watch and Studio API · OpenAPI 3.1") || !strings.Contains(currentBody, "/docs/openapi.yaml") {
		t.Fatal("current documentation page is not clearly labeled")
	}
	if strings.Contains(currentBody, "<script>") {
		t.Fatal("documentation page contains an inline script blocked by CSP")
	}
	if csp := current.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("unexpected documentation CSP: %q", csp)
	}

	legacy, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/legacy", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	legacyBody := responseBody(t, legacy.Body)
	if !strings.Contains(legacyBody, "Legacy administrator API · deprecated") || !strings.Contains(legacyBody, "/docs/legacy/openapi.json") {
		t.Fatal("legacy documentation page is not clearly labeled")
	}
}

func TestDocumentationAssetAllowlist(t *testing.T) {
	app := fiber.New()
	app.Get("/assets/:asset", serveDocumentationAsset)

	allowed, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/assets/xivi-docs.js", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	if allowed.StatusCode != fiber.StatusOK || !strings.Contains(responseBody(t, allowed.Body), "X-CSRF-Token") {
		t.Fatal("documentation initializer was not served")
	}

	stylesheet, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/assets/swagger-ui.css", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	if stylesheet.StatusCode != fiber.StatusOK || !strings.Contains(responseBody(t, stylesheet.Body), ".swagger-ui") {
		t.Fatal("embedded Swagger UI stylesheet was not served")
	}

	rejected, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/assets/not-allowed.js", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer rejected.Body.Close()
	if rejected.StatusCode != fiber.StatusNotFound {
		t.Fatalf("unknown documentation asset returned %d", rejected.StatusCode)
	}
}

func TestDocumentationPageRedirectsToLoginWithoutSession(t *testing.T) {
	app := fiber.New()
	app.Get("/docs/", redirectUnauthenticatedDocumentationPage("/docs/"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/docs/", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusSeeOther || response.Header.Get(fiber.HeaderLocation) != "/login?next=%2Fdocs%2F" {
		t.Fatalf("unexpected sign-in redirect: status=%d location=%q", response.StatusCode, response.Header.Get(fiber.HeaderLocation))
	}
}

func TestAuthenticatedAdministratorCanLoadDocumentationRoutes(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("xivi_principal", models.SessionPrincipal{Role: models.RoleAdmin})
		return c.Next()
	})
	APIDocumentationRoutes(app)

	tests := []struct {
		path       string
		contains   string
		statusCode int
	}{
		{path: "/docs/", contains: "Current Watch and Studio API", statusCode: fiber.StatusOK},
		{path: "/docs", contains: "Current Watch and Studio API", statusCode: fiber.StatusOK},
		{path: "/docs/openapi.yaml", contains: "openapi: 3.1.0", statusCode: fiber.StatusOK},
		{path: "/docs/assets/swagger-ui-bundle.js", contains: "SwaggerUIBundle", statusCode: fiber.StatusOK},
		{path: "/docs/legacy/", contains: "Legacy administrator API", statusCode: fiber.StatusOK},
	}
	for _, test := range tests {
		response, err := app.Test(httptest.NewRequest(fiber.MethodGet, test.path, nil), -1)
		if err != nil {
			t.Fatalf("GET %s: %v", test.path, err)
		}
		body := responseBody(t, response.Body)
		if response.StatusCode != test.statusCode || !strings.Contains(body, test.contains) {
			t.Fatalf("GET %s returned status=%d body=%q", test.path, response.StatusCode, body)
		}
	}
}
