package logoassets

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"xivi/backend/pkg/outbound"
	"xivi/backend/platform/settings"

	"github.com/davidbyttow/govips/v2/vips"
)

func TestSourceLogoNameIsReadableStableAndURLScoped(t *testing.T) {
	first := SourceLogoName("https://one.example/images/ESPN%20HD.svg?size=512")
	if first != SourceLogoName("https://one.example/images/ESPN%20HD.svg?size=512") {
		t.Fatal("the same source URL did not produce a stable logo name")
	}
	if first == SourceLogoName("https://two.example/images/ESPN%20HD.svg?size=512") {
		t.Fatal("different providers with the same filename produced a collision")
	}
	if !strings.HasPrefix(first, "source-espn-") {
		t.Fatalf("source logo name is not recognizable: %q", first)
	}
}

func TestStoreSourceLogoDownloadsAndNormalizesImage(t *testing.T) {
	var source bytes.Buffer
	picture := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&source, picture); err != nil {
		t.Fatalf("building source image failed: %v", err)
	}
	previousFetcher := fetchSourceLogo
	fetchSourceLogo = func(context.Context, string, outbound.Policy, int64, http.Header) ([]byte, http.Header, error) {
		return source.Bytes(), http.Header{"Content-Type": []string{"image/png"}}, nil
	}
	defer func() { fetchSourceLogo = previousFetcher }()

	previousLogoPath := settings.LOGO_FILEPATH
	settings.LOGO_FILEPATH = t.TempDir()
	defer func() { settings.LOGO_FILEPATH = previousLogoPath }()
	vips.Startup(nil)
	defer vips.Shutdown()

	if err := StoreSourceLogo(context.Background(), "https://provider.example/provider.png", "source-provider-test"); err != nil {
		t.Fatalf("storing source logo failed: %v", err)
	}
	stored := filepath.Join(settings.LOGO_FILEPATH, "source-provider-test.png")
	if info, err := os.Stat(stored); err != nil || info.Size() == 0 {
		t.Fatalf("normalized source logo was not written: info=%#v err=%v", info, err)
	}
}
