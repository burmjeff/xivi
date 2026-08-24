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
