package security

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func sendFileTestDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		// Fiber caches open files for 10 seconds. Windows cannot remove them
		// until the cache sweeper closes them; run before TempDir's cleanup.
		t.Cleanup(func() {
			deadline := time.Now().Add(30 * time.Second)
			for {
				err := os.RemoveAll(root)
				if err == nil {
					return
				}
				if time.Now().After(deadline) {
					t.Errorf("clean file cache: %v", err)
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
		})
	}
	return root
}

func TestSendFileLiteralPreservesFilename(t *testing.T) {
	root := sendFileTestDir(t)
	cases := []struct {
		name  string
		decoy string
	}{
		{"%61dmin.png", "admin.png"},
		{"logo%2fprivate.png", "logo/private.png"},
		{"logos/%2e%2e%2fsecret.png", "secret.png"},
		{"logo#fragment.png", "logo"},
		{"space and café.png", "unused.png"},
		{"percent%25.png", "percent%.png"},
	}
	if runtime.GOOS != "windows" {
		cases = append(cases, struct{ name, decoy string }{"logo?query.png", "logo"})
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caseDir := filepath.Join(root, strconv.Itoa(i))
			path := filepath.Join(caseDir, filepath.FromSlash(tc.name))
			decoy := filepath.Join(caseDir, filepath.FromSlash(tc.decoy))
			for name, body := range map[string]string{path: "authorized literal file", decoy: "wrong file"} {
				if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			app := fiber.New()
			app.Get("/file", func(c *fiber.Ctx) error { return SendFileLiteral(c, path) })
			response, err := app.Test(httptest.NewRequest(http.MethodGet, "/file", nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || string(body) != "authorized literal file" {
				t.Fatalf("literal %q: status=%d body=%q", tc.name, response.StatusCode, body)
			}
		})
	}
}

func TestSendFileLiteralPreservesHTTPBehavior(t *testing.T) {
	root := sendFileTestDir(t)
	t.Chdir(root)
	path := filepath.Join(root, "%61udio.ts")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "missing.png"), []byte("unauthorized decoy"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/file", func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "private, no-store")
		c.Set(fiber.HeaderContentType, "video/MP2T")
		err := SendFileLiteral(c, "./%61udio.ts")
		if c.OriginalURL() != "/file?access_token=example" {
			t.Errorf("original URL was not restored: %q", c.OriginalURL())
		}
		return err
	})
	app.Get("/missing", func(c *fiber.Ctx) error { return SendFileLiteral(c, filepath.Join(root, "missing")) })
	app.Get("/encoded-missing", func(c *fiber.Ctx) error { return SendFileLiteral(c, filepath.Join(root, "%6dissing.png")) })
	app.Get("/directory", func(c *fiber.Ctx) error { return SendFileLiteral(c, filepath.Join(root, "directory")+"/") })
	for _, tc := range []struct {
		name, method, target, byteRange, body string
		status                                int
	}{
		{"get", http.MethodGet, "/file?access_token=example", "", "0123456789", http.StatusOK},
		{"range", http.MethodGet, "/file?access_token=example", "bytes=2-5", "2345", http.StatusPartialContent},
		{"head", http.MethodHead, "/file?access_token=example", "", "", http.StatusOK},
		{"missing", http.MethodGet, "/missing", "", "", http.StatusNotFound},
		{"no decoded fallback", http.MethodGet, "/encoded-missing", "", "", http.StatusNotFound},
		{"no directory listing", http.MethodGet, "/directory", "", "", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.target, nil)
			if tc.byteRange != "" {
				request.Header.Set("Range", tc.byteRange)
			}
			response, err := app.Test(request, -1)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.status {
				t.Fatalf("status=%d body=%q", response.StatusCode, body)
			}
			if tc.status >= 400 {
				return
			}
			if string(body) != tc.body {
				t.Fatalf("body=%q, want %q", body, tc.body)
			}
			if response.Header.Get("Cache-Control") != "private, no-store" || response.Header.Get("Content-Type") != "video/MP2T" {
				t.Fatalf("lost response headers: %v", response.Header)
			}
			if tc.byteRange != "" && response.Header.Get("Content-Range") != "bytes 2-5/10" {
				t.Fatalf("bad range: %v", response.Header)
			}
		})
	}
}
