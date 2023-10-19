package configs

import (
	"time"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html"
)

// FiberConfig func for configuration Fiber app.
// See: https://docs.gofiber.io/api/fiber#config
func FiberConfig() fiber.Config {
	// Define server settings.
	engine := html.New("./build", ".html")

	// Return Fiber configuration.
	return fiber.Config{
		ReadTimeout: time.Second * time.Duration(settings.APP_SETTINGS.Server.ReadTimeout),
		Views:       engine, //set as render engine
	}
}
