package routes

import (
	"xivi/backend/app/controllers"
	"xivi/backend/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

func MediaRoutes(a *fiber.App) {
	media := a.Group("/media/v1", middleware.DeclareRoutePolicy("media-key"))
	media.Get("/lineups/:lineup_id/playlist.m3u", middleware.RequireLineupMediaAccess("lineup_id"), middleware.RequestBudget(12, "playlist_output"), controllers.GetDynamicM3U)
	media.Get("/lineups/:lineup_id/guide.xml", middleware.RequireLineupMediaAccess("lineup_id"), middleware.RequestBudget(12, "guide_output"), controllers.GetDynamicXMLTV)

	short := a.Group("", middleware.DeclareRoutePolicy("media-output-alias"))
	short.Get("/m/:code", middleware.RequireMediaOutputAlias(), middleware.RequestBudget(12, "playlist_output"), controllers.GetShortM3U)
	short.Get("/x/:code", middleware.RequireMediaOutputAlias(), middleware.RequestBudget(12, "guide_output"), controllers.GetShortXMLTV)
}
