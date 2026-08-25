package queries

import (
	"context"
	"strings"
	"testing"
	"time"

	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestInsertStreamEventBoundsPayloadAndAmortizesSessionCeiling(t *testing.T) {
	db := sqlx.MustOpen("sqlite3", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	db.MustExec(`CREATE TABLE stream_event (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		incident_id TEXT, connection_id TEXT, stream_id TEXT, severity TEXT,
		code TEXT, message TEXT, source_position INTEGER, details TEXT, created_at DATETIME
	)`)
	streamEventRetentionCounter.Store(0)
	queries := NewStreamQueries(db)
	now := time.Now().UTC()
	for index := 0; index < 1100; index++ {
		event := models.StreamEvent{
			IncidentID: "incident", StreamID: "channel", Severity: strings.Repeat("s", 40),
			Code: strings.Repeat("c", 100), Message: strings.Repeat("m", 2000),
			Details: strings.Repeat("d", 10000), CreatedAt: now.Add(time.Duration(index) * time.Millisecond),
		}
		if err := queries.InsertStreamEvent(context.Background(), event); err != nil {
			t.Fatalf("insert event %d: %v", index, err)
		}
	}

	assertCount(t, db, `SELECT COUNT(*) FROM stream_event`, 1000)
	var lengths struct {
		Severity int `db:"severity"`
		Code     int `db:"code"`
		Message  int `db:"message"`
		Details  int `db:"details"`
	}
	if err := db.Get(&lengths, `SELECT length(severity) AS severity, length(code) AS code,
		length(message) AS message, length(details) AS details FROM stream_event LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if lengths.Severity != 16 || lengths.Code != 64 || lengths.Message != 1024 || lengths.Details != 8192 {
		t.Fatalf("unexpected persisted payload lengths: %+v", lengths)
	}
}
