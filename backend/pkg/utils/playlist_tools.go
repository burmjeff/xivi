package utils

import (
	"path"
	"slices"
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
	tmplChannelItem := &models.TemplateChannelItem{}

	if playlistChannel.Title != "" {
		templateChannel.Name = playlistChannel.Title
	}
	if playlistChannel.TvgID != nil {
		templateChannel.TvgID = playlistChannel.TvgID
	}
	if playlistChannel.Logo != nil {
		logoName := strings.Split(path.Base(*playlistChannel.Logo), ".")[0]
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
			tmplChannelItem.ChannelId = channelID
			tmplChannelItem.PlaylistChannelId = playlistChannel.ID
			database.Db.CreateTmplChannelItem(tmplChannelItem)
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
	tmplChannels, err := database.Db.GetTmplChannelsByGroup(group.ID)
	if err != nil {
		log.Debug().Msg(err.Error())
	}

	plChannels, err := database.Db.GetPlGroupChannels(group.DynamicGroup)
	if err != nil {
		log.Warn().Msg(err.Error())
	}

	var foundChannels []models.TemplateChannel
	for _, plChannel := range *plChannels {
		foundChannel := false

		for _, tmplChannel := range tmplChannels {
			if plChannel.TvgID == tmplChannel.TvgID {
				tmplChannel.Name = plChannel.Title
				if err := database.Db.UpdateTmplChannel(tmplChannel); err != nil {
					log.Err(err)
				}
				foundChannel = true
				foundChannels = append(foundChannels, tmplChannel)
				break
			}
		}
		if !foundChannel {
			channelID := ConvertPlChannel(plChannel)
			tmplGroupChannel := models.TemplateGroupChannel{GroupId: group.ID, ChannelId: channelID}
			err := database.Db.CreateTmplGroupChannel(tmplGroupChannel)
			if err != nil {
				log.Err(err)
			}
		}
	}
	for _, tmplChannel := range tmplChannels {
		if !(slices.Contains(foundChannels, tmplChannel)) {
			if err := database.Db.DeleteTmplChannel(tmplChannel.ID); err != nil {
				log.Err(err)
			}
		}
	}
}
