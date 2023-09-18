package utils

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"xivi/backend/app"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/davidbyttow/govips/v2/vips"
	log "github.com/sirupsen/logrus"
)

type ImageTools struct {
	Db *database.Queries
}

func CreateLogo(db *database.Queries, logoUrl string) (int64, error) {
	validate := NewValidator()
	logo := &models.Logo{}

	img, err := downloadImage(logoUrl)
	if err != nil {
		log.Warnln("Failed Image Download: ", err)
		return 0, err
	}

	imgPath, err := normalizeImage(img)
	if err != nil {
		log.Warnln(err)
		return 0, err
	}
	logo.Img = imgPath

	// Validate playlist fields.
	if err := validate.Struct(logo); err != nil {
		//Some fields are not valid.
		log.Warnln(err)
	} else {
		logoID, err := db.CreateLogo(logo)
		if err != nil {
			log.Warnln(err)
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
	imageScale := 256 / image1.Width()

	image1.ThumbnailWithSize((image1.Width() * imageScale), (image1.Height() * imageScale), vips.InterestingAll, vips.SizeBoth)
	ep := vips.NewDefaultPNGExportParams()
	image1bytes, _, err := image1.Export(ep)

	uuid := CreateUuid()
	imgPath := fmt.Sprintf("%s/%s.png", app.LOGO_FILEPATH, uuid)

	err = os.WriteFile(imgPath, image1bytes, 0644)
	if err != nil {
		return "", err
	}

	return imgPath, nil

}

func GetChannelLogo(db *database.Queries, logoId int64) string {
	logo, err := db.GetLogo(logoId)
	if err != nil {
		return ""
	}

	//TODO: images route
	logoUrl := fmt.Sprintf("images/%s", filepath.Base(logo.Img))

	return logoUrl
}
