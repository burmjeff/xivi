package security

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// SendFileLiteral serves a filesystem path without interpreting its name as a URI.
// Callers must still authorize the path and enforce their directory boundaries.
func SendFileLiteral(c *fiber.Ctx, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// Fiber v2's SendFile still passes the filename through RequestURI parsing,
	// even with fasthttp v1.70 (GO-2026-4950). Quote once, after resolving the
	// filesystem path, so that parsing cannot decode a different file or parent
	// directory. Keep Fiber's range, status, URL restoration and no-index behavior.
	fileURL := url.URL{Path: filepath.ToSlash(absolute)}
	if strings.HasSuffix(filepath.ToSlash(path), "/") && !strings.HasSuffix(fileURL.Path, "/") {
		fileURL.Path += "/"
	}
	return c.SendFile(fileURL.EscapedPath())
}
