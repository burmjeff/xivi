package utils

import (
	"sort"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

func MatchPlaylistChannel(playlistCh models.PlaylistChannel) {
	if settings.APP_SETTINGS.Playlist.Tvgid_match {
		if err := matchPlaylistTvgid(playlistCh); err == nil {
			return
		} else {
			log.Debug().Err(err)
		}
	}

	if settings.APP_SETTINGS.Playlist.Name_match {
		if err := matchPlaylistChannelName(playlistCh); err != nil {
			log.Debug().Err(err)
		}
	}

}

func MatchTemplateChannel(templateCh models.TemplateChannel) {
	var addedChannels []models.PlaylistChannel
	var err error
	if settings.APP_SETTINGS.Playlist.Tvgid_match {
		if addedChannels, err = matchTemplateTvgid(templateCh); err != nil {
			log.Debug().Err(err)
		}
	}

	if settings.APP_SETTINGS.Playlist.Name_match {
		if err := matchTemplateChannelName(templateCh, addedChannels); err != nil {
			log.Debug().Err(err)
		}
	}

}

// Search if tvgid matches for playlist channel and add to template if match
func matchPlaylistTvgid(playlistCh models.PlaylistChannel) error {
	playlistChUrl, err := database.Db.GetChannelUrlByPlChannelID(playlistCh.ID)
	if err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
	}

	channels, err := database.Db.GetTmplChannelsBytvgid(playlistCh.TvgID)
	if err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
		return err
	}
	for _, channel := range *channels {
		if playlistChUrl == nil || !itemExists(channel.ID, playlistChUrl.Url) {
			channelItem := models.TemplateChannelItem{
				ChannelId:         channel.ID,
				PlaylistChannelId: playlistCh.ID,
			}
			if _, err := database.Db.CreateTmplChannelItem(channelItem); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return err
			}
		}
	}
	return nil
}

func matchPlaylistChannelName(playlistCh models.PlaylistChannel) error {
	//var channelVector models.ChannelVector
	var err error
	var chMatch int64
	var score float64

	playlistChUrl, err := database.Db.GetChannelUrlByPlChannelID(playlistCh.ID)
	if err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
	}

	channelVector, err := database.Db.GetChannelVectorByName(playlistCh.Title)
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		vectorId := UpdatePlaylistVector(playlistCh)
		if channelVector, err = database.Db.GetChannelVector(vectorId); err != nil {
			log.Debug().Msgf("matchChannelName:, %v", err)
			return err
		}
	}

	templateVectors, err := database.Db.GetTemplateChannelVectors()
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		return err
	}
	for _, templateVector := range *templateVectors {
		vector, err := database.Db.GetChannelVector(templateVector.VectorId)
		if err != nil {
			continue
		}
		if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
			if cosine >= settings.APP_SETTINGS.Playlist.Name_score && cosine > score {
				score = cosine
				chMatch = templateVector.ChannelId
			}
		}
	}
	if chMatch != 0 {
		if !itemExists(chMatch, playlistChUrl.Url) {
			channelItem := models.TemplateChannelItem{
				ChannelId:         chMatch,
				PlaylistChannelId: playlistCh.ID,
			}
			if _, err := database.Db.CreateTmplChannelItem(channelItem); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return err
			}
		}
	}

	return nil
}

// Search if tvgid matches for template channel and add to template if match
func matchTemplateTvgid(templateCh models.TemplateChannel) ([]models.PlaylistChannel, error) {
	channels, err := database.Db.GetPlChannelsByTvgID(templateCh.TvgID)
	if err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
		return nil, err
	}

	for _, channel := range *channels {
		playlistChUrl, err := database.Db.GetChannelUrlByPlChannelID(channel.ID)
		if err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
		} else if itemExists(channel.ID, playlistChUrl.Url) {
			continue
		}

		channelItem := models.TemplateChannelItem{
			ChannelId:         templateCh.ID,
			PlaylistChannelId: channel.ID,
		}
		if _, err := database.Db.CreateTmplChannelItem(channelItem); err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
			continue
		}
	}

	return *channels, nil
}

func matchTemplateChannelName(templateCh models.TemplateChannel, addedChannels []models.PlaylistChannel) error {
	//var channelVector models.ChannelVector
	var err error

	channelVector, err := database.Db.GetChannelVectorByName(templateCh.Name)
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		vectorId := UpdateTemplateVector(templateCh)
		if channelVector, err = database.Db.GetChannelVector(vectorId); err != nil {
			log.Debug().Msgf("matchChannelName:, %v", err)
			return err
		}
	}

	playlistVectors, err := database.Db.GetPlaylistChannelVectors()
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		return err
	}
	for _, playlistVector := range *playlistVectors {
		isMatch := false
		for _, channel := range addedChannels {
			if playlistVector.ChannelId == channel.ID {
				isMatch = true
				break
			}
		}
		if isMatch {
			continue
		}

		vector, err := database.Db.GetChannelVector(playlistVector.VectorId)
		if err != nil {
			continue
		}
		if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
			if cosine >= settings.APP_SETTINGS.Playlist.Name_score {
				playlistChUrl, err := database.Db.GetChannelUrlByPlChannelID(playlistVector.ChannelId)
				if err != nil {
					log.Debug().Msgf("matchChannels:, %v", err)
				} else if itemExists(templateCh.ID, playlistChUrl.Url) {
					continue
				}
				channelItem := models.TemplateChannelItem{
					ChannelId:         templateCh.ID,
					PlaylistChannelId: playlistVector.ChannelId,
				}
				if _, err := database.Db.CreateTmplChannelItem(channelItem); err != nil {
					log.Debug().Msgf("matchChannels:, %v", err)
				}
			}
		}
	}

	return nil
}

func itemExists(tmplId int64, plUrl string) bool {
	if items, _ := database.Db.GetTmplChannelItemsByCh(tmplId); len(*items) > 0 {
		for _, item := range *items {
			if itemUrl, err := database.Db.GetChannelUrlByPlChannelID(item.PlaylistChannelId); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
			} else {
				if itemUrl.Url == plUrl {
					return true
				}
			}
		}
	}
	return false
}

func TopChannelMatches(templateCh models.TemplateChannel) ([]models.VectorMatch, error) {
	//var channelVector models.ChannelVector
	var err error
	var vectorMatches []models.VectorMatch
	lowestIdx := 0
	lowestValue := 2.0

	channelVector, err := database.Db.GetChannelVectorByName(templateCh.Name)
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		vectorId := UpdateTemplateVector(templateCh)
		if channelVector, err = database.Db.GetChannelVector(vectorId); err != nil {
			log.Debug().Msgf("matchChannelName:, %v", err)
			return nil, err
		}
	}

	playlistVectors, err := database.Db.GetPlaylistChannelVectors()
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		return nil, err
	}
	for _, playlistVector := range *playlistVectors {

		vector, err := database.Db.GetChannelVector(playlistVector.VectorId)
		if err != nil {
			continue
		}
		if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
			if len(vectorMatches) < 5 {
				vectorMatches = append(vectorMatches, models.VectorMatch{Id: playlistVector.ChannelId, Score: cosine})
				if cosine < lowestValue {
					lowestIdx = len(vectorMatches) - 1
					lowestValue = cosine
				}
			} else {
				if cosine > lowestValue {
					lowestValue = 2.0
					vectorMatches[lowestIdx] = models.VectorMatch{Id: playlistVector.ChannelId, Score: cosine}
					for i := 0; i < len(vectorMatches); i++ {
						if vectorMatches[i].Score < lowestValue {
							lowestIdx = i
							lowestValue = vectorMatches[i].Score
						}
					}
				}
			}

			sort.Slice(vectorMatches, func(i, j int) bool {
				return vectorMatches[i].Score > vectorMatches[j].Score
			})

		}
	}

	return vectorMatches, nil
}
