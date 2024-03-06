package utils

import (
	"errors"
	"math"
	"sort"
	"sync"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

type vectorMatches struct {
	matches []models.VectorMatch
	Mu      sync.Mutex
}

func MatchPlaylistChannel(playlistCh models.PlaylistChannel) {
	//TODO SHOULD I ONLY MATCH IF ONE DOESN'T ALREADY EXIST??
	if playlistCh.Enabled {
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
}

func MatchTemplateChannel(templateCh *models.TemplateChannel) {
	addedChannels := &[]models.PlaylistChannel{}
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
	if playlistCh.TvgID == nil {
		return errors.New("tvgid is nil")
	}
	playlistChUrl, err := database.Db.GetChannelUrl(playlistCh.ID)
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
			channelItem := &models.TemplateChannelItem{
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
	var wg sync.WaitGroup
	var err error
	var chMatch int64
	var score float64

	playlistChUrl, err := database.Db.GetChannelUrl(playlistCh.ID)
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

	var chunks [][]models.TemplateChannelVector
	chunkSize := int(math.RoundToEven(float64(len(templateVectors)/10))) + 1
	for i := 0; i < len(templateVectors); i += chunkSize {
		end := i + chunkSize
		if end > len(templateVectors) {
			end = len(templateVectors)
		}
		chunks = append(chunks, templateVectors[i:end])
	}

	for _, chunk := range chunks {
		wg.Add(1)
		go func(chunk []models.TemplateChannelVector) {
			defer wg.Done()
			for _, vec := range chunk {
				vector, err := database.Db.GetChannelVector(vec.VectorId)
				if err == nil {
					if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
						if cosine >= settings.APP_SETTINGS.Playlist.Name_score && cosine > score {
							score = cosine
							chMatch = vec.ChannelId
						}
					}
				}
			}
		}(chunk)
	}
	wg.Wait()

	if chMatch != 0 {
		if !itemExists(chMatch, playlistChUrl.Url) {
			channelItem := &models.TemplateChannelItem{
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
func matchTemplateTvgid(templateCh *models.TemplateChannel) (*[]models.PlaylistChannel, error) {
	channels, err := database.Db.GetPlChannelsByTvgID(*templateCh.TvgID)
	if err != nil {
		log.Debug().Msgf("matchChannels:, %v", err)
		return nil, err
	}

	for _, channel := range *channels {
		playlistChUrl, err := database.Db.GetChannelUrl(channel.ID)
		if err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
		} else if itemExists(channel.ID, playlistChUrl.Url) {
			continue
		}

		channelItem := &models.TemplateChannelItem{
			ChannelId:         templateCh.ID,
			PlaylistChannelId: channel.ID,
		}
		if _, err := database.Db.CreateTmplChannelItem(channelItem); err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
			continue
		}
	}

	return channels, nil
}

func matchTemplateChannelName(templateCh *models.TemplateChannel, addedChannels *[]models.PlaylistChannel) error {
	var wg sync.WaitGroup
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

	var chunks [][]models.PlaylistChannelVector
	chunkSize := int(math.RoundToEven(float64(len(playlistVectors)/10))) + 1
	for i := 0; i < len(playlistVectors); i += chunkSize {
		end := i + chunkSize
		if end > len(playlistVectors) {
			end = len(playlistVectors)
		}
		chunks = append(chunks, playlistVectors[i:end])
	}

	for _, chunk := range chunks {
		wg.Add(1)
		go func(chunk []models.PlaylistChannelVector) {
			defer wg.Done()
			for _, vec := range chunk {
				isMatch := false
				for _, channel := range *addedChannels {
					if vec.ChannelId == channel.ID {
						isMatch = true
						break
					}
				}
				if isMatch {
					continue
				}

				vector, err := database.Db.GetChannelVector(vec.VectorId)
				if err != nil {
					continue
				}
				if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
					if cosine >= settings.APP_SETTINGS.Playlist.Name_score {
						playlistChUrl, err := database.Db.GetChannelUrl(vec.ChannelId)
						if err != nil {
							log.Debug().Msgf("matchChannels:, %v", err)
						} else if itemExists(templateCh.ID, playlistChUrl.Url) {
							continue
						}
						channelItem := &models.TemplateChannelItem{
							ChannelId:         templateCh.ID,
							PlaylistChannelId: vec.ChannelId,
						}
						if _, err := database.Db.CreateTmplChannelItem(channelItem); err != nil {
							log.Debug().Msgf("matchChannels:, %v", err)
						}
					}
				}
			}
		}(chunk)
	}
	wg.Wait()

	return nil
}

func itemExists(tmplId int64, plUrl string) bool {
	if items, _ := database.Db.GetTmplChannelItemsByCh(tmplId); len(*items) > 0 {
		for _, item := range *items {
			if itemUrl, err := database.Db.GetChannelUrl(item.PlaylistChannelId); err != nil {
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

func TopChannelMatches(templateCh *models.TemplateChannel) ([]models.VectorMatch, error) {
	vectorMatches := vectorMatches{}
	var wg sync.WaitGroup
	var err error

	channelVector, err := database.Db.GetChannelVectorByName(templateCh.Name)
	if err != nil {
		log.Debug().Msgf("GetChannelVectorByName:, %v", err)
		vectorId := UpdateTemplateVector(templateCh)
		if channelVector, err = database.Db.GetChannelVector(vectorId); err != nil {
			log.Debug().Msgf("GetChannelVector:, %v", err)
			return nil, err
		}
	}

	playlistVectors, err := database.Db.GetPlaylistChannelVectors()
	if err != nil {
		log.Debug().Msgf("GetPlaylistChannelVectors:, %v", err)
		return nil, err
	}

	var chunks [][]models.PlaylistChannelVector
	chunkSize := int(math.RoundToEven(float64(len(playlistVectors)/10))) + 1
	for i := 0; i < len(playlistVectors); i += chunkSize {
		end := i + chunkSize
		if end > len(playlistVectors) {
			end = len(playlistVectors)
		}
		chunks = append(chunks, playlistVectors[i:end])
	}

	for _, chunk := range chunks {
		wg.Add(1)
		go func(chunk []models.PlaylistChannelVector) {
			defer wg.Done()
			for _, vec := range chunk {
				if vector, err := database.Db.GetChannelVector(vec.VectorId); err == nil {
					if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
						vectorMatches.Mu.Lock()
						if len(vectorMatches.matches) < 5 {
							vectorMatches.matches = append(vectorMatches.matches, models.VectorMatch{Id: vec.ChannelId, Score: cosine})
						} else {
							if cosine > vectorMatches.matches[len(vectorMatches.matches)-1].Score {
								vectorMatches.matches[len(vectorMatches.matches)-1] = models.VectorMatch{Id: vec.ChannelId, Score: cosine}
							}
						}

						sort.SliceStable(vectorMatches.matches, func(i, j int) bool {
							return vectorMatches.matches[i].Score > vectorMatches.matches[j].Score
						})
						vectorMatches.Mu.Unlock()

					}
				}
			}
		}(chunk)
	}
	wg.Wait()

	return vectorMatches.matches, nil
}
