package controllers

import (
	"strconv"
	"time"

	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
)

// GetPlaylists func gets all playlists.
// @Description Get all playlists.
// @Summary get all playlists
// @Tags Playlist
// @Accept json
// @Produce json
// @Success 200 {array} models.Playlist
// @Router /playlists [get]
func GetPlaylists(c *fiber.Ctx) error {
	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get all playlists.
	playlists, err := db.GetPlaylists()
	if err != nil {
		// Return, if playlists not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":     true,
			"msg":       "No playlists found",
			"count":     0,
			"playlists": nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":     false,
		"msg":       nil,
		"count":     len(playlists),
		"playlists": playlists,
	})
}

// GetPlaylist func gets playlist by given ID or 404 error.
// @Description Get playlist by given ID.
// @Summary get playlist by given ID
// @Tags Playlist
// @Accept json
// @Produce json
// @Param id path string true "Playlist ID"
// @Success 200 {object} models.Playlist
// @Router /playlist/{id} [get]
func GetPlaylist(c *fiber.Ctx) error {
	// Catch playlist ID from URL.
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get playlist by ID.
	playlist, err := db.GetPlaylist(id)
	if err != nil {
		// Return, if playlist not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":    true,
			"msg":      "playlist with the given ID is not found",
			"playlist": nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"playlist": playlist,
	})
}

// GetPlaylistGroups func gets playlist groups by given playlist ID or 404 error.
// @Description Get playlist groups by given playlist ID
// @Summary get playlist groups by given playlist ID
// @Tags Playlist
// @Accept json
// @Produce json
// @Param id path string true "Playlist ID"
// @Success 200 {array} models.PlaylistGroup
// @Router /playlist/{id}/groups [get]
func GetPlaylistGroups(c *fiber.Ctx) error {
	// Catch playlist ID from URL.
	playlist_id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get playlist groups by ID.
	playlistGroups, err := db.GetPlGroups(playlist_id)
	if err != nil {
		// Return, if playlist not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":          true,
			"msg":            "playlist groups not found",
			"playlistgroups": nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":          false,
		"msg":            nil,
		"playlistgroups": playlistGroups,
	})
}

// GetPlaylistChannels func gets playlist channels by given group ID.
// @Description Get playlist channels by given group ID
// @Summary get playlist channels by given group ID
// @Tags Playlist
// @Produce json
// @Param group_id path string true "Group ID"
// @Success 200 {array} models.PlaylistChannel
// @Router /playlist/group/{group_id}/channels [get]
func GetPlaylistGroupChannels(c *fiber.Ctx) error {
	// Catch group ID from URL.
	group_id, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get playlist channels by playlist group.
	channels, err := db.GetPlGroupChannels(group_id)
	if err != nil {
		// Return, if playlist not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":          true,
			"msg":            "playlist groups not found",
			"playlistgroups": nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":            false,
		"msg":              nil,
		"playlistchannels": channels,
	})
}

// CreatePlaylist func for createing a new playlist.
// @Summary Create a new playlist
// @Description Create a new playlist and parse m3u.
// @Tags Playlist
// @Accept json
// @Produce json
// @Param playlist body models.PlaylistCreateParam true "Playlist"
// @Success 200 {object} models.Playlist
// @Security ApiKeyAuth
// @Router /playlist [post]
func CreatePlaylist(c *fiber.Ctx) error {
	// Create new Playlist struct
	playlist := &models.Playlist{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(playlist); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create a new validator for a Playlist model.
	validate := utils.NewValidator()

	// Set initialized default data for playlist:
	playlist.CreatedAt = time.Now()
	//TODO IF Enable on new channel add: playlist.Enabled = 1 // 0 == inactive, 1 == active

	// Validate playlist fields.
	if err := validate.Struct(playlist); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Create playlist.
	id, err := db.CreatePlaylist(playlist)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	playlist.ID = id

	//TODO async Parse m3u and insert channels
	m3uParser := utils.M3uParser{Db: db}
	go m3uParser.ParseM3u(id, playlist.URL)

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"playlist": playlist,
	})
}

// UpdatePlaylist func for updates playlist by given ID.
// @Description Update playlist.
// @Summary update playlist
// @Tags Playlist
// @Accept json
// @Produce json
// @Param id body string true "Playlist ID"
// @Param name body string true "Name"
// @Param url body string true "URL"
// @Success 201 {string} status "ok"
// @Security ApiKeyAuth
// @Router /playlist [put]
func UpdatePlaylist(c *fiber.Ctx) error {
	// Get now time.
	now := time.Now().Unix()

	// Get claims from JWT.
	claims, err := utils.ExtractTokenMetadata(c)
	if err != nil {
		// Return status 500 and JWT parse error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Set expiration time from JWT data of current playlist.
	expires := claims.Expires

	// Checking, if now time greather than expiration from JWT.
	if now > expires {
		// Return status 401 and unauthorized error message.
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "unauthorized, check expiration time of your token",
		})
	}

	// Create new Playlist struct
	playlist := &models.Playlist{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(playlist); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Checking, if playlist with given ID is exists.
	foundPlaylist, err := db.GetPlaylist(playlist.ID)
	if err != nil {
		// Return status 404 and playlist not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "playlist with this ID not found",
		})
	}

	// Set initialized default data for playlist:
	playlist.UpdatedAt = time.Now()

	// Create a new validator for a Playlist model.
	validate := utils.NewValidator()

	// Validate playlist fields.
	if err := validate.Struct(playlist); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Update playlist by given ID.
	if err := db.UpdatePlaylist(foundPlaylist.ID, playlist); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 201.
	return c.SendStatus(fiber.StatusCreated)
}

// DeletePlaylist func to delete a playlist by given ID.
// @Description Delete playlist by given ID.
// @Summary delete playlist by given ID
// @Tags Playlist
// @Param id body string true "Playlist ID"
// @Success 204 {string} status "ok"
// @Router /playlist/{playlist_id} [delete]
func DeletePlaylist(c *fiber.Ctx) error {
	// Catch playlist ID from URL.
	playlist_id, err := strconv.ParseInt(c.Params("playlist_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Checking, if playlist with given ID is exists.
	foundPlaylist, err := db.GetPlaylist(playlist_id)
	if err != nil {
		// Return status 404 and playlist not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "playlist with this ID not found",
		})
	}

	// Delete playlist by given ID.
	if err := db.DeletePlaylist(foundPlaylist.ID); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}

// ConvertPlaylistGroup func for converting a playlist group into a template group.
// @Summary Convert a playlist group into a template group.
// @Description Convert a playlist group into a template group.
// @Tags Playlist
// @Produce json
// @Param group_id path string true "Group ID"
// @Param templategroup body models.TemplateGroupCreateParam true "TemplateGroup"
// @Success 200 {object} models.TemplateGroup
// @Router /playlist/group/{group_id}/convert [post]
func ConvertPlaylistGroup(c *fiber.Ctx) error {
	// Create new Template struct
	templateGroup := &models.TemplateGroup{}

	group_id, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(templateGroup); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create a new validator for a Template model.
	validate := utils.NewValidator()

	// Validate template fields.
	if err := validate.Struct(templateGroup); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	playlistChannels, err := db.GetPlGroupChannels(group_id)
	if err != nil {
		// Return, if playlistgroup not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":           true,
			"msg":             "playlist group with the given ID is not found",
			"playlistGroupId": group_id,
		})
	}

	// Create template group.
	tmplGroupID, err := db.CreateTmplGroup(templateGroup)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	playlistTools := utils.PlaylistTools{Db: db}
	go playlistTools.ConvertPlGroup(tmplGroupID, playlistChannels)

	templateGroup.ID = tmplGroupID

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":         false,
		"msg":           nil,
		"templategroup": templateGroup,
	})
}
