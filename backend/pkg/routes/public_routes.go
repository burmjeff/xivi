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
	route.Get("/playlists", controllers.GetPlaylists)                           // get list of all playlists
	route.Get("/playlist/:id", controllers.GetPlaylist)                         // get one playlist by ID
	route.Get("/token/new", controllers.GetNewAccessToken)                      // create a new access tokens
	route.Post("/playlist", controllers.CreatePlaylist)                         // create a new playlist
	route.Post("/playlist/group/convert/:id", controllers.ConvertPlaylistGroup) // convert playlistgroup
	route.Post("/m3u/:id", controllers.CreateM3U)                               // create m3u from template id
	route.Post("/template", controllers.CreateTemplate)                         // create a new template
	route.Delete("/template/item/:id", controllers.DeleteTemplateItem)          // delete one playlist by ID
}
