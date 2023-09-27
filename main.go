package main

import (
	"errors"
	"log"
	"os"

	"xivi/backend/app"
	"xivi/backend/pkg/configs"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/routes"
	_ "xivi/docs" // load API Docs files (Swagger)

	"github.com/gofiber/fiber/v2"
	_ "github.com/joho/godotenv/autoload" // load .env file automatically
	"github.com/rs/zerolog"
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

	// Start server (with graceful shutdown).
	if _, err := os.Stat(app.CONFIG_PATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(app.CONFIG_PATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	if _, err := os.Stat(app.SERVE_PATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(app.SERVE_PATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	if _, err := os.Stat(app.M3U_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(app.M3U_FILEPATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	if _, err := os.Stat(app.EPG_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(app.EPG_FILEPATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	if _, err := os.Stat(app.LOGO_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(app.LOGO_FILEPATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	if _, err := os.Stat(app.STREAM_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(app.STREAM_FILEPATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	if _, err := os.Stat(app.MODEL_PATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(app.MODEL_PATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
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
