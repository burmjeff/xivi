package routes

import (
	"github.com/gofiber/fiber/v2"
	"path/filepath"
	"strings"
	"xivi/backend/pkg/middleware"
)

// Svelte Routes
func SvelteRoutes(a *fiber.App) {
	// Create routes group.
	router := a.Group("/", middleware.DeclareRoutePolicy("anonymous-shell"))
	router.Use(func(c *fiber.Ctx) error {
		extension := strings.ToLower(filepath.Ext(c.Path()))
		if extension == "" || extension == ".html" {
			c.Set(fiber.HeaderCacheControl, "no-store")
		}
		return c.Next()
	})
	router.Static("/", "./build")

	// Routes for GET method:
	router.Get("/", func(c *fiber.Ctx) error {
		return c.SendFile("./build/index.html")
	})
	router.Get("/channels", func(c *fiber.Ctx) error {
		return c.SendFile("./build/index.html")
	})
	router.Get("/guide", func(c *fiber.Ctx) error { return c.SendFile("./build/index.html") })
	router.Get("/login", func(c *fiber.Ctx) error { return c.SendFile("./build/index.html") })
	router.Get("/account", func(c *fiber.Ctx) error { return c.SendFile("./build/index.html") })
	router.Get("/watch/channel/:id", func(c *fiber.Ctx) error { return c.SendFile("./build/index.html") })
	router.Get("/studio", func(c *fiber.Ctx) error { return c.SendFile("./build/index.html") })
	router.Get("/studio/*", func(c *fiber.Ctx) error { return c.SendFile("./build/index.html") })
}
