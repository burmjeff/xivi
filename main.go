package main

import (
	"xivi/backend/app"
	"xivi/backend/pkg/configs"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/routes"
	"xivi/backend/platform/settings"
	_ "xivi/docs" // load API Docs files (Swagger)

	"github.com/gofiber/fiber/v2"
	_ "github.com/joho/godotenv/autoload" // load .env file automatically
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	var err error

	if err := settings.InitPaths(); err != nil {
		log.Fatal().Msg(err.Error())
	}

	if settings.APP_SETTINGS, err = settings.InitSettings(); err != nil {
		log.Fatal().Msg(err.Error())
	}

	// Define Fiber config.
	config := configs.FiberConfig()

	// Define a new Fiber app with config.
	a := fiber.New(config)

	// Middlewares.
	middleware.FiberMiddleware(a) // Register Fiber's middleware for app.

	// Routes.
	routes.SvelteRoutes(a)
	routes.SwaggerRoutes(a) // Register a route for API Docs (Swagger).
	routes.PublicRoutes(a)  // Register a public routes for app.
	routes.PrivateRoutes(a) // Register a private routes for app.
	routes.NotFoundRoute(a) // Register route for 404 Error.

	app.StartServer(a)
}
