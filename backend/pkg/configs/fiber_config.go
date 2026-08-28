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
		ReadTimeout:           time.Second * time.Duration(settings.Current().Server.ReadTimeout),
		BodyLimit:             8 * 1024 * 1024,
		ReadBufferSize:        16 * 1024,
		ServerHeader:          "",
		DisableStartupMessage: true,
		Views:                 engine, //set as render engine
	}
}
