package routes

import (
	"xivi/backend/app/controllers"

	"github.com/gofiber/fiber/v2"
)

// PublicRoutes func for describe group of public routes.
func PublicRoutes(a *fiber.App) {
	// Create routes group.
	route := a.Group("/api")

	// Routes for GET method:
	route.Get("/token/new", controllers.GetNewAccessToken)                                             // create a new access tokens
	route.Get("/playlists", controllers.GetPlaylists)                                                  // get list of all playlists
	route.Get("/playlist/:id", controllers.GetPlaylist)                                                // get one playlist by ID
	route.Get("/playlist/:id/groups", controllers.GetPlaylistGroups)                                   // get groups by playlist_id
	route.Get("/playlist/:playlist_id/group/:group_id/channels", controllers.GetPlaylistGroupChannels) // get channels by playlist and group
	route.Post("/playlist", controllers.CreatePlaylist)                                                // create a new playlist
	route.Post("/playlist/:playlist_id/group/:group_id/convert", controllers.ConvertPlaylistGroup)     // convert playlistgroup
	route.Post("/m3u/:id", controllers.CreateM3U)                                                      // create m3u from template id
	route.Post("/template", controllers.CreateTemplate)                                                // create a new template
	route.Delete("/template/:template_id/group/:group_id/item", controllers.DeleteTemplateGroupItem)   // delete one playlist by ID
	route.Post("/epg", controllers.AddEpg)                                                             // Add a new Epg
	route.Post("/epg/create/:id", controllers.CreateEPG)                                               // Creat a new Epg xml
}
