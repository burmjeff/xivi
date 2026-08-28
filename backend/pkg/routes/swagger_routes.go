package routes

import (
	"github.com/gofiber/fiber/v2"
	"xivi/backend/pkg/middleware"

	swagger "github.com/arsmn/fiber-swagger/v2"
)

// SwaggerRoute func for describe group of API Docs routes.
func SwaggerRoutes(a *fiber.App) {
	// Create routes group.
	router := a.Group("/swagger", middleware.DeclareRoutePolicy("admin"), middleware.RequireAdmin(), middleware.RequirePasswordChanged())

	// Routes for GET method:
	router.Get("*", swagger.HandlerDefault) // get one user by ID
}
