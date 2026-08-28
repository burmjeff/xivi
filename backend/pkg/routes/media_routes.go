package routes

import (
	"time"
	"xivi/backend/app/controllers"
	"xivi/backend/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

func MediaRoutes(a *fiber.App) {
	media := a.Group("/media/v1", middleware.DeclareRoutePolicy("media-key"), middleware.BoundedRateLimit(60, time.Minute, false))
	media.Get("/lineups/:lineup_id/playlist.m3u", middleware.RequireLineupMediaAccess("lineup_id"), controllers.GetDynamicM3U)
	media.Get("/lineups/:lineup_id/guide.xml", middleware.RequireLineupMediaAccess("lineup_id"), controllers.GetDynamicXMLTV)
}
