package logoassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"xivi/backend/pkg/outbound"
	"xivi/backend/platform/settings"

	"github.com/davidbyttow/govips/v2/vips"
)

const maxSourceLogoBytes = 8 << 20

var fetchSourceLogo = outbound.FetchBytes

// SourceLogoName creates a readable, filesystem-safe identity for a remote
// playlist logo. The URL hash prevents unrelated providers with the same image
// filename from overwriting one another.
func SourceLogoName(rawURL string) string {
	base := "channel"
	if parsed, err := url.Parse(rawURL); err == nil {
		candidate := strings.TrimSuffix(path.Base(parsed.Path), path.Ext(parsed.Path))
		if strings.TrimSpace(candidate) != "" && candidate != "." && candidate != "/" {
			base = candidate
		}
	}
	var cleaned strings.Builder
	lastSeparator := false
	for _, value := range strings.ToLower(base) {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			cleaned.WriteRune(value)
			lastSeparator = false
		} else if !lastSeparator && cleaned.Len() > 0 {
			cleaned.WriteByte('-')
			lastSeparator = true
		}
	}
	readable := strings.Trim(cleaned.String(), "-")
	if readable == "" {
		readable = "channel"
	}
	if runes := []rune(readable); len(runes) > 72 {
		readable = strings.TrimRight(string(runes[:72]), "-")
	}
	digest := sha256.Sum256([]byte(rawURL))
	return fmt.Sprintf("source-%s-%s", readable, hex.EncodeToString(digest[:5]))
}

// StoreSourceLogo downloads and normalizes a remote playlist logo into Xivi's
// local logo directory. The caller owns the corresponding database record.
func StoreSourceLogo(ctx context.Context, rawURL, name string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("source logo URL is invalid")
	}
	if err := os.MkdirAll(settings.LOGO_FILEPATH, 0o700); err != nil {
		return err
	}
	target := filepath.Join(settings.LOGO_FILEPATH, name+".png")
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	content, headers, err := fetchSourceLogo(ctx, parsed.String(), outbound.Policy{AllowPrivate: true, Timeout: 12 * time.Second, MaxRedirects: 3}, maxSourceLogoBytes, nil)
	if err != nil {
		return err
	}
	if contentType := strings.ToLower(headers.Get("Content-Type")); contentType != "" && !strings.HasPrefix(contentType, "image/") && contentType != "application/octet-stream" {
		return fmt.Errorf("source logo response was not an image")
	}

	image, err := vips.NewImageFromBuffer(content)
	if err != nil {
		return fmt.Errorf("source logo is not a supported image: %w", err)
	}
	defer image.Close()
	if image.Width() < 1 || image.Height() < 1 || int64(image.Width())*int64(image.Height()) > 40_000_000 {
		return fmt.Errorf("source logo dimensions exceed the 40 megapixel limit")
	}
	if err := image.ThumbnailWithSize(256, 256, vips.InterestingNone, vips.SizeBoth); err != nil {
		return err
	}
	imageBytes, _, err := image.Export(vips.NewDefaultPNGExportParams())
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp(settings.LOGO_FILEPATH, name+"-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(imageBytes); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, target); err != nil {
		if _, statErr := os.Stat(target); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}
