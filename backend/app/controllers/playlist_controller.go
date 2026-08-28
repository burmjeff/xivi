package controllers

import (
	"context"
	"strconv"
	"time"

	"xivi/backend/app/models"
	"xivi/backend/pkg/security"
	"xivi/backend/pkg/streaming"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/cron"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
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
	// Get all playlists.
	playlists, err := database.Db.GetPlaylists()
	if err != nil {
		// Return, if playlists not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":     true,
			"msg":       "No playlists found",
			"count":     0,
			"playlists": nil,
		})
	}
	usage := make(map[int64]int)
	for _, source := range streaming.DefaultManager.ConnectionUsage() {
		usage[source.PoolID] = source.Active
	}
	for index := range *playlists {
		(*playlists)[index].ActiveConnections = usage[(*playlists)[index].ID]
		(*playlists)[index].URL = security.RedactProviderURL((*playlists)[index].URL)
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":     false,
		"msg":       nil,
		"count":     len(*playlists),
		"playlists": playlists,
	})
}

// GetPlaylist func gets playlist by given ID or 404 error.
// @Description Get playlist by given ID.
// @Summary get playlist by given ID
// @Tags Playlist
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

	// Get playlist by ID.
	playlist, err := database.Db.GetPlaylist(id)
	if err != nil {
		// Return, if playlist not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":    true,
			"msg":      "playlist with the given ID is not found",
			"playlist": nil,
		})
	}
	for _, source := range streaming.DefaultManager.ConnectionUsage() {
		if source.PoolID == playlist.ID {
			playlist.ActiveConnections = source.Active
			break
		}
	}
	playlist.URL = security.RedactProviderURL(playlist.URL)

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

	// Get playlist groups by ID.
	playlistGroups, err := database.Db.GetPlGroups(playlist_id)
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

// GetAllPlaylistGroups
// @Description Get all playlist groups
// @Summary get all playlist groups
// @Tags Playlist
// @Accept json
// @Produce json
// @Success 200 {array} models.PlaylistGroup
// @Router /playlist/groups/all [get]
func GetAllPlaylistGroups(c *fiber.Ctx) error {

	// Get playlist groups by ID.
	playlistGroups, err := database.Db.GetAllPlGroups()
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

// GetPlaylistChannels func gets playlist channels by group.
// @Description Get playlist channels by group
// @Summary get playlist channels by group
// @Tags Playlist
// @Produce json
// @Param playlist_id path string true "Playlist ID"
// @Param group_id path string true "Group ID"
// @Success 200 {array} models.PlaylistChannel
// @Router /playlist/{playlist_id}/group/{group_id}/channels [get]
func GetPlaylistGroupChannels(c *fiber.Ctx) error {
	_, err := strconv.ParseInt(c.Params("playlist_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	group_id, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get playlist channels by playlist group.
	channels, err := database.Db.GetPlGroupChannels(group_id)
	if err != nil {
		log.Error().Msgf("GetPLGroupChannels: %v", err)
		// Return, if playlist not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":          true,
			"msg":            "playlist groups not found",
			"playlistgroups": nil,
		})
	}
	for index := range channels {
		scopeAdminPlaylistChannelLogo(&channels[index])
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
// @Param playlist body models.Playlist true "Playlist"
// @Success 200 {object} models.Playlist
// @Router /playlist [post]
func CreatePlaylist(c *fiber.Ctx) error {
	// Create new Playlist struct
	playlist := &models.Playlist{}

	// Check, if received JSON data is valid.
	if err := decodeStrict(c, playlist); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create a new validator for a Playlist model.
	validate := utils.NewValidator()

	// Set initialized default data for playlist:
	playlist.CreatedAt = time.Now()
	playlist.UpdatedAt = time.Now()
	if playlist.ConnectionLimit == 0 {
		playlist.ConnectionLimit = 1
	}
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
	id, err := database.Db.CreatePlaylist(*playlist)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	playlist.ID = id

	//TODO async Parse m3u and insert channels
	m3uParser := utils.M3uParser{}
	providerPlaylist := *playlist
	go func() {
		if err := m3uParser.ParseM3u(providerPlaylist); err != nil {
			log.Error().Err(err).Int64("playlist_id", playlist.ID).Msg("Initial playlist import failed")
		}
	}()
	playlist.URL = security.RedactProviderURL(playlist.URL)

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"playlist": playlist,
	})
}

// UpdatePlaylist func to update a playlist by given ID.
// @Description Update playlist.
// @Summary update playlist
// @Tags Playlist
// @Accept json
// @Produce json
// @Param playlist body models.Playlist true "Playlist"
// @Success 201 {string} status "ok"
// @Router /playlist [put]
func UpdatePlaylist(c *fiber.Ctx) error {
	// Create new Playlist struct
	playlist := &models.Playlist{}

	// Check, if received JSON data is valid.
	if err := decodeStrict(c, playlist); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Checking, if playlist with given ID is exists.
	foundPlaylist, err := database.Db.GetPlaylist(playlist.ID)
	if err != nil {
		// Return status 404 and playlist not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "playlist with this ID not found",
		})
	}

	// Set initialized default data for playlist:
	playlist.UpdatedAt = time.Now()
	// Older API clients do not send the additive connection_limit field. Keep
	// their existing provider budget instead of turning a routine edit into a
	// validation failure or silently changing the limit.
	if playlist.ConnectionLimit == 0 {
		playlist.ConnectionLimit = foundPlaylist.ConnectionLimit
	}
	if playlist.URL == "" {
		playlist.URL = foundPlaylist.URL
	}

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
	if err := database.Db.UpdatePlaylist(foundPlaylist.ID, playlist); err != nil {
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

	// Delete playlist by given ID.
	if err := database.Db.DeletePlaylist(playlist_id); err != nil {
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
// @Param playlist_id path string true "Group ID"
// @Param group_id path string true "Group ID"
// @Param templategroup body models.TemplateGroup true "TemplateGroup"
// @Success 200 {object} models.TemplateGroup
// @Router /playlist/{playlist_id}/group/{group_id}/convert [post]
func ConvertPlaylistGroup(c *fiber.Ctx) error {
	// Create new Template struct
	templateGroup := &models.TemplateGroup{}

	_, err := strconv.ParseInt(c.Params("playlist_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	group_id, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Check, if received JSON data is valid.
	if err := decodeStrict(c, templateGroup); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
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

	playlistChannels, err := database.Db.GetPlGroupChannels(group_id)
	if err != nil {
		// Return, if playlistgroup not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":           true,
			"msg":             "playlist group with the given ID is not found",
			"playlistGroupId": group_id,
		})
	}

	// Create template group.
	tmplGroupID, err := database.Db.CreateTmplGroup(templateGroup)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	go utils.ConvertPlGroup(tmplGroupID, playlistChannels)

	templateGroup.ID = tmplGroupID

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":         false,
		"msg":           nil,
		"templategroup": templateGroup,
	})
}

// RefreshPlaylist func to manually refresh/update a playlist by given ID.
// @Description Manually refresh/update a playlist by given ID.
// @Summary manually refresh/update a playlist by given ID
// @Tags Playlist
// @Produce json
// @Param id path string true "Playlist ID"
// @Success 200 {string} status "ok"
// @Router /playlist/{playlist_id}/refresh [post]
func RefreshPlaylist(c *fiber.Ctx) error {
	// Catch playlist ID from URL.
	playlist_id, err := strconv.ParseInt(c.Params("playlist_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get the playlist by ID
	playlist, err := database.Db.GetPlaylist(playlist_id)
	if err != nil {
		// Return status 404 and playlist not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "playlist with this ID not found",
		})
	}

	// Parse the M3U file to update the playlist
	m3uParser := utils.M3uParser{}
	go func() {
		// Capture start time BEFORE parsing begins - this represents the baseline
		// for determining which channels existed before this update
		startTime := time.Now()
		log.Info().Int64("playlist_id", playlist.ID).Time("start_time", startTime).Msg("Starting playlist refresh")

		// Parse the M3U file - this will update timestamps for existing channels
		// and create new channels with current timestamps
		if err := m3uParser.ParseM3u(*playlist); err != nil {
			log.Error().Err(err).Int64("playlist_id", playlist.ID).Msg("Playlist refresh failed; retaining the previous snapshot")
			return
		}

		// Clean up stale channels - any channel not touched during parsing
		// will have timestamps before startTime and will be removed
		cron.CleanPlaylist(*playlist, startTime)
		if _, err := database.Db.SyncSourceGroupsForPlaylist(context.Background(), playlist.ID); err != nil {
			log.Error().Err(err).Int64("playlist_id", playlist.ID).Msg("One or more connected groups could not be synced")
		}

		log.Info().Int64("playlist_id", playlist.ID).Msg("Playlist refresh completed")
	}()

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   "Playlist refresh started",
	})
}

// ConvertPlaylistChannel func for converting a playlist channel into a template channel.
// @Summary Convert a playlist channel into a template channel.
// @Description Convert a playlist channel into a template channel.
// @Tags Playlist
// @Produce json
// @Param channel_id path int64 true "Channel ID"
// @Param group_id path int64 true "Group ID"
// @Param templatechannel body models.TemplateChannel true "TemplateChannel"
// @Success 200 {object} models.TemplateChannel
// @Router /playlist/channel/{channel_id}/convert/{group_id} [post]
func ConvertPlaylistChannel(c *fiber.Ctx) error {
	channel_id, err := strconv.ParseInt(c.Params("channel_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	group_id, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	playlistChannel, err := database.Db.GetPlChannel(channel_id)
	if err != nil {
		// Return, if playlistchannel not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":             true,
			"msg":               "playlist channel with the given ID is not found",
			"playlistChannelId": channel_id,
		})
	}

	channelID := utils.ConvertPlChannel(*playlistChannel)
	tmplGroupChannel := models.TemplateGroupChannel{GroupId: group_id, ChannelId: channelID}

	if err := database.Db.CreateTmplGroupChannel(tmplGroupChannel); err != nil {
		log.Warn().Msg(err.Error())
		// Return, if tmplGroupChannel not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":             true,
			"msg":               "Failed to create tmplGroupChannel",
			"templateChannelId": channel_id,
		})
	}

	templateChannel, err := database.Db.GetTmplChannel(channelID)
	if err != nil {
		// Return, if templatechannel not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":             true,
			"msg":               "template channel with the given ID is not found",
			"templateChannelId": channel_id,
		})
	}
	channelLogo := models.TemplateChannelLogo{}
	channelLogo.TemplateChannel = *templateChannel
	// Get logo.
	logo, _ := database.Db.GetLogo(context.Background(), templateChannel.LogoId)
	channelLogo.Logo = utils.GetLogoUrl(logo.Name)

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":           false,
		"msg":             nil,
		"templatechannel": channelLogo,
	})
}

// UpdatePlaylistGroup func to update a Playlist group.
// @Description Update Playlist Group.
// @Summary update Playlist Group
// @Tags Playlist Group
// @Accept json
// @Param playlistgroup body models.PlaylistGroup true "Playlist group"
// @Success 201 {string} status "ok"
// @Router /playlist/group [put]
func UpdatePlaylistGroup(c *fiber.Ctx) error {

	playlistGroup := &models.PlaylistGroup{}

	// Check, if received JSON data is valid.
	if err := decodeStrict(c, playlistGroup); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create a new validator for a Playlist model.
	validate := utils.NewValidator()

	// Validate playlist fields.
	if err := validate.Struct(playlistGroup); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	_, err := database.Db.GetPlGroup(playlistGroup.ID)
	if err != nil {
		log.Err(err)
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Update playlist group.
	if err := database.Db.UpdatePlGroup(playlistGroup); err != nil {
		log.Err(err)
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 201.
	return c.SendStatus(fiber.StatusCreated)
}
