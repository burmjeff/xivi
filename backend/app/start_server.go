package app

import (
	"flag"
	"fmt"

	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// StartServer func for starting a simple server.
func StartServer(app *fiber.App) {

	//Initialize DB
	database.InitDB()

	//config values
	host := flag.String("host", settings.APP_SETTINGS.Host, "Server Host")
	port := flag.Int("port", settings.APP_SETTINGS.Port, "Server Port")
	settings.SERVER_PATH = fmt.Sprintf("%s:%d", *host, *port)

	// Run server.
	log.Printf("Server starting at http://%s ...\n", settings.SERVER_PATH)
	if err := app.Listen(settings.SERVER_PATH); err != nil {
		log.Printf("Oops... Server is not running! Reason: %v", err)
	}
}
