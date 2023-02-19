// Copyright 2022 The NLP Odyssey Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package utils

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/nlpodyssey/cybertron/pkg/models/bert"
	"github.com/nlpodyssey/cybertron/pkg/tasks"
	"github.com/nlpodyssey/cybertron/pkg/tasks/textencoding"
	log "github.com/sirupsen/logrus"
)

const limit = 10

func VectorizeString(text string) {
	modelsDir := os.Getenv("MODEL_PATH")
	modelName := os.Getenv("MODEL_NAME")

	m, err := tasks.Load[textencoding.Interface](&tasks.Config{
		ModelsDir: modelsDir,
		ModelName: modelName,
	})
	if err != nil {
		log.Error("VECTORIZE_STRING: ", err)
		return
	}
	defer tasks.Finalize(m)

	fn := func(text string) *textencoding.Response {
		result, err := m.Encode(context.Background(), text, int(bert.MeanPooling))
		if err != nil {
			log.Error("VECTORIZE_STRING: ", err)
			return nil
		}
		return &result
	}

	r1 := fn(text)
	fmt.Println(r1.Vector.Data().F64()[:limit])

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
