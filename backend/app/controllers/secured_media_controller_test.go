package controllers

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

func TestRewriteSecuredXMLTVScopesLocalIconsAndDropsExternalURLs(t *testing.T) {
	input := `<?xml version="1.0"?><tv><channel id="one"><icon src="/images/one.png"/></channel><programme channel="one"><icon src="https://provider.example/private.png?token=secret"/></programme></tv>`
	var output bytes.Buffer
	if err := rewriteSecuredXMLTV(strings.NewReader(input), &output, "https://tv.example", "xmk_test.secret", 7); err != nil {
		t.Fatal(err)
	}
	value := output.String()
	if !strings.Contains(value, `src="https://tv.example/images/one.png?access_token=xmk_test.secret&amp;lineup_id=7"`) {
		t.Fatalf("secured local icon missing: %s", value)
	}
	if strings.Contains(value, "provider.example") || strings.Contains(value, "token=secret") {
		t.Fatalf("upstream icon leaked: %s", value)
	}
}

func TestSecuredImageRejectsPathTraversal(t *testing.T) {
	original := settings.LOGO_FILEPATH
	settings.LOGO_FILEPATH = filepath.Join(t.TempDir(), "logos")
	t.Cleanup(func() { settings.LOGO_FILEPATH = original })
	if err := os.MkdirAll(settings.LOGO_FILEPATH, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(settings.LOGO_FILEPATH), "secret.png"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/images/:asset", GetSecuredImage)
	for _, target := range []string{"/images/..%2Fsecret.png", "/images/%2e%2e%2fsecret.png"} {
		response, err := app.Test(httptest.NewRequest(fiber.MethodGet, target, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode == fiber.StatusOK {
			t.Fatalf("path traversal %q returned the external file", target)
		}
		_ = response.Body.Close()
	}
}

func TestSanitizeM3UValueRemovesControlAndDelimiterInjection(t *testing.T) {
	value := sanitizeM3UValue("bad,\"\t\r\n#EXTINF:evil\x00\x7f")
	if strings.ContainsAny(value, ",\"\t\r\n\x00\x7f") {
		t.Fatalf("unsafe M3U value: %q", value)
	}
}
