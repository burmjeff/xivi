package routes

import (
	"time"
	"xivi/backend/app/controllers"
	"xivi/backend/pkg/middleware"

	"github.com/gofiber/fiber/v2"
)

// PublicRoutes func for describe group of public routes.
func PublicRoutes(a *fiber.App) {
	a.Get("/healthz", middleware.DeclareRoutePolicy("anonymous"), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "mobile_api_version": 1})
	})
	// Create routes group.
	api := a.Group("/api", middleware.DeclareRoutePolicy("admin"), middleware.RequireAdmin(), middleware.RequirePasswordChanged(), middleware.BoundedRateLimit(240, time.Minute, true), middleware.CSRFProtected(), middleware.SanitizeLegacyServerErrors())
	router := a.Group("/")

	// Playlist Routes
	api.Get("/playlists", controllers.GetPlaylists)                                                  // get list of all playlists
	api.Get("/playlist/:id", controllers.GetPlaylist)                                                // get a playlist by ID
	api.Get("/playlist/:id/groups", controllers.GetPlaylistGroups)                                   // get groups by playlist_id
	api.Get("/playlist/groups/all", controllers.GetAllPlaylistGroups)                                // get playlist groups
	api.Get("/playlist/:playlist_id/group/:group_id/channels", controllers.GetPlaylistGroupChannels) // get channels by playlist group
	api.Post("/playlist", controllers.CreatePlaylist)                                                // create a new playlist
	api.Post("/playlist/:playlist_id/group/:group_id/convert", controllers.ConvertPlaylistGroup)     // convert playlistgroup
	api.Post("/playlist/channel/:channel_id/convert/:group_id", controllers.ConvertPlaylistChannel)  // convert playlistchannel
	api.Post("/playlist/:playlist_id/refresh", controllers.RefreshPlaylist)                          // manually refresh a playlist
	api.Delete("/playlist/:playlist_id", controllers.DeletePlaylist)                                 // delete playlist by ID
	api.Put("/playlist/group", controllers.UpdatePlaylistGroup)                                      // update playlist group
	api.Put("/playlist", controllers.UpdatePlaylist)                                                 // update playlist

	// Template Routes
	api.Get("/templates", controllers.GetTemplates)                                                // get all templates
	api.Get("/template/:template_id", controllers.GetTemplate)                                     // get a template by ID
	api.Post("/template", controllers.CreateTemplate)                                              // create a new template
	api.Put("/template", controllers.UpdateTemplate)                                               // update a template
	api.Delete("/template/:template_id", controllers.DeleteTemplate)                               // delete a template by ID
	api.Get("/template/:template_id/groups", controllers.GetTemplateGroups)                        // get groups by template_id
	api.Post("/template/:template_id/group/:group_id/item", controllers.CreateTemplateGroupItem)   // create a templategroupitem by ID
	api.Delete("/template/:template_id/group/:group_id/item", controllers.DeleteTemplateGroupItem) // delete a templategroupitem by ID

	// Template Group Routes
	api.Get("/template/groups/all", controllers.GetGroups)                   // get all template groups
	api.Post("/template/group", controllers.CreateTemplateGroup)             // create a new template group
	api.Put("/template/group", controllers.UpdateTemplateGroup)              // update a template group
	api.Delete("/template/group/:group_id", controllers.DeleteTemplateGroup) // delete a template group by ID

	// Template Channel Routes
	api.Get("/template/group/:group_id/channels", controllers.GetTemplateGroupChannels)     // Get channels by template group
	api.Post("/template/group/:group_id/channel", controllers.CreateTemplateChannel)        // Create a template channel
	api.Put("/template/channel", controllers.UpdateTemplateChannel)                         // Update a template channel
	api.Delete("/template/channel/:channel_id", controllers.DeleteTemplateChannel)          // Delete a template channel
	api.Get("/template/channel/:channel_id/items", controllers.GetTemplateChannelItems)     // Get channel items
	api.Delete("/template/chan/item", controllers.DeleteTemplateChannelItem)                // Delete channel item
	api.Get("/template/channel/:channel_id/matches", controllers.GetTemplateChannelMatches) // Get channel matches
	api.Post("/template/channel/:channel_id/match/:match_id", controllers.AddChannelMatch)  // Add channel match

	//M3U Routes
	api.Post("/m3u/:template_id", controllers.CreateM3U) // create m3u from template id

	// EPG Routes
	api.Get("/epgs", controllers.GetEpgs)                    // Get all Epgs
	api.Post("/epg", controllers.AddEpg)                     // Add a new Epg
	api.Put("/epg", controllers.UpdateEpg)                   // Modify Epg
	api.Post("/epg/create/:epg_id", controllers.CreateEpg)   // Generate a new Epg xmltv xml
	api.Delete("/epg/:epg_id", controllers.DeleteEpg)        // Delete epg
	api.Post("/epg/:epg_id/refresh", controllers.RefreshEpg) // Manually refresh an EPG
	api.Get("/epg/tvgids", controllers.GetEpgTvgids)         // get epg channel ids

	// Logo Routes
	api.Get("/logos", controllers.GetLogos)                                                        // get list of all logos
	api.Get("/logo/:logoid", controllers.GetLogo)                                                  // get a logo by ID
	api.Post("/logo", middleware.BoundedRateLimit(10, time.Minute, false), controllers.UploadLogo) // create a new logo
	api.Delete("/logo/:logoid", controllers.DeleteLogo)                                            // delete a logo by ID

	// Settings Routes
	api.Get("/settings", controllers.GetSettings)    // get settings
	api.Put("/settings", controllers.UpdateSettings) // update settings

	// System Status Routes
	api.Get("/system/status", controllers.GetSystemStatus) // get system status metrics

	// Stream Routes
	router.Get("/stream/:stream_id", middleware.DeclareRoutePolicy("media-or-viewer"), middleware.RequirePlayback(), controllers.GetStream)
	router.Get("/stream/hls/:stream_id", middleware.DeclareRoutePolicy("media-or-viewer"), middleware.RequirePlayback(), controllers.GetHlsStream)
	router.Get("/stream/hls/:stream_id/:asset", middleware.DeclareRoutePolicy("media-or-viewer"), middleware.RequirePlayback(), controllers.GetHlsAsset)
	api.Get("/channels/hls/:group_id", controllers.GetHlsChannels)

	// Each enabled lineup is exposed as an independent virtual network tuner.
	tuner := router.Group("/virtual-tuner", middleware.DeclareRoutePolicy("trusted-lan-device"), middleware.RequireTrustedLAN())
	tuner.Use(func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "private, no-store")
		return c.Next()
	})
	tuner.Get("/:lineup_id/discover.json", controllers.GetVirtualTunerDiscover)
	tuner.Get("/:lineup_id/lineup_status.json", controllers.GetVirtualTunerLineupStatus)
	tuner.Get("/:lineup_id/lineup.json", controllers.GetVirtualTunerLineup)
	tuner.Post("/:lineup_id/lineup.post", controllers.PostVirtualTunerLineup)
	tuner.Get("/:lineup_id/device.xml", controllers.GetVirtualTunerDeviceDescription)

	// App Routes
	router.Get("/images/:asset", middleware.DeclareRoutePolicy("lineup-media-or-viewer"), middleware.RequireImageAccess(), middleware.RequestBudget(1, "image"), controllers.GetSecuredImage)
}
