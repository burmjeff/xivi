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
		return c.SendFile("./build/index.html")
	})
	router.Get("/viewer", func(c *fiber.Ctx) error {
		return c.SendFile("./build/index.html")
	})
	router.Get("/channels", func(c *fiber.Ctx) error {
		return c.SendFile("./build/index.html")
	})
	router.Get("/epg", func(c *fiber.Ctx) error {
		return c.SendFile("./build/index.html")
	})
	router.Get("/settings", func(c *fiber.Ctx) error {
		return c.SendFile("./build/index.html")
	})
	router.Get("/guide", func(c *fiber.Ctx) error { return c.SendFile("./build/index.html") })
	router.Get("/watch/channel/:id", func(c *fiber.Ctx) error { return c.SendFile("./build/index.html") })
	router.Get("/studio", func(c *fiber.Ctx) error { return c.SendFile("./build/index.html") })
	router.Get("/studio/*", func(c *fiber.Ctx) error { return c.SendFile("./build/index.html") })
}
