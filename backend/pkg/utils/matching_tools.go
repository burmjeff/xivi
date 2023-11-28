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

func MatchChannel(db *database.Queries, playlistCh *models.PlaylistChannel) {
	if settings.APP_SETTINGS.Playlist.Tvgid_match {
		if err := matchChannelTvgid(db, playlistCh); err == nil {
			return
		}
	}

	if settings.APP_SETTINGS.Playlist.Name_match {
		if err := matchPlaylistChannelName(db, playlistCh); err == nil {
			return
		}
	}

}

// Search if tvgid matches for playlist channel and add to template if match
func matchChannelTvgid(db *database.Queries, playlistCh *models.PlaylistChannel) error {
	channel, err := db.GetTmplChannelBytvgid(playlistCh.TvgID)
	if err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
		return err
	}
	channelItem := models.TemplateChannelItem{
		ChannelId:         channel.ID,
		PlaylistChannelId: playlistCh.ID,
	}
	if _, err := db.CreateTmplChannelItem(&channelItem); err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
		return err
	}
	return nil
}

func matchPlaylistChannelName(db *database.Queries, playlistCh *models.PlaylistChannel) error {
	//var channelVector models.ChannelVector
	var err error
	var chMatch int64
	var score float64

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
	for _, templateVector := range *templateVectors {
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
		channelItem := models.TemplateChannelItem{
			ChannelId:         chMatch,
			PlaylistChannelId: playlistCh.ID,
		}
		if _, err := db.CreateTmplChannelItem(&channelItem); err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
			return err
		}
	}

	return nil
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
			vectorMatch := models.VectorMatch{ChannelId: playlistVector.ChannelId, Score: cosine}
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
