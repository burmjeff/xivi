package routes

import (
	"github.com/gofiber/fiber/v2"
)

// SwaggerRoute func for describe group of API Docs routes.
func SvelteRoute(a *fiber.App) {
	// Create routes group.
	route := a.Group("/")
	route.Static("/", "./build")

	// Routes for GET method:
	route.Get("/", func(c *fiber.Ctx) error {
		return c.Render("index", nil)
	})
}
