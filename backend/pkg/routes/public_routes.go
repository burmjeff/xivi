package routes

import (
	"net/http"
	"xivi/backend/app/controllers"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
)

// PublicRoutes func for describe group of public routes.
func PublicRoutes(a *fiber.App) {
	// Create routes group.
	api := a.Group("/api")
	router := a.Group("/")

	// Playlist Routes
	api.Get("/token/new", controllers.GetNewAccessToken)                                // create a new access tokens
	api.Get("/playlists", controllers.GetPlaylists)                                     // get list of all playlists
	api.Get("/playlist/:id", controllers.GetPlaylist)                                   // get a playlist by ID
	api.Get("/playlist/:id/groups", controllers.GetPlaylistGroups)                      // get groups by playlist_id
	api.Get("/playlist/group/:group_id/channels", controllers.GetPlaylistGroupChannels) // get channels by playlist group
	api.Post("/playlist", controllers.CreatePlaylist)                                   // create a new playlist
	api.Post("/playlist/group/:group_id/convert", controllers.ConvertPlaylistGroup)     // convert playlistgroup
	api.Delete("/playlist/:playlist_id", controllers.DeletePlaylist)                    // delete playlist by ID

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
	api.Get("/template/group/:group_id/channels", controllers.GetTemplateGroupChannels)     // get channels by template group
	api.Post("/template/group/:group_id/channel", controllers.CreateTemplateChannel)        // Create a template channel
	api.Put("/template/channel", controllers.UpdateTemplateChannel)                         // update a template channel
	api.Delete("/template/channel/:channel_id", controllers.DeleteTemplateChannel)          // Delete a template channel
	api.Get("/template/channel/:channel_id/items", controllers.GetTemplateChannelItems)     // get channel items
	api.Delete("/api/template/channel/item", controllers.DeleteTemplateChannelItem)         // Delete channel item
	api.Get("/template/channel/:channel_id/matches", controllers.GetTemplateChannelMatches) // get channel matches

	//M3U Routes
	api.Post("/m3u/:template_id", controllers.CreateM3U) // create m3u from template id

	// EPG Routes
	api.Get("/epgs", controllers.GetEpgs)                  // Get all Epgs
	api.Post("/epg", controllers.AddEpg)                   // Add a new Epg
	api.Post("/epg/create/:epg_id", controllers.CreateEpg) // Generate a new Epg xmltv xml
	api.Delete("/epg/:epg_id", controllers.DeleteEpg)      // Delete epg

	// Logo Routes
	api.Get("/logos", controllers.GetLogos)             // get list of all logos
	api.Get("/logo/:logoid", controllers.GetLogo)       // get a logo by ID
	api.Post("/logo", controllers.UploadLogo)           // create a new logo
	api.Delete("/logo/:logoid", controllers.DeleteLogo) // delete a logo by ID

	// Settings Routes
	api.Get("/settings", controllers.GetSettings)    // get settings
	api.Put("/settings", controllers.UpdateSettings) // update settings

	// Stream Routes
	router.Get("/stream/:stream_id", controllers.GetStream)

	// App Routes
	router.Use("/images", filesystem.New(filesystem.Config{
		Root:   http.Dir(settings.LOGO_FILEPATH),
		Browse: false,
	}))
	router.Use("/m3u", filesystem.New(filesystem.Config{
		Root:   http.Dir(settings.M3U_FILEPATH),
		Browse: false,
	}))
	router.Use("/xmltv", filesystem.New(filesystem.Config{
		Root:   http.Dir(settings.EPG_FILEPATH),
		Browse: false,
	}))
}
