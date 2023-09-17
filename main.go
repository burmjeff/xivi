package main

import (
	"errors"
	"fmt"
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
	CONFIG_PATH := os.Getenv("CONFIG_PATH")
	STREAM_PATH := os.Getenv("STREAM_PATH")
	M3U_FILEPATH := fmt.Sprintf("%s/m3u", STREAM_PATH)
	EPG_FILEPATH := fmt.Sprintf("%s/epg", STREAM_PATH)
	LOGO_FILEPATH := fmt.Sprintf("%s/logo", STREAM_PATH)
	MODEL_PATH := os.Getenv("MODEL_PATH")

	zerolog.SetGlobalLevel(zerolog.ErrorLevel)

	// Start server (with graceful shutdown).
	if _, err := os.Stat(CONFIG_PATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(CONFIG_PATH, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}
	if _, err := os.Stat(STREAM_PATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(STREAM_PATH, os.ModePerm)
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
	if _, err := os.Stat(LOGO_FILEPATH); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(LOGO_FILEPATH, os.ModePerm)
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
