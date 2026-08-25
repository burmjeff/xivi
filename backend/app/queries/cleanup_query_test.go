package queries

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestPruneRetentionDataBoundsHistoryAndProtectsActiveRows(t *testing.T) {
	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on")
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	for _, statement := range []string{
		`CREATE TABLE operation_job (id INTEGER PRIMARY KEY, status TEXT, created_at DATETIME, updated_at DATETIME, finished_at DATETIME)`,
		`CREATE TABLE stream_session_history (incident_id TEXT PRIMARY KEY, started_at DATETIME, ended_at DATETIME)`,
		`CREATE TABLE stream_connection_history (id TEXT PRIMARY KEY, incident_id TEXT, started_at DATETIME, ended_at DATETIME, FOREIGN KEY (incident_id) REFERENCES stream_session_history(incident_id) ON DELETE CASCADE)`,
		`CREATE TABLE stream_event (id INTEGER PRIMARY KEY, incident_id TEXT, created_at DATETIME, FOREIGN KEY (incident_id) REFERENCES stream_session_history(incident_id) ON DELETE CASCADE)`,
		`CREATE TABLE epgprogramme (id INTEGER PRIMARY KEY, stop DATETIME)`,
		`CREATE TABLE epgchannel (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE epgchannelitem (id INTEGER PRIMARY KEY, epg_channel_id INTEGER, epg_programme_id INTEGER)`,
		`CREATE TABLE channelvectors (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE templatechannelvectors (id INTEGER PRIMARY KEY, vector_id INTEGER)`,
		`CREATE TABLE playlistchannelvectors (id INTEGER PRIMARY KEY, vector_id INTEGER)`,
		`CREATE TABLE templatechannel (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE playlistchannel (id INTEGER PRIMARY KEY, title TEXT)`,
	} {
		db.MustExec(statement)
	}

	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	stamp := func(offset time.Duration) string { return databaseTimestamp(now.Add(offset)) }
	db.MustExec(`INSERT INTO operation_job VALUES
		(1, 'succeeded', ?, ?, ?),
		(2, 'failed', ?, ?, ?),
		(3, 'succeeded', ?, ?, ?),
		(4, 'cancelled', ?, ?, ?),
		(5, 'running', ?, ?, NULL)`,
		stamp(-60*24*time.Hour), stamp(-60*24*time.Hour), stamp(-60*24*time.Hour),
		stamp(-4*time.Hour), stamp(-4*time.Hour), stamp(-4*time.Hour),
		stamp(-3*time.Hour), stamp(-3*time.Hour), stamp(-3*time.Hour),
		stamp(-2*time.Hour), stamp(-2*time.Hour), stamp(-2*time.Hour),
		stamp(-365*24*time.Hour), stamp(-365*24*time.Hour))

	db.MustExec(`INSERT INTO stream_session_history VALUES
		('expired', ?, ?), ('recent-1', ?, ?), ('recent-2', ?, ?),
		('recent-3', ?, ?), ('active', ?, NULL)`,
		stamp(-60*24*time.Hour), stamp(-59*24*time.Hour),
		stamp(-4*time.Hour), stamp(-4*time.Hour),
		stamp(-3*time.Hour), stamp(-3*time.Hour),
		stamp(-2*time.Hour), stamp(-2*time.Hour),
		stamp(-365*24*time.Hour))
	db.MustExec(`INSERT INTO stream_event VALUES (1, 'expired', ?), (2, 'active', ?)`, stamp(-59*24*time.Hour), stamp(-time.Hour))
	db.MustExec(`INSERT INTO stream_connection_history VALUES ('expired-connection', 'expired', ?, ?), ('active-connection', 'active', ?, NULL)`, stamp(-59*24*time.Hour), stamp(-59*24*time.Hour), stamp(-time.Hour))
	for id := 10; id < 15; id++ {
		db.MustExec(`INSERT INTO stream_event VALUES (?, 'recent-3', ?)`, id, stamp(time.Duration(id)*time.Minute))
	}
	for id := 10; id < 14; id++ {
		db.MustExec(`INSERT INTO stream_connection_history VALUES (?, 'recent-3', ?, ?)`, fmt.Sprintf("connection-%d", id), stamp(time.Duration(id)*time.Minute), stamp(time.Duration(id)*time.Minute))
	}

	db.MustExec(`INSERT INTO epgprogramme VALUES (1, ?), (2, ?), (3, ?), (4, ?)`,
		stamp(-48*time.Hour), stamp(-25*time.Hour), stamp(time.Hour), "2026-08-24 08:30:00-04:00")
	db.MustExec(`INSERT INTO channelvectors VALUES (1, 'referenced'), (2, 'orphan-1'), (3, 'orphan-2'), (4, 'pending-channel')`)
	db.MustExec(`INSERT INTO templatechannelvectors VALUES (1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES (1, 'pending-channel')`)

	report, err := NewCleanupQueries(db).PruneRetentionData(context.Background(), RetentionPolicy{
		OperationJobsBefore:          now.Add(-30 * 24 * time.Hour),
		StreamHistoryBefore:          now.Add(-14 * 24 * time.Hour),
		EPGProgrammesBefore:          now.Add(-24 * time.Hour),
		MaximumOperationJobs:         2,
		MaximumStreamSessions:        2,
		MaximumEventsPerSession:      3,
		MaximumConnectionsPerSession: 2,
		BatchSize:                    1,
		PruneOrphanVectors:           true,
	})
	if err != nil {
		t.Fatalf("prune retention data: %v", err)
	}
	if report.OperationJobs != 2 || report.StreamSessions != 2 || report.StreamEvents != 2 || report.StreamConnections != 2 || report.EPGProgrammes != 2 || report.OrphanVectors != 2 {
		t.Fatalf("unexpected cleanup report: %+v", report)
	}

	assertCount(t, db, `SELECT COUNT(*) FROM operation_job WHERE status = 'running'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM operation_job WHERE status != 'running'`, 2)
	assertCount(t, db, `SELECT COUNT(*) FROM stream_session_history WHERE ended_at IS NULL`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM stream_session_history WHERE ended_at IS NOT NULL`, 2)
	assertCount(t, db, `SELECT COUNT(*) FROM stream_event WHERE incident_id = 'recent-3'`, 3)
	assertCount(t, db, `SELECT COUNT(*) FROM stream_connection_history WHERE incident_id = 'recent-3'`, 2)
	assertCount(t, db, `SELECT COUNT(*) FROM stream_connection_history WHERE ended_at IS NULL`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM stream_event WHERE incident_id = 'expired'`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM stream_connection_history WHERE incident_id = 'expired'`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM epgprogramme`, 2)
	assertCount(t, db, `SELECT COUNT(*) FROM channelvectors`, 2)
}

func assertCount(t *testing.T, db *sqlx.DB, query string, expected int) {
	t.Helper()
	var actual int
	if err := db.Get(&actual, query); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if actual != expected {
		t.Fatalf("count query %q: got %d, want %d", query, actual, expected)
	}
}
