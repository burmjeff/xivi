package utils

import (
	"sort"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

type MatchingTools struct {
	Db *database.Queries
}

func MatchPlaylistChannel(db *database.Queries, playlistCh *models.PlaylistChannel) {
	if settings.APP_SETTINGS.Playlist.Tvgid_match {
		if err := matchPlaylistTvgid(db, playlistCh); err == nil {
			return
		} else {
			log.Debug().Err(err)
		}
	}

	if settings.APP_SETTINGS.Playlist.Name_match {
		if err := matchPlaylistChannelName(db, playlistCh); err != nil {
			log.Debug().Err(err)
		}
	}

}

func MatchTemplateChannel(db *database.Queries, templateCh *models.TemplateChannel) {
	var addedChannels []models.PlaylistChannel
	var err error
	if settings.APP_SETTINGS.Playlist.Tvgid_match {
		if addedChannels, err = matchTemplateTvgid(db, templateCh); err != nil {
			log.Debug().Err(err)
		}
	}

	if settings.APP_SETTINGS.Playlist.Name_match {
		if err := matchTemplateChannelName(db, templateCh, addedChannels); err != nil {
			log.Debug().Err(err)
		}
	}

}

// Search if tvgid matches for playlist channel and add to template if match
func matchPlaylistTvgid(db *database.Queries, playlistCh *models.PlaylistChannel) error {
	playlistChUrl, err := db.GetChannelUrlByPlChannelID(playlistCh.ID)
	if err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
	}

	channels, err := db.GetTmplChannelsBytvgid(playlistCh.TvgID)
	if err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
		return err
	}
	for _, channel := range channels {
		if !itemExists(db, channel.ID, playlistChUrl.Url) {
			channelItem := models.TemplateChannelItem{
				ChannelId:         channel.ID,
				PlaylistChannelId: playlistCh.ID,
			}
			if _, err := db.CreateTmplChannelItem(&channelItem); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return err
			}
		}
	}
	return nil
}

func matchPlaylistChannelName(db *database.Queries, playlistCh *models.PlaylistChannel) error {
	//var channelVector models.ChannelVector
	var err error
	var chMatch int64
	var score float64

	playlistChUrl, err := db.GetChannelUrlByPlChannelID(playlistCh.ID)
	if err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
	}

	channelVector, err := db.GetChannelVectorByName(playlistCh.Title)
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		vectorId := UpdatePlaylistVector(db, playlistCh)
		if channelVector, err = db.GetChannelVector(vectorId); err != nil {
			log.Debug().Msgf("matchChannelName:, %v", err)
			return err
		}
	}

	templateVectors, err := db.GetTemplateChannelVectors()
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		return err
	}
	for _, templateVector := range templateVectors {
		vector, err := db.GetChannelVector(templateVector.VectorId)
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
		if !itemExists(db, chMatch, playlistChUrl.Url) {
			channelItem := models.TemplateChannelItem{
				ChannelId:         chMatch,
				PlaylistChannelId: playlistCh.ID,
			}
			if _, err := db.CreateTmplChannelItem(&channelItem); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return err
			}
		}
	}

	return nil
}

// Search if tvgid matches for template channel and add to template if match
func matchTemplateTvgid(db *database.Queries, templateCh *models.TemplateChannel) ([]models.PlaylistChannel, error) {
	channels, err := db.GetPlChannelsByTvgID(templateCh.TvgID)
	if err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
		return nil, err
	}

	for _, channel := range channels {
		playlistChUrl, err := db.GetChannelUrlByPlChannelID(channel.ID)
		if err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
		} else if itemExists(db, channel.ID, playlistChUrl.Url) {
			continue
		}

		channelItem := models.TemplateChannelItem{
			ChannelId:         templateCh.ID,
			PlaylistChannelId: channel.ID,
		}
		if _, err := db.CreateTmplChannelItem(&channelItem); err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
			continue
		}
	}

	return channels, nil
}

func matchTemplateChannelName(db *database.Queries, templateCh *models.TemplateChannel, addedChannels []models.PlaylistChannel) error {
	//var channelVector models.ChannelVector
	var err error

	channelVector, err := db.GetChannelVectorByName(templateCh.Name)
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		vectorId := UpdateTemplateVector(db, templateCh)
		if channelVector, err = db.GetChannelVector(vectorId); err != nil {
			log.Debug().Msgf("matchChannelName:, %v", err)
			return err
		}
	}

	playlistVectors, err := db.GetPlaylistChannelVectors()
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		return err
	}
	for _, playlistVector := range playlistVectors {
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

		vector, err := db.GetChannelVector(playlistVector.VectorId)
		if err != nil {
			continue
		}
		if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
			if cosine >= settings.APP_SETTINGS.Playlist.Name_score {
				playlistChUrl, err := db.GetChannelUrlByPlChannelID(playlistVector.ChannelId)
				if err != nil {
					log.Debug().Msgf("matchChannels:, %v", err)
				} else if itemExists(db, templateCh.ID, playlistChUrl.Url) {
					continue
				}
				channelItem := models.TemplateChannelItem{
					ChannelId:         templateCh.ID,
					PlaylistChannelId: playlistVector.ChannelId,
				}
				if _, err := db.CreateTmplChannelItem(&channelItem); err != nil {
					log.Debug().Msgf("matchChannels:, %v", err)
				}
			}
		}
	}

	return nil
}

func itemExists(db *database.Queries, tmplId int64, plUrl string) bool {
	if items, _ := db.GetTmplChannelItemsByCh(tmplId); len(items) > 0 {
		for _, item := range items {
			if itemUrl, err := db.GetChannelUrlByPlChannelID(item.PlaylistChannelId); err != nil {
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

func TemplateChannelMatches(db *database.Queries, templateCh *models.TemplateChannel) ([]models.VectorMatch, error) {
	//var channelVector models.ChannelVector
	var err error
	var vectorMatches []models.VectorMatch

	channelVector, err := db.GetChannelVectorByName(templateCh.Name)
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		vectorId := UpdateTemplateVector(db, templateCh)
		if channelVector, err = db.GetChannelVector(vectorId); err != nil {
			log.Debug().Msgf("matchChannelName:, %v", err)
			return nil, err
		}
	}

	playlistVectors, err := db.GetPlaylistChannelVectors()
	if err != nil {
		log.Debug().Msgf("matchChannelName:, %v", err)
		return nil, err
	}
	for _, playlistVector := range playlistVectors {
		vector, err := db.GetChannelVector(playlistVector.VectorId)
		if err != nil {
			continue
		}
		if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
			vectorMatch := models.VectorMatch{Id: playlistVector.ChannelId, Score: cosine}
			if len(vectorMatches) < 5 {
				vectorMatches = append(vectorMatches, vectorMatch)
			} else {
				lowestIdx := 0
				lowestValue := vectorMatches[0].Score

				for i := 1; i < len(vectorMatches); i++ {
					if vectorMatches[i].Score < lowestValue {
						lowestIdx = i
						lowestValue = vectorMatches[i].Score
					}
				}
				vectorMatches[lowestIdx] = vectorMatch
			}

			sort.Slice(vectorMatches, func(i, j int) bool {
				return vectorMatches[i].Score > vectorMatches[j].Score
			})

		}
	}

	return vectorMatches, nil
}
