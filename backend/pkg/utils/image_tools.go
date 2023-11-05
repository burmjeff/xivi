package utils

import (
	"encoding/base64"
	"errors"
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

type ImageTools struct {
	Db *database.Queries
}

func UploadLogo(db *database.Queries, logoUpload *models.LogoUpload) (int64, error) {
	validate := NewValidator()
	logo := &models.Logo{}

	image, err := base64Decode(logoUpload.Image)

	if err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}

	imgPath, err := normalizeImage(image)
	if err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}
	logo.Img = imgPath

	// Validate playlist fields.
	if err := validate.Struct(logo); err != nil {
		//Some fields are not valid.
		log.Warn().Msg(err.Error())
	} else {
		logo_id, err := db.CreateLogo(logo)
		if err != nil {
			log.Warn().Msg(err.Error())
		} else {

			return logo_id, nil
		}

	}
	return 0, err
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

func CreateLogo(db *database.Queries, logoUrl string) (int64, error) {
	validate := NewValidator()
	logo := &models.Logo{}

	img, err := downloadImage(logoUrl)
	if err != nil {
		log.Warn().Msgf("Failed Image Download: %v", err)
		return 0, err
	}

	imgPath, err := normalizeImage(img)
	if err != nil {
		log.Warn().Msg(err.Error())
		return 0, err
	}
	logo.Img = imgPath

	// Validate playlist fields.
	if err := validate.Struct(logo); err != nil {
		//Some fields are not valid.
		log.Warn().Msg(err.Error())
	} else {
		logoID, err := db.CreateLogo(logo)
		if err != nil {
			log.Warn().Msg(err.Error())
		} else {
			return logoID, nil
		}

	}
	return 0, err
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

func normalizeImage(img []byte) (string, error) {
	image1, err := vips.NewImageFromBuffer(img)
	if err != nil {
		return "", err
	}

	//imageScale := 256 / image1.Width()
	//image1.ThumbnailWithSize((image1.Width() * imageScale), (image1.Height() * imageScale), vips.InterestingAll, vips.SizeForce)
	image1.SmartCrop(256, 256, vips.InterestingAll)
	ep := vips.NewDefaultPNGExportParams()
	image1bytes, _, err := image1.Export(ep)

	uuid := CreateUuid()
	imgPath := fmt.Sprintf("%s/%s.png", settings.LOGO_FILEPATH, uuid)

	err = os.WriteFile(imgPath, image1bytes, 0644)
	if err != nil {
		return "", err
	}

	return imgPath, nil

}

func GetChannelLogo(db *database.Queries, logoId int64) (models.Logo, error) {
	logo, err := db.GetLogo(logoId)
	if err != nil {
		return logo, err
	}

	//TODO: images route
	logo.Img = fmt.Sprintf("images/%s", filepath.Base(logo.Img))

	return logo, nil
}
