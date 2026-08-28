package routes

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

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

	rejected, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/assets/not-allowed.js", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer rejected.Body.Close()
	if rejected.StatusCode != fiber.StatusNotFound {
		t.Fatalf("unknown documentation asset returned %d", rejected.StatusCode)
	}
}
