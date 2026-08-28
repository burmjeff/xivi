package routes

import (
	"xivi/backend/app/models"
	"xivi/backend/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// NotFoundRoute func for describe 404 Error route.
func NotFoundRoute(a *fiber.App) {
	// Register new special route.
	a.Use(
		middleware.DeclareRoutePolicy("anonymous-not-found"),
		// Anonymous function.
		func(c *fiber.Ctx) error {
			// Return HTTP 404 status and JSON response.
			return c.Status(fiber.StatusNotFound).JSON(models.APIError{
				Code: "not_found", Message: "The requested resource was not found.", Retryable: false,
			})
		},
	)
}
