package dbutils

import (
	"database/sql"
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
