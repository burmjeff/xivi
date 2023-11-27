// Copyright 2022 The NLP Odyssey Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package utils

import (
	"context"
	"errors"
	"math"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/nlpodyssey/cybertron/pkg/models/bert"
	"github.com/nlpodyssey/cybertron/pkg/tasks"
	"github.com/nlpodyssey/cybertron/pkg/tasks/textencoding"
	"github.com/rs/zerolog/log"
)

func PlaylistVectorQueue(in <-chan *models.PlaylistChannel, db *database.Queries) {
	for playlistCh := range in {
		_ = UpdatePlaylistVector(db, playlistCh)

		//try to match template channel only if auto-match=true
		go MatchChannel(db, playlistCh)
	}
}

// returns vector id
func UpdatePlaylistVector(db *database.Queries, playlistCh *models.PlaylistChannel) int64 {
	if vectorId, err := addChannelVector(db, playlistCh.Title); err != nil {
		log.Warn().Msgf("VECTORIZE_STRING: %v", err)
	} else {
		if channelVector, err := db.GetPlaylistChannelVector(playlistCh.ID); err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
			channelVector = &models.PlaylistChannelVector{
				ChannelId: playlistCh.ID,
				VectorId:  vectorId,
			}
			if _, err := db.CreatePlaylistChannelVector(channelVector); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return channelVector.VectorId
			}
		} else {
			channelVector.VectorId = vectorId
			if err := db.UpdatePlaylistChannelVector(channelVector); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return channelVector.VectorId
			}
		}
	}
	return 0
}

func UpdateTemplateVector(db *database.Queries, templateCh *models.TemplateChannel) {
	if vectorId, err := addChannelVector(db, templateCh.Name); err != nil {
		log.Warn().Msgf("VECTORIZE_STRING: %v", err)
	} else {
		if channelVector, err := db.GetTemplateChannelVector(templateCh.ID); err != nil {
			log.Debug().Msgf("matchChannels:, %v", err)
			channelVector = &models.TemplateChannelVector{
				ChannelId: templateCh.ID,
				VectorId:  vectorId,
			}
			if _, err := db.CreateTemplateChannelVector(channelVector); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return
			}
		} else {
			channelVector.VectorId = vectorId
			if err := db.UpdateTemplateChannelVector(channelVector); err != nil {
				log.Debug().Msgf("matchChannels:, %v", err)
				return
			}
		}
	}
}

func addChannelVector(db *database.Queries, name string) (int64, error) {
	var channelVector models.ChannelVector
	var err error

	channelVector, err = db.GetChannelVectorByName(name)
	if err != nil {
		vector, err := vectorizeString(name)
		if err != nil {
			return 0, err
		}
		channelVector = models.ChannelVector{Name: name, Vector: vector}
		channelVector.ID, err = db.CreateChannelVector(channelVector)
		if err != nil {
			return 0, err
		}
		log.Info().Msgf("VECTOR_TOOLS: ADDED VECTOR FOR %s", name)
	}
	return channelVector.ID, nil
}

func vectorizeString(text string) ([]float64, error) {
	//TODO: NEW MODELS
	modelName := settings.APP_SETTINGS.Application.Model

	m, err := tasks.Load[textencoding.Interface](&tasks.Config{
		ModelsDir: settings.MODEL_PATH,
		ModelName: modelName,
	})
	if err != nil {
		return nil, err
	}

	fn, err := m.Encode(context.Background(), text, int(bert.MeanPooling))
	if err != nil {
		return nil, err
	}

	return fn.Vector.Data().F64(), nil

	//fmt.Println(Cosine(r1.Vector.Data().F64(), r2.Vector.Data().F64()))
}

func CosineMatch(a []float64, b []float64) (cosine float64, err error) {
	count := 0
	length_a := len(a)
	length_b := len(b)
	if length_a > length_b {
		count = length_a
	} else {
		count = length_b
	}
	sumA := 0.0
	s1 := 0.0
	s2 := 0.0
	for k := 0; k < count; k++ {
		if k >= length_a {
			s2 += math.Pow(b[k], 2)
			continue
		}
		if k >= length_b {
			s1 += math.Pow(a[k], 2)
			continue
		}
		sumA += a[k] * b[k]
		s1 += math.Pow(a[k], 2)
		s2 += math.Pow(b[k], 2)
	}
	if s1 == 0 || s2 == 0 {
		return 0.0, errors.New("vectors should not be null (all zeros)")
	}
	return sumA / (math.Sqrt(s1) * math.Sqrt(s2)), nil
}
