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
		channelID := p.ConvertPlChannel(channel)
		tmplGroupChannel := &models.TemplateGroupChannel{GroupId: templateGroup, ChannelId: channelID}
		err := p.Db.CreateTmplGroupChannel(tmplGroupChannel)
		if err != nil {
			log.Warn().Msg(err.Error())
		}
	}
}

// Convert a playlist channel to a template channel
func (p *PlaylistTools) ConvertPlChannel(playlistChannel models.PlaylistChannel) int64 {
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
		templateChannel.LogoId, _ = CreateLogo(p.Db, playlistChannel.Logo)
	}
	templateChannel.Uuid = CreateUuid()

	// Validate playlist fields.
	if err := validate.Struct(templateChannel); err != nil {
		//Some fields are not valid.
		log.Warn().Msg(err.Error())
	} else {
		channelID, err := p.Db.CreateTmplChannel(templateChannel)
		if err != nil {
			log.Warn().Msg(err.Error())
		} else {
			tmplChannelItem.ChannelId = channelID
			tmplChannelItem.PlaylistChannelId = playlistChannel.ID
			p.Db.CreateTmplChannelItem(tmplChannelItem)
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
