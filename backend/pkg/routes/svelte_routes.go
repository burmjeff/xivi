package routes

import (
	"net/http"
	"xivi/backend/app"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
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

	router.Use("/images", filesystem.New(filesystem.Config{
		Root:   http.Dir(app.LOGO_FILEPATH),
		Browse: false,
	}))
	router.Use("/m3u", filesystem.New(filesystem.Config{
		Root:   http.Dir(app.M3U_FILEPATH),
		Browse: false,
	}))
	router.Use("/epg", filesystem.New(filesystem.Config{
		Root:   http.Dir(app.EPG_FILEPATH),
		Browse: false,
	}))
}
