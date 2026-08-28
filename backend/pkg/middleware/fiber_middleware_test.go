package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestGlobalCSPLeavesApplicationHashesToStaticDocument(t *testing.T) {
	app := fiber.New()
	FiberMiddleware(app)
	app.Get("/", func(c *fiber.Ctx) error { return c.SendString("ok") })

	response, err := app.Test(httptest.NewRequest(fiber.MethodGet, "http://xivi.test/", nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if got := response.Header.Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
		t.Fatalf("global CSP would conflict with the static document hashes: %q", got)
	}
	if got := response.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("frame denial header = %q, want DENY", got)
	}
}
