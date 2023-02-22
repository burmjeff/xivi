package dbutils

import (
	"database/sql"
	"math"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
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
	return strings.Replace(column, "[]", "_array_", 1)
}

func ConvertTime(timeStr string) (time.Time, error) {
	localTime, err := time.LoadLocation(os.Getenv("TZ"))
	if err != nil {
		log.Error("No TZ provided", err)
		return time.Now(), err
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
