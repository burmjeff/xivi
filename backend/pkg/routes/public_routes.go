package routes

import (
	"xivi/backend/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// PublicRoutes func for describe group of public routes.
func PublicRoutes(a *fiber.App) {
	// Create routes group.
	router := a.Group("/api")

	// Playlist Routes:
	router.Get("/token/new", controllers.GetNewAccessToken)                                             // create a new access tokens
	router.Get("/playlists", controllers.GetPlaylists)                                                  // get list of all playlists
	router.Get("/playlist/:id", controllers.GetPlaylist)                                                // get one playlist by ID
	router.Get("/playlist/:id/groups", controllers.GetPlaylistGroups)                                   // get groups by playlist_id
	router.Get("/playlist/:playlist_id/group/:group_id/channels", controllers.GetPlaylistGroupChannels) // get channels by playlist and group
	router.Post("/playlist", controllers.CreatePlaylist)                                                // create a new playlist
	router.Post("/playlist/:playlist_id/group/:group_id/convert", controllers.ConvertPlaylistGroup)     // convert playlistgroup
	router.Post("/m3u/:id", controllers.CreateM3U)                                                      // create m3u from template id

	// Template Routes
	router.Get("/templates", controllers.GetTemplates)                                                // get list of all templates
	router.Get("/template/:template_id", controllers.GetTemplate)                                     // get one template by ID
	router.Get("/template/:template_id/groups", controllers.GetTemplateGroups)                        // get groups by template_id
	router.Get("/template/groups/all", controllers.GetAllTemplateGroups)                              // get groups by template_id
	router.Get("/template/group/:group_id/channels", controllers.GetTemplateGroupChannels)            // get channels by template and group
	router.Post("/template", controllers.CreateTemplate)                                              // create a new template
	router.Post("/template/group", controllers.CreateTemplateGroup)                                   // create a new template
	router.Delete("/template/:template_id/group/:group_id/item", controllers.DeleteTemplateGroupItem) // delete one template by ID

	// EPG Routes
	router.Post("/epg", controllers.AddEpg)               // Add a new Epg
	router.Post("/epg/create/:id", controllers.CreateEPG) // Creat a new Epg xml

}
