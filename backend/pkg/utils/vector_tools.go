// Copyright 2022 The NLP Odyssey Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package utils

import (
	"context"
	"errors"
	"math"
	"os"
	"xivi/backend/app"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/nlpodyssey/cybertron/pkg/models/bert"
	"github.com/nlpodyssey/cybertron/pkg/tasks"
	"github.com/nlpodyssey/cybertron/pkg/tasks/textencoding"
	log "github.com/sirupsen/logrus"
)

func VectorQueue(in <-chan string, db *database.Queries) {
	for name := range in {
		AddChannelVector(db, name)
	}
}

func AddChannelVector(db *database.Queries, name string) {
	_, err := db.GetChannelVector(name)
	if err != nil {
		vector, err := VectorizeString(name)
		if err != nil {
			log.Warn("VECTORIZE_STRING: ", err)
			return
		}
		channelVector := models.ChannelVector{Name: name, Vector: vector}
		err = db.CreateChannelVector(channelVector)
		if err != nil {
			log.Warn("VECTORIZE_STRING: ", err)
			return
		}

		log.Info("VECTOR_TOOLS: ADDED VECTOR FOR ", name)
	}
}

func VectorizeString(text string) ([]float64, error) {
	//TODO: NEW MODELS
	modelName := os.Getenv("MODEL_NAME")

	m, err := tasks.Load[textencoding.Interface](&tasks.Config{
		ModelsDir: app.MODEL_PATH,
		ModelName: modelName,
	})
	if err != nil {
		return nil, err
	}
	defer tasks.Finalize(m)

	fn, err := m.Encode(context.Background(), text, int(bert.MeanPooling))
	if err != nil {
		return nil, err
	}

	return fn.Vector.Data().F64(), nil

	//fmt.Println(Cosine(r1.Vector.Data().F64(), r2.Vector.Data().F64()))
}

func Cosine(a []float64, b []float64) (cosine float64, err error) {
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
