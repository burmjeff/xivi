package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"xivi/backend/app"
	"xivi/backend/pkg/configs"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/routes"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	_ "github.com/joho/godotenv/autoload" // load .env file automatically
	"github.com/rs/zerolog/log"
)

// @title API
// @version 1.0
// @description Legacy administrator API. The security and product interface contract is docs/openapi-v2.yaml.
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @BasePath /api
func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		port := strings.TrimSpace(os.Getenv("SERVER_PORT"))
		if port == "" {
			port = "3000"
		}
		client := &http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://127.0.0.1:" + port + "/healthz")
		if err != nil || response.StatusCode != http.StatusOK {
			fmt.Fprintln(os.Stderr, "Xivi health check failed")
			os.Exit(1)
		}
		_ = response.Body.Close()
		return
	}

	var err error

	if err := settings.InitPaths(); err != nil {
		log.Fatal().Msg(err.Error())
	}
	// Key generation must work before a first production config exists. It
	// does not open the database or accept secret material in process arguments.
	if app.IsGenerateKeyCLI(os.Args[1:]) {
		if err := app.RunAuthCLI(os.Args[1:]); err != nil {
			log.Fatal().Err(err).Msg("Authentication command failed")
		}
		return
	}

	if err = settings.InitSettings(); err != nil {
		log.Fatal().Msg(err.Error())
	}
	if err := security.InitializeKey(); err != nil {
		log.Fatal().Err(err).Msg("Authentication security could not be initialized")
	}
	if app.IsAuthCLI(os.Args[1:]) {
		if err := app.RunAuthCLI(os.Args[1:]); err != nil {
			log.Fatal().Err(err).Msg("Authentication command failed")
		}
		return
	}

	//TODO LET USER SET DEFAULT CHANNEL LOGO
	settings.CopyDefaultLogo()

	// Define Fiber config.
	config := configs.FiberConfig()

	// Define a new Fiber app with config.
	a := fiber.New(config)

	// Middlewares.
	middleware.FiberMiddleware(a) // Register Fiber's middleware for app.

	// Routes.
	routes.SvelteRoutes(a)
	routes.APIDocumentationRoutes(a) // Register protected current and legacy API documentation.
	routes.V2Routes(a)               // Register the additive product-interface API.
	routes.PublicRoutes(a)           // Register legacy administrator routes and public media paths.
	routes.MediaRoutes(a)            // Authenticated M3U and XMLTV outputs.
	routes.NotFoundRoute(a)          // Register route for 404 Error.

	app.StartServer(a)
}
