package routes

import (
	"xivi/backend/app/controllers"
	"xivi/backend/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// PrivateRoutes func for describe group of private routes.
func PrivateRoutes(a *fiber.App) {
	// Create routes group.
	route := a.Group("/api")

	// Routes for POST method:
	//route.Post("/playlist", middleware.JWTProtected(), controllers.CreatePlaylist) // create a newplaylist

	// Routes for PUT method:
	route.Put("/playlist", middleware.JWTProtected(), controllers.UpdatePlaylist) // update one playlist by ID

	// Routes for DELETE method:
	route.Delete("/playlist", middleware.JWTProtected(), controllers.DeletePlaylist) // delete one playlist by ID
}
