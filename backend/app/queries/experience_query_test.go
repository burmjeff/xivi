//go:build cgo

package queries

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newExperienceTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_loc=UTC")
	t.Cleanup(func() { _ = db.Close() })
	schema := []string{
		`CREATE TABLE template (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE templategroup (id INTEGER PRIMARY KEY, name TEXT NOT NULL, dynamic BOOLEAN, dynamicgroup INTEGER)`,
		`CREATE TABLE template_group_item (template_id INTEGER, group_id INTEGER, orderr INTEGER)`,
		`CREATE TABLE templatechannel (id INTEGER PRIMARY KEY, name TEXT, tvgid TEXT, logoid INTEGER, uuid TEXT)`,
		`CREATE TABLE playlistgroup (id INTEGER PRIMARY KEY, name TEXT, playlist_id INTEGER, enabled BOOLEAN)`,
		`CREATE TABLE playlistchannel (id INTEGER PRIMARY KEY, tvg_id TEXT, tvg_name TEXT, tvg_logo TEXT, title TEXT, group_id INTEGER, enabled BOOLEAN)`,
		`CREATE TABLE template_group_channel (group_id INTEGER, channel_id INTEGER, orderr INTEGER, PRIMARY KEY (group_id, channel_id), FOREIGN KEY (channel_id) REFERENCES templatechannel(id) ON DELETE CASCADE)`,
		`CREATE TABLE templatechannelitem (id INTEGER PRIMARY KEY, channel_id INTEGER, playlist_channel_id INTEGER, orderr INTEGER, match_method TEXT, match_score REAL, runner_up_score REAL, matcher_version INTEGER, manual_locked BOOLEAN, FOREIGN KEY (channel_id) REFERENCES templatechannel(id) ON DELETE CASCADE)`,
		`CREATE TABLE logo (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE epgprogramme (id INTEGER PRIMARY KEY, start DATETIME, stop DATETIME, channel TEXT, "title.value" TEXT, subtitle TEXT, desc TEXT, categories TEXT)`,
		`CREATE TABLE operation_job (id INTEGER PRIMARY KEY AUTOINCREMENT, kind TEXT, resource TEXT, resource_id INTEGER, status TEXT DEFAULT 'queued', progress INTEGER DEFAULT 0, message TEXT DEFAULT '', error_code TEXT DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, finished_at DATETIME)`,
	}
	for _, statement := range schema {
		db.MustExec(statement)
	}
	return db
}

func TestGetGuideChannelsUsesWindowBoundariesAndSavedOrder(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main')`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL)`)
	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 4)`)
	db.MustExec(`INSERT INTO logo VALUES (0, 'xivi_channel')`)
	db.MustExec(`INSERT INTO templatechannel VALUES (100, 'Alpha', 'alpha.tv', 0, 'alpha-uuid'), (101, 'No Source', 'none.tv', 0, 'none-uuid')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES (10, 100, 8), (10, 101, 9)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES (1, 100, 1000, 0, 'manual', 1, NULL, 2, 1)`)

	from := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	to := from.Add(2 * time.Hour)
	insert := `INSERT INTO epgprogramme (id, start, stop, channel, "title.value", subtitle, desc, categories) VALUES (?, ?, ?, 'alpha.tv', ?, '', '', '')`
	db.MustExec(insert, 1, from.Add(-time.Hour), from, "Ends at boundary")
	db.MustExec(insert, 2, from, from.Add(time.Hour), "First hour")
	db.MustExec(insert, 3, from.Add(time.Hour), to, "Second hour")
	db.MustExec(insert, 4, to, to.Add(time.Hour), "Starts at boundary")

	query := NewExperienceQueries(db)
	channels, total, err := query.GetGuideChannels(context.Background(), 1, nil, "", from, to, 100, 0)
	if err != nil {
		t.Fatalf("GetGuideChannels returned an error: %v", err)
	}
	if total != 1 || len(channels) != 1 {
		t.Fatalf("expected one playable channel, total=%d len=%d", total, len(channels))
	}
	if channels[0].Number != 1 || channels[0].Name != "Alpha" {
		t.Fatalf("saved order was not preserved: %#v", channels[0])
	}
	if len(channels[0].Programmes) != 2 {
		t.Fatalf("expected two overlapping programmes, got %d", len(channels[0].Programmes))
	}
	if channels[0].Programmes[0].Title != "First hour" || channels[0].Programmes[1].Title != "Second hour" {
		t.Fatalf("unexpected window contents: %#v", channels[0].Programmes)
	}
	if channels[0].Logo != "" {
		t.Fatalf("default logo should map to the Signal Tile placeholder, got %q", channels[0].Logo)
	}
}

func TestMoveWorkspaceChannelRequiresOneAnchor(t *testing.T) {
	db := newExperienceTestDB(t)
	query := NewExperienceQueries(db)
	if err := query.MoveWorkspaceChannel(context.Background(), 1, 1, nil, nil); err == nil {
		t.Fatal("expected a validation error without before_id or after_id")
	}
	anchor := int64(2)
	if err := query.MoveWorkspaceChannel(context.Background(), 1, 1, &anchor, &anchor); err == nil {
		t.Fatal("expected a validation error with both anchors")
	}
}

func TestMoveWorkspaceChannelReindexesBothDirections(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template_group_channel VALUES (10, 1, 10), (10, 2, 20), (10, 3, 30)`)
	query := NewExperienceQueries(db)
	third := int64(3)
	if err := query.MoveWorkspaceChannel(context.Background(), 10, 1, nil, &third); err != nil {
		t.Fatalf("moving after a later channel failed: %v", err)
	}
	var ids []int64
	db.Select(&ids, `SELECT channel_id FROM template_group_channel WHERE group_id = 10 ORDER BY orderr`)
	if got, want := ids, []int64{2, 3, 1}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("unexpected order after downward move: %v", got)
	}
	second := int64(2)
	if err := query.MoveWorkspaceChannel(context.Background(), 10, 1, &second, nil); err != nil {
		t.Fatalf("moving before an earlier channel failed: %v", err)
	}
	ids = nil
	db.Select(&ids, `SELECT channel_id FROM template_group_channel WHERE group_id = 10 ORDER BY orderr`)
	if got, want := ids, []int64{1, 2, 3}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("unexpected order after upward move: %v", got)
	}
}

func TestBatchWorkspaceChannelLifecycle(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL), (20, 'Everywhere', 0, NULL)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (30, 'Provider', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES (1000, 'alpha.tv', 'Alpha TV', NULL, 'Alpha', 30, 1), (1001, 'beta.tv', 'Beta TV', NULL, 'Beta', 30, 1)`)
	query := NewExperienceQueries(db)
	created, err := query.BatchAddWorkspaceChannels(context.Background(), 10, []int64{1000, 1001}, []string{"alpha-uuid", "beta-uuid"})
	if err != nil {
		t.Fatalf("batch add failed: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected two created channels, got %v", created)
	}
	var methods []string
	db.Select(&methods, `SELECT match_method FROM templatechannelitem ORDER BY channel_id`)
	if len(methods) != 2 || methods[0] != "manual" || methods[1] != "manual" {
		t.Fatalf("source variants were not manually locked: %v", methods)
	}
	if err := query.BatchMoveWorkspaceChannels(context.Background(), 10, 20, created, nil, nil); err != nil {
		t.Fatalf("batch move failed: %v", err)
	}
	var target []int64
	db.Select(&target, `SELECT channel_id FROM template_group_channel WHERE group_id = 20 ORDER BY orderr`)
	if len(target) != 2 || target[0] != created[0] || target[1] != created[1] {
		t.Fatalf("moved block order was not preserved: %v", target)
	}
	if err := query.BatchRemoveWorkspaceChannels(context.Background(), 20, []int64{created[0]}); err != nil {
		t.Fatalf("batch remove failed: %v", err)
	}
	var remaining []int64
	db.Select(&remaining, `SELECT channel_id FROM template_group_channel WHERE group_id = 20 ORDER BY orderr`)
	if len(remaining) != 1 || remaining[0] != created[1] {
		t.Fatalf("unexpected membership after removal: %v", remaining)
	}
	var position int
	db.Get(&position, `SELECT orderr FROM template_group_channel WHERE group_id = 20 AND channel_id = ?`, created[1])
	if position != 1 {
		t.Fatalf("remaining channel was not reindexed: %d", position)
	}
}

func TestOperationJobLifecycle(t *testing.T) {
	db := newExperienceTestDB(t)
	query := NewExperienceQueries(db)
	resourceID := int64(42)
	job, err := query.CreateJob(context.Background(), "publish", "lineup", &resourceID, "Queued")
	if err != nil {
		t.Fatalf("CreateJob returned an error: %v", err)
	}
	if job.Status != "queued" || job.Progress != 0 || job.ResourceID == nil || *job.ResourceID != resourceID {
		t.Fatalf("unexpected queued job: %#v", job)
	}
	if err := query.UpdateJob(context.Background(), job.ID, "succeeded", 100, "Published", ""); err != nil {
		t.Fatalf("UpdateJob returned an error: %v", err)
	}
	finished, err := query.GetJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetJob returned an error: %v", err)
	}
	if finished.Status != "succeeded" || finished.Progress != 100 || finished.FinishedAt == nil {
		t.Fatalf("unexpected completed job: %#v", finished)
	}
}
