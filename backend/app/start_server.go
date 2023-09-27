package app

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
	log "github.com/sirupsen/logrus"
)

// StartServer func for starting a simple server.
func StartServer(app *fiber.App) {
	//serverHost := os.Getenv("SERVER_HOST")
	serverPort, err := strconv.Atoi(os.Getenv("SERVER_PORT"))
	if err != nil {
		log.Error("Not a valid Port: ", os.Getenv("SERVER_PORT"))
		return
	}

	//Initialize DB
	database.InitDB()

	//config values
	//host := flag.String("host", serverHost, "Server Host")
	port := flag.Int("port", serverPort, "Server Port")
	SERVER_PATH = fmt.Sprintf("0.0.0.0:%d", *port)

	// Run server.
	log.Printf("Server starting at http://%s ...\n", SERVER_PATH)
	if err := app.Listen(SERVER_PATH); err != nil {
		log.Printf("Oops... Server is not running! Reason: %v", err)
	}
}
