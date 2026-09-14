package queries

import (
	"time"
	"xivi/backend/platform/settings"
)

// EPG timestamps are persisted using the configured application timezone.
// Keep query bounds in that same location so SQLite's DATETIME comparisons do
// not compare UTC and local wall-clock representations as plain text.
func epgQueryTime(value time.Time) time.Time {
	location, err := time.LoadLocation(settings.Current().Application.TZ)
	if err != nil {
		location = time.UTC
	}
	return value.In(location)
}
