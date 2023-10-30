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

	// Playlist Routes:
	api.Get("/token/new", controllers.GetNewAccessToken)                                             // create a new access tokens
	api.Get("/playlists", controllers.GetPlaylists)                                                  // get list of all playlists
	api.Get("/playlist/:id", controllers.GetPlaylist)                                                // get one playlist by ID
	api.Get("/playlist/:id/groups", controllers.GetPlaylistGroups)                                   // get groups by playlist_id
	api.Get("/playlist/:playlist_id/group/:group_id/channels", controllers.GetPlaylistGroupChannels) // get channels by playlist and group
	api.Post("/playlist", controllers.CreatePlaylist)                                                // create a new playlist
	api.Post("/playlist/:playlist_id/group/:group_id/convert", controllers.ConvertPlaylistGroup)     // convert playlistgroup

	// Template Routes
	api.Get("/templates", controllers.GetTemplates)                                                // get all templates
	api.Get("/template/:template_id", controllers.GetTemplate)                                     // get one template by ID
	api.Get("/template/:template_id/groups", controllers.GetTemplateGroups)                        // get groups by template_id
	api.Get("/template/groups/all", controllers.GetGroups)                                         // get all template groups
	api.Get("/template/group/:group_id/channels", controllers.GetTemplateGroupChannels)            // get channels by template and group
	api.Post("/template", controllers.CreateTemplate)                                              // create a new template
	api.Post("/template/group", controllers.CreateTemplateGroup)                                   // create a new template group
	api.Post("/template/:template_id/group/:group_id/item", controllers.CreateTemplateGroupItem)   // create one templategroupitem by ID
	api.Put("/template/group", controllers.UpdateTemplateGroup)                                    // update one template group by ID
	api.Delete("/template/:template_id/group/:group_id/item", controllers.DeleteTemplateGroupItem) // delete one templategroupitem by ID
	api.Delete("/template/group/:group_id", controllers.DeleteTemplateGroup)                       // delete one template group by ID

	//M3U Routes
	api.Post("/m3u/:id", controllers.CreateM3U) // create m3u from template id

	// EPG Routes
	api.Post("/epg", controllers.AddEpg)               // Add a new Epg
	api.Post("/epg/create/:id", controllers.CreateEPG) // Creat a new Epg xml

	// Logo Routes
	api.Get("/logos", controllers.GetLogos)              // get list of all logos
	api.Get("/logo/:logo_id", controllers.GetLogo)       // get one logo by ID
	api.Post("/logo", controllers.CreateLogo)            // create a new logo
	api.Delete("/logo/:logo_id", controllers.DeleteLogo) // delete one logo by ID

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
	router.Use("/epg", filesystem.New(filesystem.Config{
		Root:   http.Dir(settings.EPG_FILEPATH),
		Browse: false,
	}))
}
