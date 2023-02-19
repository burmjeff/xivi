package app

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"

	"xivi/backend/pkg/configs"
	"xivi/backend/pkg/middleware"
	"xivi/backend/pkg/routes"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
	log "github.com/sirupsen/logrus"
)

// StartServerWithGracefulShutdown function for starting server with a graceful shutdown.
func StartServerWithGracefulShutdown(a *fiber.App) {
	// Create channel for idle connections.
	idleConnsClosed := make(chan struct{})

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt) // Catch OS signals.
		<-sigint

		// Received an interrupt signal, shutdown.
		if err := a.Shutdown(); err != nil {
			// Error from closing listeners, or context timeout:
			log.Printf("Oops... Server is not shutting down! Reason: %v", err)
		}

		close(idleConnsClosed)
	}()

	// Run server.
	if err := a.Listen(os.Getenv("SERVER_HOST")); err != nil {
		log.Printf("Oops... Server is not running! Reason: %v", err)
	}

	<-idleConnsClosed
}

// StartServer func for starting a simple server.
func StartServer() {
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

	// Define Fiber config.
	config := configs.FiberConfig()

	// Define a new Fiber app with config.
	app := fiber.New(config)

	// Middlewares.
	middleware.FiberMiddleware(app) // Register Fiber's middleware for app.

	// Routes.
	routes.SvelteRoute(app)
	routes.SwaggerRoute(app)  // Register a route for API Docs (Swagger).
	routes.PublicRoutes(app)  // Register a public routes for app.
	routes.PrivateRoutes(app) // Register a private routes for app.
	routes.NotFoundRoute(app) // Register route for 404 Error.

	// serve static files
	//app.Static("/", "./build")
	//app.Get("/", func(c *fiber.Ctx) error {
	//	return c.Render("index", "./build")
	//})

	// Run server.
	log.Printf("Server starting at http://%s ...\n", serverPath)
	if err := app.Listen(serverPath); err != nil {
		log.Printf("Oops... Server is not running! Reason: %v", err)
	}
}
