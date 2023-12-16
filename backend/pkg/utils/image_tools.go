package utils

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/rs/zerolog/log"
)

func UploadLogo(logo *models.Logo) (int64, error) {
	image, err := base64Decode(logo.Image)

	if err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}

	if err := saveImage(logo.Name, image); err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}

	logoID, err := database.Db.CreateLogo(logo.Name)
	if err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}
	return logoID, nil
}

func base64Decode(str string) ([]byte, error) {
	i := strings.Index(str, ",")
	if i < 0 {
		return nil, errors.New(fmt.Sprintf("invalid b64 image: %s", str))
	}
	data, err := base64.StdEncoding.DecodeString(str[i+1:])
	if err != nil {
		return nil, err
	}
	return data, nil
}

func CreateLogo(logoUrl string) (int64, error) {
	img, err := downloadImage(logoUrl)
	if err != nil {
		log.Warn().Msgf("Failed Image Download: %v", err)
		return 0, err
	}
	logoName := strings.Split(path.Base(logoUrl), ".")[0]

	if err := saveImage(logoName, img); err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}

	logoID, err := database.Db.CreateLogo(logoName)
	if err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}

	return logoID, nil
}

func downloadImage(URL string) ([]byte, error) {
	//Get the response bytes from the url
	response, err := http.Get(URL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil, errors.New("Received non 200 response code")
	}
	imgBuf, err := io.ReadAll(response.Body)
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
	imageBytes, _, err := image.Export(ep)

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
