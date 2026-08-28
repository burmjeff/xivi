package middleware

import "github.com/gofiber/fiber/v2"

const RoutePolicyHeader = "X-Xivi-Route-Policy"

// DeclareRoutePolicy makes every security boundary machine-auditable. The
// value is intentionally coarse and contains no user or resource data.
func DeclareRoutePolicy(policy string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("xivi_route_policy", policy)
		c.Set(RoutePolicyHeader, policy)
		return c.Next()
	}
}
