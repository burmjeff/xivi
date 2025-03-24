package utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/rs/zerolog/log"
)

func UploadLogo(logo *models.Logo) (int64, error) {
	ctx := context.Background()
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
	i := strings.Index(str, ",")
	if i < 0 {
		return nil, fmt.Errorf("invalid b64 image: %s", str)
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

	req, err := http.NewRequest("GET", URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := new(http.Client).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("received non 200 response code of %v", resp.StatusCode)
	}
	imgBuf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return imgBuf, nil
}

func saveImage(name string, img []byte) error {
	image, err := vips.NewImageFromBuffer(img)
	if err != nil {
		return err
	}

	//imageScale := float64(256 / image.Width())
	//image.Resize(imageScale, vips.KernelAuto)
	image.ThumbnailWithSize(256, 256, vips.InterestingNone, vips.SizeBoth)
	ep := vips.NewDefaultPNGExportParams()
	imageBytes, _, _ := image.Export(ep)

	imgPath := GetLogoPath(name)

	err = os.WriteFile(imgPath, imageBytes, 0644)
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
	//TODO: images route
	return fmt.Sprintf("%s/%s.png", settings.LOGO_FILEPATH, name)

}

func logoExists(logoName string) *models.Logo {
	ctx := context.Background()
	if logo, err := database.Db.GetLogoByName(ctx, logoName); err != nil {
		log.Debug().Msgf("logoExists: %v", err.Error())
		return nil
	} else {
		return logo
	}
}
