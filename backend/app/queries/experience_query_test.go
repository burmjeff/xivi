//go:build cgo

package queries

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newExperienceTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_loc=UTC")
	t.Cleanup(func() { _ = db.Close() })
	schema := []string{
		`CREATE TABLE playlist (id INTEGER PRIMARY KEY, name TEXT NOT NULL, url TEXT, created_at DATETIME, updated_at DATETIME)`,
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
		`CREATE TABLE lineup_group_source_link (group_id INTEGER PRIMARY KEY, playlist_id INTEGER NOT NULL, source_group_id INTEGER, source_group_name TEXT NOT NULL, follow_group_name BOOLEAN NOT NULL DEFAULT 1, follow_channel_names BOOLEAN NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'pending', last_synced_at DATETIME, last_error TEXT NOT NULL DEFAULT '', added_count INTEGER NOT NULL DEFAULT 0, updated_count INTEGER NOT NULL DEFAULT 0, removed_count INTEGER NOT NULL DEFAULT 0, FOREIGN KEY (group_id) REFERENCES templategroup(id) ON DELETE CASCADE, FOREIGN KEY (playlist_id) REFERENCES playlist(id) ON DELETE CASCADE, FOREIGN KEY (source_group_id) REFERENCES playlistgroup(id) ON DELETE SET NULL)`,
		`CREATE TABLE lineup_group_source_member (group_id INTEGER NOT NULL, template_channel_id INTEGER NOT NULL, source_channel_id INTEGER, source_identity TEXT NOT NULL, last_source_name TEXT NOT NULL DEFAULT '', PRIMARY KEY (group_id, template_channel_id), UNIQUE (group_id, source_identity), FOREIGN KEY (group_id) REFERENCES lineup_group_source_link(group_id) ON DELETE CASCADE, FOREIGN KEY (template_channel_id) REFERENCES templatechannel(id) ON DELETE CASCADE, FOREIGN KEY (source_channel_id) REFERENCES playlistchannel(id) ON DELETE SET NULL)`,
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
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'One', NULL, 0, 'one'), (2, 'Two', NULL, 0, 'two'), (3, 'Three', NULL, 0, 'three')`)
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

func TestCreateStudioGroupRollsBackEveryStepWhenSourceLinkFails(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main')`)
	query := NewExperienceQueries(db)
	request := models.StudioGroupCreateRequest{
		Name: "News",
		SourceLink: &models.SourceGroupLinkRequest{
			SourceGroupID:      999,
			FollowGroupName:    true,
			FollowChannelNames: true,
		},
	}
	if _, err := query.CreateStudioGroup(context.Background(), 1, request); !errors.Is(err, ErrSourceGroupNotFound) {
		t.Fatalf("expected a missing source error, got %v", err)
	}
	for _, table := range []string{"templategroup", "template_group_item", "lineup_group_source_link"} {
		var count int
		db.Get(&count, `SELECT COUNT(*) FROM `+table)
		if count != 0 {
			t.Fatalf("%s retained %d rows after the transaction failed", table, count)
		}
	}
}

func TestCreateStudioGroupAtomicallyAttachesLineupAndSource(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main')`)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', 'https://example.test/list.m3u', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (20, 'Provider News', 1, 1)`)
	query := NewExperienceQueries(db)
	groupID, err := query.CreateStudioGroup(context.Background(), 1, models.StudioGroupCreateRequest{
		Name: "News",
		SourceLink: &models.SourceGroupLinkRequest{
			SourceGroupID:      20,
			FollowGroupName:    true,
			FollowChannelNames: true,
		},
	})
	if err != nil {
		t.Fatalf("CreateStudioGroup failed: %v", err)
	}
	var state struct {
		Name          string `db:"name"`
		Order         int64  `db:"orderr"`
		SourceGroupID int64  `db:"source_group_id"`
		Dynamic       bool   `db:"dynamic"`
	}
	if err := db.Get(&state, `SELECT tg.name, tg.dynamic, tgi.orderr, l.source_group_id
		FROM templategroup tg
		JOIN template_group_item tgi ON tgi.group_id = tg.id
		JOIN lineup_group_source_link l ON l.group_id = tg.id
		WHERE tg.id = ? AND tgi.template_id = 1`, groupID); err != nil {
		t.Fatalf("atomic group state was incomplete: %v", err)
	}
	if state.Name != "News" || state.Order != 1 || state.SourceGroupID != 20 || state.Dynamic {
		t.Fatalf("unexpected atomic group state: %#v", state)
	}
}

func TestDeleteStudioGroupRemovesOnlyOrphanedCanonicalChannels(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'One', 0, NULL), (20, 'Two', 0, NULL)`)
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'Shared', NULL, 0, 'shared'), (2, 'Owned', NULL, 0, 'owned')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES (10, 1, 1), (10, 2, 2), (20, 1, 1)`)
	query := NewExperienceQueries(db)
	if err := query.DeleteStudioGroup(context.Background(), 10); err != nil {
		t.Fatalf("DeleteStudioGroup failed: %v", err)
	}
	var ids []int64
	db.Select(&ids, `SELECT id FROM templatechannel ORDER BY id`)
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("shared and orphaned channels were cleaned incorrectly: %v", ids)
	}
}

func TestCopySourceGroupToLineupCreatesManualSnapshotWithUniqueName(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main')`)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', 'https://example.test/list.m3u', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (20, 'News', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, 'alpha.tv', 'Alpha Guide', NULL, 'Alpha', 20, 1),
		(101, 'beta.tv', 'Beta Guide', NULL, 'Beta', 20, 1)`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL)`)
	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 4)`)

	result, err := NewExperienceQueries(db).CopySourceGroupToLineup(context.Background(), 1, 20)
	if err != nil {
		t.Fatalf("CopySourceGroupToLineup failed: %v", err)
	}
	if result.GroupName != "News copy" || result.AddedCount != 2 || result.SkippedCount != 0 || len(result.ChannelIDs) != 2 {
		t.Fatalf("unexpected copy result: %#v", result)
	}
	var groupState struct {
		Order  int64 `db:"orderr"`
		Linked bool  `db:"linked"`
		Count  int64 `db:"channel_count"`
	}
	if err := db.Get(&groupState, `SELECT tgi.orderr,
		EXISTS(SELECT 1 FROM lineup_group_source_link WHERE group_id = tg.id) AS linked,
		COUNT(tgc.channel_id) AS channel_count
		FROM templategroup tg
		JOIN template_group_item tgi ON tgi.group_id = tg.id
		LEFT JOIN template_group_channel tgc ON tgc.group_id = tg.id
		WHERE tg.id = ? GROUP BY tg.id, tgi.orderr`, result.GroupID); err != nil {
		t.Fatalf("copied group state was incomplete: %v", err)
	}
	if groupState.Order != 5 || groupState.Linked || groupState.Count != 2 {
		t.Fatalf("unexpected copied group state: %#v", groupState)
	}
	var lockedCount int64
	db.Get(&lockedCount, `SELECT COUNT(*) FROM templatechannelitem tci
		JOIN template_group_channel tgc ON tgc.channel_id = tci.channel_id
		WHERE tgc.group_id = ? AND tci.match_method = 'manual' AND tci.manual_locked = 1`, result.GroupID)
	if lockedCount != 2 {
		t.Fatalf("expected two manually locked source variants, got %d", lockedCount)
	}
}

func TestAddSourceGroupToStudioGroupSkipsExistingVariants(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', 'https://example.test/list.m3u', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (20, 'News', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, 'alpha.tv', 'Alpha Guide', NULL, 'Alpha', 20, 1),
		(101, 'beta.tv', 'Beta Guide', NULL, 'Beta', 20, 1)`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'Destination', 0, NULL)`)
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'Alpha', 'alpha.tv', 0, 'existing')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES (10, 1, 1)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES (1, 1, 100, 1, 'manual', 1, NULL, 2, 1)`)

	query := NewExperienceQueries(db)
	first, err := query.AddSourceGroupToStudioGroup(context.Background(), 10, 20)
	if err != nil {
		t.Fatalf("AddSourceGroupToStudioGroup failed: %v", err)
	}
	if first.AddedCount != 1 || first.SkippedCount != 1 || len(first.ChannelIDs) != 1 {
		t.Fatalf("unexpected first import: %#v", first)
	}
	second, err := query.AddSourceGroupToStudioGroup(context.Background(), 10, 20)
	if err != nil {
		t.Fatalf("repeated AddSourceGroupToStudioGroup failed: %v", err)
	}
	if second.AddedCount != 0 || second.SkippedCount != 2 || len(second.ChannelIDs) != 0 {
		t.Fatalf("repeated import was not idempotent: %#v", second)
	}
	var channelCount int64
	db.Get(&channelCount, `SELECT COUNT(*) FROM template_group_channel WHERE group_id = 10`)
	if channelCount != 2 {
		t.Fatalf("expected two destination channels, got %d", channelCount)
	}
}

func TestCopySourceGroupToLineupRollsBackEmptySource(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main')`)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', 'https://example.test/list.m3u', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (20, 'Empty', 1, 1)`)
	query := NewExperienceQueries(db)
	if _, err := query.CopySourceGroupToLineup(context.Background(), 1, 20); !errors.Is(err, ErrSourceGroupEmpty) {
		t.Fatalf("expected an empty source error, got %v", err)
	}
	for _, table := range []string{"templategroup", "template_group_item", "templatechannel"} {
		var count int64
		db.Get(&count, `SELECT COUNT(*) FROM `+table)
		if count != 0 {
			t.Fatalf("%s retained %d rows after empty-source rollback", table, count)
		}
	}
}

func TestSourceGroupSyncReconcilesOwnedMembershipAndPreservesEnrichment(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', 'https://example.test/list.m3u', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (20, 'Provider News', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, 'alpha.tv', 'Provider Alpha', NULL, 'Alpha', 20, 1),
		(101, NULL, 'Provider Local', NULL, 'Local', 20, 1)`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL)`)
	query := NewExperienceQueries(db)
	request := models.SourceGroupLinkRequest{SourceGroupID: 20, FollowGroupName: true, FollowChannelNames: true}
	if err := query.SetSourceGroupLink(context.Background(), 10, request); err != nil {
		t.Fatalf("SetSourceGroupLink failed: %v", err)
	}
	first, err := query.SyncSourceGroup(context.Background(), 10)
	if err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}
	if first.AddedCount != 2 || first.RemovedCount != 0 {
		t.Fatalf("unexpected initial result: %#v", first)
	}
	var channels []struct {
		ID     int64  `db:"id"`
		Name   string `db:"name"`
		TVGID  string `db:"tvgid"`
		LogoID int64  `db:"logoid"`
	}
	db.Select(&channels, `SELECT tc.id, tc.name, COALESCE(tc.tvgid, '') AS tvgid, tc.logoid FROM templatechannel tc JOIN template_group_channel tgc ON tgc.channel_id = tc.id WHERE tgc.group_id = 10 ORDER BY tgc.orderr`)
	if len(channels) != 2 || channels[0].Name != "Alpha" || channels[1].Name != "Local" {
		t.Fatalf("source membership was not mirrored: %#v", channels)
	}
	alphaID := channels[0].ID
	db.MustExec(`UPDATE templatechannel SET tvgid = 'custom.guide', logoid = 42 WHERE id = ?`, alphaID)
	db.MustExec(`UPDATE playlistchannel SET title = 'Alpha Renamed', tvg_id = 'provider.changed' WHERE id = 100`)
	db.MustExec(`DELETE FROM playlistchannel WHERE id = 101`)
	second, err := query.SyncSourceGroup(context.Background(), 10)
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if second.UpdatedCount != 1 || second.RemovedCount != 1 {
		t.Fatalf("unexpected reconciliation result: %#v", second)
	}
	channels = nil
	db.Select(&channels, `SELECT tc.id, tc.name, COALESCE(tc.tvgid, '') AS tvgid, tc.logoid FROM templatechannel tc JOIN template_group_channel tgc ON tgc.channel_id = tc.id WHERE tgc.group_id = 10 ORDER BY tgc.orderr`)
	if len(channels) != 1 || channels[0].Name != "Alpha Renamed" || channels[0].TVGID != "custom.guide" || channels[0].LogoID != 42 {
		t.Fatalf("source names or Xivi enrichment were reconciled incorrectly: %#v", channels)
	}
	link, err := query.GetSourceGroupLink(context.Background(), 10)
	if err != nil || link.Status != "active" || link.RemovedCount != 1 || link.LastSyncedAt == nil {
		t.Fatalf("unexpected link status: %#v err=%v", link, err)
	}
}

func TestSourceGroupSyncRetainsLastSnapshotWhenSourceBecomesEmpty(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', 'https://example.test/list.m3u', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (20, 'Provider News', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES (100, 'alpha.tv', 'Provider Alpha', NULL, 'Alpha', 20, 1)`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL)`)
	query := NewExperienceQueries(db)
	if err := query.SetSourceGroupLink(context.Background(), 10, models.SourceGroupLinkRequest{SourceGroupID: 20, FollowGroupName: true, FollowChannelNames: true}); err != nil {
		t.Fatalf("SetSourceGroupLink failed: %v", err)
	}
	if _, err := query.SyncSourceGroup(context.Background(), 10); err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}
	db.MustExec(`DELETE FROM playlistchannel WHERE group_id = 20`)
	if _, err := query.SyncSourceGroup(context.Background(), 10); err == nil {
		t.Fatal("expected an empty source group to fail safely")
	}
	var memberships int
	db.Get(&memberships, `SELECT COUNT(*) FROM template_group_channel WHERE group_id = 10`)
	if memberships != 1 {
		t.Fatalf("last valid snapshot was not retained: %d memberships", memberships)
	}
	link, err := query.GetSourceGroupLink(context.Background(), 10)
	if err != nil || link.Status != "error" || !strings.Contains(link.LastError, "previous lineup snapshot was retained") {
		t.Fatalf("failure was not exposed on the source link: %#v err=%v", link, err)
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
