package main

import (
	"errors"
	"log"
	"os"
	"xivi/backend/app"

	_ "xivi/docs" // load API Docs files (Swagger)

	_ "github.com/joho/godotenv/autoload" // load .env file automatically
)

// @title API
// @version 1.0
// @description This is an auto-generated API Docs.
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.email your@mail.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @BasePath /api
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

func main() {
	CONFIG_PATH := os.Getenv("CONFIG_PATH")
	M3U_FILEPATH := os.Getenv("M3U_FILEPATH")
	EPG_FILEPATH := os.Getenv("EPG_FILEPATH")
	MODEL_PATH := os.Getenv("MODEL_PATH")

	// Start server (with graceful shutdown).
	if _, err := os.Stat(CONFIG_PATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(CONFIG_PATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	if _, err := os.Stat(M3U_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(M3U_FILEPATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	if _, err := os.Stat(EPG_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(EPG_FILEPATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	if _, err := os.Stat(MODEL_PATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(MODEL_PATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	app.StartServer()
}
