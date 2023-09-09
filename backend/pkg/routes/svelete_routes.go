package routes

import (
	"github.com/gofiber/fiber/v2"
)

// Svelte Routes
func SvelteRoutes(a *fiber.App) {
	// Create routes group.
	router := a.Group("/")
	router.Static("/", "./build")

	// Routes for GET method:
	router.Get("/", func(c *fiber.Ctx) error {
		return c.Render("index", nil)
	})
	router.Get("/templates", func(c *fiber.Ctx) error {
		return c.Render("index", nil)
	})
}
