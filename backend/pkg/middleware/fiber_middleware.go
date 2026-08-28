package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

// FiberMiddleware provide Fiber's built-in middlewares.
// See: https://docs.gofiber.io/api/middleware
func FiberMiddleware(a *fiber.App) {
	a.Use(recover.New(recover.Config{EnableStackTrace: false}))
	a.Use(requestid.New())
	a.Use(func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Cross-Origin-Resource-Policy", "same-origin")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		// The static SvelteKit document carries its build-specific CSP hashes in
		// a meta policy. Keep frame denial in an HTTP header because browsers do
		// not honor frame-ancestors from a meta-delivered CSP. Documentation routes
		// replace this with their own complete, external-resource-only policy.
		c.Set("Content-Security-Policy", "frame-ancestors 'none'")
		if len(c.Path()) >= 4 && c.Path()[:4] == "/api" {
			c.Set(fiber.HeaderCacheControl, "no-store")
		}
		return c.Next()
	})
	a.Use(logger.New(logger.Config{
		Format: "${time} ${status} ${latency} ${method} ${path} request_id=${locals:requestid}\n",
	}))
	a.Use(AuthenticateSession)
	a.Use(ObserveAutomationSignals)
}
