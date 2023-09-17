package utils

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/davidbyttow/govips/v2/vips"
	log "github.com/sirupsen/logrus"
)

type ImageTools struct {
	Db *database.Queries
}

func CreateLogo(db *database.Queries, logoUrl string) int64 {
	validate := NewValidator()
	logo := &models.Logo{}

	img, err := downloadImage(logoUrl)
	if err != nil {
		log.Warnln("Failed Image Download: ", err)
		return 0
	}

	imgPath, err := normalizeImage(img)
	if err != nil {
		log.Warnln(err)
		return 0
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
			return logoID
		}

	}
	return 0
}

func downloadImage(URL string) (io.Reader, error) {
	//Get the response bytes from the url
	response, err := http.Get(URL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil, errors.New("Received non 200 response code")
	}

	return response.Body, nil
}

func normalizeImage(img io.Reader) (string, error) {
	vips.Startup(nil)
	defer vips.Shutdown()

	image1, err := vips.NewImageFromReader(img)
	if err != nil {
		return "", err
	}
	imageScale := (256 / image1.Width())

	image1.ThumbnailWithSize((image1.Width() * imageScale), (image1.Height() * imageScale), vips.InterestingAll, vips.SizeBoth)
	ep := vips.NewDefaultPNGExportParams()
	image1bytes, _, err := image1.Export(ep)

	uuid := CreateUuid()
	imgPath := fmt.Sprintf("%s/logo/%s.png", os.Getenv("STREAM_PATH"), uuid)

	err = os.WriteFile(imgPath, image1bytes, 0644)
	if err != nil {
		return "", err
	}

	return imgPath, nil

}

func getChannelLogo(logoId int64) string {

	return ""
}
