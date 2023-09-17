package utils

import (
	"errors"
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

	uuid := CreateUuid()
	downloadImage(logoUrl, uuid)

	vips.Startup(nil)
	defer vips.Shutdown()
	image1, err := vips.NewImageFromFile("input.jpg")

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

func downloadImage(URL, fileName string) error {
	//Get the response bytes from the url
	response, err := http.Get(URL)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return errors.New("Received non 200 response code")
	}
	//Create a empty file
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	//Write the bytes to the fiel
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}

	return nil
}

func getChannelLogo(logoId int64) string {

	return ""
}
