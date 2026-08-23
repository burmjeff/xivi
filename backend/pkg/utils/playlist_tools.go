package utils

import (
	"path/filepath"
	"strings"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

// Convert all playlist channels in playlist group to template channels attached to given group name.
func ConvertPlGroup(templateGroup int64, playlistChannels []models.PlaylistChannel) {
	for _, channel := range playlistChannels {
		channelID := ConvertPlChannel(channel)
		tmplGroupChannel := models.TemplateGroupChannel{GroupId: templateGroup, ChannelId: channelID}
		err := database.Db.CreateTmplGroupChannel(tmplGroupChannel)
		if err != nil {
			log.Warn().Msg(err.Error())
		}
	}
}

// Convert a playlist channel to a template channel
func ConvertPlChannel(playlistChannel models.PlaylistChannel) int64 {
	validate := NewValidator()
	templateChannel := &models.TemplateChannel{}

	if playlistChannel.Title != "" {
		templateChannel.Name = playlistChannel.Title
	}
	if playlistChannel.TvgID != nil {
		templateChannel.TvgID = playlistChannel.TvgID
	}
	if playlistChannel.Logo != nil {
		logoName := strings.TrimSuffix(filepath.Base(*playlistChannel.Logo), filepath.Ext(*playlistChannel.Logo))
		if logo := logoExists(logoName); logo != nil {
			templateChannel.LogoId = logo.ID
		} else {
			templateChannel.LogoId, _ = CreateLogo(*playlistChannel.Logo)

		}
	}
	templateChannel.Uuid = CreateUuid()

	// Validate playlist fields.
	if err := validate.Struct(templateChannel); err != nil {
		//Some fields are not valid.
		log.Warn().Msg(err.Error())
	} else {
		channelID, err := database.Db.CreateTmplChannel(*templateChannel)
		if err != nil {
			log.Warn().Msg(err.Error())
		} else {
			templateChannel.ID = channelID
			tmplChannelItem := &models.TemplateChannelItem{
				ChannelId:         channelID,
				PlaylistChannelId: playlistChannel.ID,
				MatchMethod:       "manual",
				MatcherVersion:    2,
				ManualLocked:      true,
			}
			database.Db.CreateTmplChannelItem(tmplChannelItem)

			MatchTemplateChannel(templateChannel)
		}
		return channelID
	}

	return 0
}

func MatchDomain(playlistId int64) int64 {
	domainRegex := CompileRegex(`^(?:https?:\/\/)?(?:[^@\/\n]+@)?(?:www\.)?([^:\/?\n]+)`)

	newPlaylist, err := database.Db.GetPlaylist(playlistId)
	if err != nil {
		// Return empty object and error.
		log.Err(err)
		return 0
	}

	newDomain := GetByRegex(domainRegex, newPlaylist.URL)

	playlists, err := database.Db.GetPlaylists()
	if err != nil {
		// Return empty object and error.
		log.Err(err)
		return 0
	}

	for _, playlist := range *playlists {
		domain := GetByRegex(domainRegex, playlist.URL)
		if newDomain == domain && newPlaylist.ID != playlist.ID {
			return playlist.ID
		}
	}
	return 0
}

func UpdateDynamicGroup(group models.TemplateGroup) {
	// Verify the group is dynamic and has a valid dynamicgroup
	if !group.Dynamic || group.DynamicGroup == nil {
		log.Debug().Int64("group_id", group.ID).Msg("Group is not dynamic or has no dynamicgroup")
		return
	}

	// Verify the playlist group still exists
	playlistGroup, err := database.Db.GetPlGroup(*group.DynamicGroup)
	if err != nil {
		log.Warn().Int64("group_id", group.ID).Int64("dynamicgroup_id", *group.DynamicGroup).Msg("Playlist group no longer exists, disabling dynamic connection")
		// Playlist group no longer exists, disable dynamic connection
		group.Dynamic = false
		group.DynamicGroup = nil
		if err := database.Db.UpdateTmplGroup(&group); err != nil {
			log.Err(err).Msg("Failed to update template group")
		}
		return
	}

	// Get template channels for this group
	tmplChannels, err := database.Db.GetTmplChannelsByGroup(group.ID)
	if err != nil {
		log.Debug().Int64("group_id", group.ID).Msg(err.Error())
		// Continue with empty slice rather than returning
		tmplChannels = []models.TemplateChannel{}
	}

	// Get playlist channels for the dynamic group
	plChannels, err := database.Db.GetPlGroupChannels(*group.DynamicGroup)
	if err != nil {
		log.Warn().Int64("dynamicgroup_id", *group.DynamicGroup).Msg(err.Error())
		return
	}

	// If the playlist group has no channels, log a warning but don't disconnect
	if len(plChannels) == 0 {
		log.Warn().Int64("group_id", group.ID).Int64("dynamicgroup_id", *group.DynamicGroup).Msg("Playlist group has no channels")
	}

	// Create a map of template channels by tvgID for faster lookups
	tmplChannelsByTvgID := make(map[string]*models.TemplateChannel, len(tmplChannels))
	for i := range tmplChannels {
		if tmplChannels[i].TvgID != nil {
			tmplChannelsByTvgID[*tmplChannels[i].TvgID] = &tmplChannels[i]
		}
	}

	// Create a map to track which template channels are still needed
	keepChannelIDs := make(map[int64]bool, len(tmplChannels))

	// Process each playlist channel
	for _, plChannel := range plChannels {
		// Skip channels without tvgID
		if plChannel.TvgID == nil {
			continue
		}

		// Check if we already have a matching template channel
		if tmplChannel, exists := tmplChannelsByTvgID[*plChannel.TvgID]; exists {
			// Update the template channel name if needed
			if tmplChannel.Name != plChannel.Title {
				tmplChannel.Name = plChannel.Title
				if err := database.Db.UpdateTmplChannel(*tmplChannel); err != nil {
					log.Err(err).Int64("channel_id", tmplChannel.ID).Msg("Failed to update template channel")
				}
			}
			// Mark this channel as needed
			keepChannelIDs[tmplChannel.ID] = true
		} else {
			// Create a new template channel
			channelID := ConvertPlChannel(plChannel)
			if channelID > 0 {
				tmplGroupChannel := models.TemplateGroupChannel{GroupId: group.ID, ChannelId: channelID}
				err := database.Db.CreateTmplGroupChannel(tmplGroupChannel)
				if err != nil {
					log.Err(err).Int64("group_id", group.ID).Int64("channel_id", channelID).Msg("Failed to create template group channel")
				}
				// Mark this new channel as needed
				keepChannelIDs[channelID] = true
			}
		}
	}

	// Remove template channels that are no longer needed
	for _, tmplChannel := range tmplChannels {
		if !keepChannelIDs[tmplChannel.ID] {
			if err := database.Db.DeleteTmplChannel(tmplChannel.ID); err != nil {
				log.Err(err).Int64("channel_id", tmplChannel.ID).Msg("Failed to delete template channel")
			}
		}
	}

	// Update the template group name if needed
	if group.Name != playlistGroup.Name {
		group.Name = playlistGroup.Name
		if err := database.Db.UpdateTmplGroup(&group); err != nil {
			log.Err(err).Int64("group_id", group.ID).Msg("Failed to update template group name")
		}
	}
}
