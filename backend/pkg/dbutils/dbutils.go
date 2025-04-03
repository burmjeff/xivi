package dbutils

import (
	"database/sql"
	"math"
	"strings"
	"time"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

func NewNullString(s string) sql.NullString {
	if len(s) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{
		String: s,
		Valid:  true,
	}
}

func CustomMapper(column string) string {
	// Handle array notation
	column = strings.Replace(column, "[]", "_array_", 1)

	// Handle dot notation in column names
	if strings.Contains(column, ".") {
		parts := strings.Split(column, ".")
		if len(parts) == 2 {
			// For nested structs like title.value, map to the struct field
			// This is critical for mapping database columns to nested struct fields
			// Log the mapping for debugging
			log.Debug().Msgf("CustomMapper: mapping column %s to struct field %s", column, parts[0])

			// Return the parent field name to map to the nested struct
			return parts[0]
		}
	}

	return column
}

func ConvertTime(timeStr string) (time.Time, error) {
	localTime, err := time.LoadLocation(settings.APP_SETTINGS.Application.TZ)
	if err != nil {
		localTime, err = time.LoadLocation("Etc/UTC")
		if err != nil {
			log.Error().Msgf("No TZ provided: %v", err)
			return time.Now(), err
		}
	}
	t, err := time.Parse("20060102150405 -0700", timeStr)
	if err != nil {
		return t, err
	}
	t = t.In(localTime)
	return t, nil
}

// Convert a float64 slice to a byte slice
func VectorToBytes(vector []float64) []byte {
	vectorBytes := make([]byte, 8*len(vector))
	for i, val := range vector {
		offset := i * 8
		bits := math.Float64bits(val)
		for j := 0; j < 8; j++ {
			vectorBytes[offset+j] = byte(bits >> uint(56-8*j))
		}
	}
	return vectorBytes
}

// Convert a byte slice to a float64 slice
func BytesToVector(vectorBytes []byte) []float64 {
	vector := make([]float64, len(vectorBytes)/8)
	for i := range vector {
		offset := i * 8
		bits := uint64(0)
		for j := 0; j < 8; j++ {
			bits |= uint64(vectorBytes[offset+j]) << uint(56-8*j)
		}
		vector[i] = math.Float64frombits(bits)
	}
	return vector
}
