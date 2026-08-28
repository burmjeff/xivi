package utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/outbound"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/rs/zerolog/log"
)

func UploadLogo(logo *models.Logo) (int64, error) {
	ctx := context.Background()
	if !validLogoName(logo.Name) {
		return 0, fmt.Errorf("logo name may contain only letters, numbers, hyphens, and underscores")
	}
	image, err := base64Decode(logo.Image)

	if err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}

	if err := saveImage(logo.Name, image); err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}

	logoID, err := database.Db.CreateLogo(ctx, logo.Name)
	if err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}
	return logoID, nil
}

func base64Decode(str string) ([]byte, error) {
	if len(str) > 8<<20 {
		return nil, fmt.Errorf("image payload exceeds the upload limit")
	}
	i := strings.Index(str, ",")
	if i < 0 {
		return nil, fmt.Errorf("invalid base64 image payload")
	}
	data, err := base64.StdEncoding.DecodeString(str[i+1:])
	if err != nil {
		return nil, err
	}
	return data, nil
}

func CreateLogo(logoUrl string) (int64, error) {
	ctx := context.Background()
	img, err := downloadImage(logoUrl)
	if err != nil {
		log.Warn().Msgf("Failed Image Download: %v", err)
		return 0, err
	}
	logoName := strings.TrimSuffix(filepath.Base(logoUrl), filepath.Ext(logoUrl))

	if err := saveImage(logoName, img); err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}

	logoID, err := database.Db.CreateLogo(ctx, logoName)
	if err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}

	return logoID, nil
}

func downloadImage(URL string) ([]byte, error) {
	content, headers, err := outbound.FetchBytes(context.Background(), URL, outbound.Policy{Timeout: 15 * time.Second, MaxRedirects: 3}, 8<<20, nil)
	if err != nil {
		return nil, err
	}
	if contentType := strings.ToLower(headers.Get("Content-Type")); contentType != "" && !strings.HasPrefix(contentType, "image/") && contentType != "application/octet-stream" {
		return nil, fmt.Errorf("remote response was not an image")
	}
	return content, nil
}

func saveImage(name string, img []byte) error {
	if !validLogoName(name) || len(img) > 8<<20 {
		return fmt.Errorf("invalid logo name or image size")
	}
	image, err := vips.NewImageFromBuffer(img)
	if err != nil {
		return err
	}

	if image.Width() < 1 || image.Height() < 1 || int64(image.Width())*int64(image.Height()) > 40_000_000 {
		image.Close()
		return fmt.Errorf("image dimensions exceed the 40 megapixel limit")
	}
	defer image.Close()
	if err := image.ThumbnailWithSize(256, 256, vips.InterestingNone, vips.SizeBoth); err != nil {
		return err
	}
	ep := vips.NewDefaultPNGExportParams()
	imageBytes, _, err := image.Export(ep)
	if err != nil {
		return err
	}

	imgPath := GetLogoPath(name)
	if err := os.MkdirAll(settings.LOGO_FILEPATH, 0700); err != nil {
		return err
	}
	err = os.WriteFile(imgPath, imageBytes, 0600)
	if err != nil {
		return err
	}

	return nil
}

func GetLogoUrl(name string) string {
	//TODO: images route
	return fmt.Sprintf("images/%s.png", name)
}

func GetLogoPath(name string) string {
	return filepath.Join(settings.LOGO_FILEPATH, name+".png")

}

var safeLogoName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$`)

func validLogoName(name string) bool { return safeLogoName.MatchString(name) }

func logoExists(logoName string) *models.Logo {
	ctx := context.Background()
	if logo, err := database.Db.GetLogoByName(ctx, logoName); err != nil {
		log.Debug().Msgf("logoExists: %v", err.Error())
		return nil
	} else {
		return logo
	}
}
