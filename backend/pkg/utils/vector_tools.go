package utils

/*
#cgo CFLAGS: -I/lib_build/
#cgo LDFLAGS: /lib_build/libcandle_embeddings.a

#include "candle-embeddings.h"
*/
import "C"

import (
	"encoding/json"
	"math"
	"unsafe"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

func PlaylistVectorQueue(in <-chan models.PlaylistChannel) {
	bufChan := make(chan struct{}, 5)
	for playlistCh := range in {
		bufChan <- struct{}{}
		go func(playlistCh models.PlaylistChannel) {
			defer func() {
				<-bufChan
			}()

			UpdatePlaylistVector(playlistCh)
			MatchPlaylistChannel(playlistCh)
		}(playlistCh)
	}
}

func TemplateVectorQueue(in <-chan models.TemplateChannel) {
	bufChan := make(chan struct{}, 5)
	for templateCh := range in {
		bufChan <- struct{}{}
		go func(templateCh models.TemplateChannel) {
			defer func() {
				<-bufChan
			}()

			UpdateTemplateVector(&templateCh)
			MatchTemplateChannel(&templateCh)
		}(templateCh)
	}
}

// returns vector id
func UpdatePlaylistVector(playlistCh models.PlaylistChannel) int64 {
	if vectorId, err := getChannelVector(playlistCh.Title); err != nil {
		log.Warn().Msgf("VECTORIZE_PLAYLIST_STRING: %v", err)
	} else {
		if channelVector, err := database.Db.GetPlaylistChannelVector(playlistCh.ID); err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
			channelVector = &models.PlaylistChannelVector{
				ChannelId: playlistCh.ID,
				VectorId:  vectorId,
			}
			if _, err := database.Db.CreatePlaylistChannelVector(channelVector); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return channelVector.VectorId
			}
		} else {
			channelVector.VectorId = vectorId
			if err := database.Db.UpdatePlaylistChannelVector(channelVector); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return channelVector.VectorId
			}
		}
	}
	return 0
}

// returns vector id
func UpdateTemplateVector(templateCh *models.TemplateChannel) int64 {
	if vectorId, err := getChannelVector(templateCh.Name); err != nil {
		log.Warn().Msgf("VECTORIZE_TEMPLATE_STRING: %v", err)
	} else {
		if channelVector, err := database.Db.GetTemplateChannelVector(templateCh.ID); err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
			channelVector = &models.TemplateChannelVector{
				ChannelId: templateCh.ID,
				VectorId:  vectorId,
			}
			if _, err := database.Db.CreateTemplateChannelVector(channelVector); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return channelVector.VectorId
			}
		} else {
			channelVector.VectorId = vectorId
			if err := database.Db.UpdateTemplateChannelVector(channelVector); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return channelVector.VectorId
			}
		}
	}
	return 0
}

func getChannelVector(name string) (int64, error) {
	channelVector, err := database.Db.GetChannelVectorByName(name)
	if err != nil {
		vector, err := vectorizeString(name)
		if err != nil {
			return 0, err
		}
		channelVector = &models.ChannelVector{Name: name, Vector: vector}
		channelVector.ID, err = database.Db.CreateChannelVector(channelVector)
		if err != nil {
			return 0, err
		}
		log.Info().Msgf("VECTOR_TOOLS: ADDED VECTOR FOR %s", name)
	}
	return channelVector.ID, nil
}

func vectorizeString(text string) ([]float64, error) {
	cInput := C.CString(text)
	defer C.free(unsafe.Pointer(cInput))

	output := C.create_embedding(cInput)
	jsonStr := C.GoString(output)

	var tensorVec []float64
	err := json.Unmarshal([]byte(jsonStr), &tensorVec)
	if err != nil {
		return nil, err
	}

	return tensorVec, nil
}

func CosineMatch(a, b []float64) (cosine float64, err error) {
	dotProduct := 0.0
	aSquared := 0.0
	bSquared := 0.0

	for i := 0; i < len(a) && i < len(b); i++ {
		dotProduct += a[i] * b[i]
		aSquared += a[i] * a[i]
		bSquared += b[i] * b[i]
	}

	return math.Round(dotProduct/(math.Sqrt(aSquared*bSquared))*10000) / 10000, nil
}
