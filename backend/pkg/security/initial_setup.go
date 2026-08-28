package security

import "sync/atomic"

var initialSetupRequired atomic.Bool

// SetInitialSetupRequired publishes the database-backed first-run state to
// high-frequency media authorization paths without adding a SQLite query to
// every HLS segment or MPEG-TS request.
func SetInitialSetupRequired(required bool) {
	initialSetupRequired.Store(required)
}

func InitialSetupRequired() bool {
	return initialSetupRequired.Load()
}
