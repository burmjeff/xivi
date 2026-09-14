package routes

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestSvelteAboutIsAnonymousAndNotCached(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("build", 0700); err != nil {
		t.Fatal(err)
	}
	const shell = "<!doctype html><title>Xivi shell</title>"
	if err := os.WriteFile(filepath.Join("build", "index.html"), []byte(shell), 0600); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	SvelteRoutes(app)
	response, err := app.Test(httptest.NewRequest("GET", "/about", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || string(body) != shell {
		t.Fatalf("anonymous about route: status %d, body %q", response.StatusCode, body)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("about shell must not be cached: %v", response.Header)
	}
}
