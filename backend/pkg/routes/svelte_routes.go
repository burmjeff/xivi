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
	router.Get("/viewer", func(c *fiber.Ctx) error {
		return c.Render("index", nil)
	})
	router.Get("/channels", func(c *fiber.Ctx) error {
		return c.Render("index", nil)
	})
	router.Get("/epg", func(c *fiber.Ctx) error {
		return c.Render("index", nil)
	})
	router.Get("/settings", func(c *fiber.Ctx) error {
		return c.Render("index", nil)
	})
}
