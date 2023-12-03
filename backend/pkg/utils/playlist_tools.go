package utils

import (
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

type PlaylistTools struct {
	Db *database.Queries
}

// Convert all playlist channels in playlist group to template channels attached to given group name.
func (p *PlaylistTools) ConvertPlGroup(templateGroup int64, playlistChannels []models.PlaylistChannel) {
	for _, channel := range playlistChannels {
		channelID := ConvertPlChannel(p.Db, channel)
		tmplGroupChannel := &models.TemplateGroupChannel{GroupId: templateGroup, ChannelId: channelID}
		err := p.Db.CreateTmplGroupChannel(tmplGroupChannel)
		if err != nil {
			log.Warn().Msg(err.Error())
		}
	}
}

// Convert a playlist channel to a template channel
func ConvertPlChannel(db *database.Queries, playlistChannel models.PlaylistChannel) int64 {
	validate := NewValidator()
	templateChannel := &models.TemplateChannel{}
	tmplChannelItem := &models.TemplateChannelItem{}

	if playlistChannel.Title != "" {
		templateChannel.Name = playlistChannel.Title
	}
	if playlistChannel.TvgID != "" {
		templateChannel.TvgID = playlistChannel.TvgID
	}
	if playlistChannel.Logo != "" {
		templateChannel.LogoId, _ = CreateLogo(db, playlistChannel.Logo)
	}
	templateChannel.Uuid = CreateUuid()

	// Validate playlist fields.
	if err := validate.Struct(templateChannel); err != nil {
		//Some fields are not valid.
		log.Warn().Msg(err.Error())
	} else {
		channelID, err := db.CreateTmplChannel(templateChannel)
		if err != nil {
			log.Warn().Msg(err.Error())
		} else {
			tmplChannelItem.ChannelId = channelID
			tmplChannelItem.PlaylistChannelId = playlistChannel.ID
			db.CreateTmplChannelItem(tmplChannelItem)
		}
		return channelID
	}
	return 0
}

func MatchDomain(db *database.Queries, playlistId int64) int64 {
	domainRegex := CompileRegex(`^(?:https?:\/\/)?(?:[^@\/\n]+@)?(?:www\.)?([^:\/?\n]+)`)

	newPlaylist, err := db.GetPlaylist(playlistId)
	if err != nil {
		// Return empty object and error.
		log.Err(err)
		return 0
	}

	newDomain := GetByRegex(domainRegex, newPlaylist.URL)

	playlists, err := db.GetPlaylists()
	if err != nil {
		// Return empty object and error.
		log.Err(err)
		return 0
	}

	for _, playlist := range playlists {
		domain := GetByRegex(domainRegex, playlist.URL)
		if newDomain == domain && newPlaylist.ID != playlist.ID {
			return playlist.ID
		}
	}
	return 0
}

func UpdateDynamicGroup(db *database.Queries, group models.TemplateGroup) {
	tmplChannels, err := db.GetTmplChannelsByGroup(group.ID)
	if err != nil {
		log.Debug().Msg(err.Error())
	}

	plChannels, err := db.GetPlGroupChannels(group.PlaylistGroup)
	if err != nil {
		log.Warn().Msg(err.Error())
	}

	for _, plChannel := range plChannels {
		foundChannel := false
		var tmplChannel models.TemplateChannel

		for _, tmplChannel := range tmplChannels {
			if plChannel.TvgID == tmplChannel.TvgID {
				tmplChannel.Name = plChannel.Title
				if err := db.UpdateTmplChannel(&tmplChannel); err != nil {
					log.Err(err)
				}
				foundChannel = true
				tmplChannel = tmplChannel
				break
			}
		}
		if !foundChannel {
			if err := db.DeleteTmplChannel(tmplChannel.ID); err != nil {
				log.Err(err)
			}
			channelID := ConvertPlChannel(db, plChannel)
			tmplGroupChannel := &models.TemplateGroupChannel{GroupId: group.ID, ChannelId: channelID}
			err := db.CreateTmplGroupChannel(tmplGroupChannel)
			if err != nil {
				log.Err(err)
			}
		}
	}
}
