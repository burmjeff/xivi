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
	serverHost := os.Getenv("SERVER_HOST")
	serverPort, err := strconv.Atoi(os.Getenv("SERVER_PORT"))
	if err != nil {
		log.Error("Not a valid Port: ", os.Getenv("SERVER_PORT"))
		return
	}

	//Initialize DB
	database.InitDB()

	//config values
	host := flag.String("host", serverHost, "Server Host")
	port := flag.Int("port", serverPort, "Server Port")
	serverPath := fmt.Sprintf("%s:%d", *host, *port)

	// serve static files
	app.Static("/", "./build")
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Render("index", nil)
	})

	// Run server.
	log.Printf("Server starting at http://%s ...\n", serverPath)
	if err := app.Listen(serverPath); err != nil {
		log.Printf("Oops... Server is not running! Reason: %v", err)
	}
}
