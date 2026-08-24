//go:build cgo

package queries

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/logoassets"
	"xivi/backend/platform/settings"

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
		`CREATE TABLE templatechannelitem (id INTEGER PRIMARY KEY, channel_id INTEGER, playlist_channel_id INTEGER, orderr INTEGER, match_method TEXT, match_score REAL, runner_up_score REAL, matcher_version INTEGER, manual_locked BOOLEAN, FOREIGN KEY (channel_id) REFERENCES templatechannel(id) ON DELETE CASCADE, UNIQUE(channel_id, playlist_channel_id) ON CONFLICT IGNORE)`,
		`CREATE TABLE channelmatchrejection (id INTEGER PRIMARY KEY, channel_id INTEGER, playlist_id INTEGER, tvg_id_norm TEXT NOT NULL DEFAULT '', name_norm TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, UNIQUE(channel_id, playlist_id, tvg_id_norm, name_norm) ON CONFLICT IGNORE)`,
		`CREATE TABLE channelurl (id INTEGER PRIMARY KEY, url TEXT, channel_id INTEGER, orderr INTEGER, created_at DATETIME, updated_at DATETIME)`,
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

func TestGetGuideChannelsReturnsAnEmptyProgrammeArray(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main')`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL)`)
	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 1)`)
	db.MustExec(`INSERT INTO logo VALUES (0, 'xivi_channel')`)
	db.MustExec(`INSERT INTO templatechannel VALUES (100, 'No Schedule', 'missing.tv', 0, 'missing-uuid')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES (10, 100, 1)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES (1, 100, 1000, 0, 'manual', 1, NULL, 2, 1)`)

	from := time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC)
	channels, total, err := NewExperienceQueries(db).GetGuideChannels(context.Background(), 1, nil, "", from, from.Add(24*time.Hour), 100, 0)
	if err != nil {
		t.Fatalf("GetGuideChannels returned an error: %v", err)
	}
	if total != 1 || len(channels) != 1 {
		t.Fatalf("expected one channel, total=%d len=%d", total, len(channels))
	}
	if channels[0].Programmes == nil || len(channels[0].Programmes) != 0 {
		t.Fatalf("expected a non-nil empty programme array, got %#v", channels[0].Programmes)
	}
	payload, err := json.Marshal(channels[0])
	if err != nil {
		t.Fatalf("marshal guide channel: %v", err)
	}
	if strings.Contains(string(payload), `"programmes":null`) {
		t.Fatalf("guide contract emitted a null programme array: %s", payload)
	}
}

func TestGetGuideChannelsUsesConfiguredTimezoneForProgrammeWindow(t *testing.T) {
	originalTZ := settings.APP_SETTINGS.Application.TZ
	settings.APP_SETTINGS.Application.TZ = "America/New_York"
	t.Cleanup(func() { settings.APP_SETTINGS.Application.TZ = originalTZ })

	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main')`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'Local', 0, NULL)`)
	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 1)`)
	db.MustExec(`INSERT INTO logo VALUES (0, 'xivi_channel')`)
	db.MustExec(`INSERT INTO templatechannel VALUES (100, 'ABC WPVI', 'ABCWPVI.us', 0, 'wpvi-uuid')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES (10, 100, 1)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES (1, 100, 1000, 0, 'manual', 1, NULL, 2, 1)`)

	location, err := time.LoadLocation(settings.APP_SETTINGS.Application.TZ)
	if err != nil {
		t.Fatalf("load test timezone: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	insert := `INSERT INTO epgprogramme (id, start, stop, channel, "title.value", subtitle, desc, categories) VALUES (?, ?, ?, 'ABCWPVI.us', ?, '', '', '')`
	db.MustExec(insert, 1, now.Add(-15*time.Minute).In(location), now.Add(15*time.Minute).In(location), "Current local programme")
	db.MustExec(insert, 2, now.Add(15*time.Minute).In(location), now.Add(75*time.Minute).In(location), "Next local programme")

	channels, total, err := NewExperienceQueries(db).GetGuideChannels(
		context.Background(), 1, nil, "", now.Add(-30*time.Minute), now.Add(4*time.Hour), 100, 0,
	)
	if err != nil {
		t.Fatalf("GetGuideChannels returned an error: %v", err)
	}
	if total != 1 || len(channels) != 1 {
		t.Fatalf("expected one playable channel, total=%d len=%d", total, len(channels))
	}
	if len(channels[0].Programmes) != 2 {
		t.Fatalf("expected current and next programmes in the UTC request window, got %#v", channels[0].Programmes)
	}
	if channels[0].Current == nil || channels[0].Current.Title != "Current local programme" {
		t.Fatalf("expected the current local programme, got %#v", channels[0].Current)
	}
	if channels[0].Next == nil || channels[0].Next.Title != "Next local programme" {
		t.Fatalf("expected the next local programme, got %#v", channels[0].Next)
	}
}

func TestStudioOverviewSeparatesUnmatchedAndLowConfidenceChannels(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main')`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL)`)
	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 1)`)
	db.MustExec(`INSERT INTO logo VALUES (0, 'xivi_channel')`)
	db.MustExec(`INSERT INTO templatechannel VALUES
		(1, 'Unmatched', NULL, 0, 'unmatched'),
		(2, 'Low confidence', NULL, 0, 'low'),
		(3, 'Healthy automatic', NULL, 0, 'healthy'),
		(4, 'Manually resolved', NULL, 0, 'manual')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES (10, 1, 1), (10, 2, 2), (10, 3, 3), (10, 4, 4)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES
		(1, 2, 102, 1, 'name', 0.7, 0.6, 2, 0),
		(2, 3, 103, 1, 'name', 0.95, 0.4, 2, 0),
		(3, 4, 104, 1, 'name', 0.65, 0.6, 2, 0),
		(4, 4, 105, 2, 'manual', 1, NULL, 2, 1)`)

	query := NewExperienceQueries(db)
	overview, err := query.GetStudioOverview(context.Background())
	if err != nil {
		t.Fatalf("GetStudioOverview failed: %v", err)
	}
	if overview.UnmatchedCount != 1 || overview.LowConfidenceCount != 1 || overview.ReviewCount != 2 {
		t.Fatalf("unexpected match-health summary: %#v", overview)
	}

	unmatched, unmatchedTotal, err := query.GetWorkspaceChannels(context.Background(), 10, "", "unmatched", 100, 0)
	if err != nil || unmatchedTotal != 1 || len(unmatched) != 1 || unmatched[0].Name != "Unmatched" {
		t.Fatalf("unexpected unmatched filter: total=%d rows=%#v err=%v", unmatchedTotal, unmatched, err)
	}
	low, lowTotal, err := query.GetWorkspaceChannels(context.Background(), 10, "", "low-confidence", 100, 0)
	if err != nil || lowTotal != 1 || len(low) != 1 || low[0].Name != "Low confidence" {
		t.Fatalf("unexpected low-confidence filter: total=%d rows=%#v err=%v", lowTotal, low, err)
	}
}

func TestGetMatchSuggestionsRanksEligibleUnattachedSources(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'BBC World News', 'bbc.world', 0, 'bbc')`)
	db.MustExec(`INSERT INTO playlist VALUES
		(1, 'Already attached', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(2, 'TVG provider', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(3, 'Name provider', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(4, 'Fuzzy provider', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(5, 'Rejected provider', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(6, 'Disabled channel', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(7, 'Disabled group', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES
		(10, 'News', 1, 1), (20, 'News', 2, 1), (30, 'News', 3, 1),
		(40, 'News', 4, 1), (50, 'News', 5, 1), (60, 'News', 6, 1),
		(70, 'News', 7, 0)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, 'bbc.world', 'BBC World News', 'https://logos.test/attached.png', 'BBC World News', 10, 1),
		(200, 'BBC.WORLD', 'Different provider title', 'https://logos.test/tvg.png', 'Different provider title', 20, 1),
		(300, NULL, 'BBC World News', NULL, 'BBC World News', 30, 1),
		(400, NULL, 'BBC World News International', NULL, 'BBC World News International', 40, 1),
		(500, NULL, 'BBC World News', NULL, 'BBC World News', 50, 1),
		(600, NULL, 'BBC World News', NULL, 'BBC World News', 60, 0),
		(700, NULL, 'BBC World News', NULL, 'BBC World News', 70, 1)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES (1, 1, 100, 1, 'manual', 1, NULL, 2, 1)`)
	db.MustExec(`INSERT INTO channelmatchrejection (id, channel_id, playlist_id, name_norm) VALUES (1, 1, 5, 'bbc world news')`)

	suggestions, total, err := NewExperienceQueries(db).GetMatchSuggestions(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("GetMatchSuggestions failed: %v", err)
	}
	if total != 3 || len(suggestions) != 3 {
		t.Fatalf("expected three eligible suggestions, total=%d suggestions=%#v", total, suggestions)
	}
	if suggestions[0].SourceChannelID != 200 || suggestions[0].Score != 1 || suggestions[0].Method != "exact_tvg_id" {
		t.Fatalf("exact TVG-ID suggestion did not rank first: %#v", suggestions[0])
	}
	if suggestions[1].SourceChannelID != 300 || suggestions[1].Score != 1 || suggestions[1].Method != "exact_name" {
		t.Fatalf("exact-name suggestion did not rank second: %#v", suggestions[1])
	}
	if suggestions[2].SourceChannelID != 400 || suggestions[2].Score >= 1 || suggestions[2].Method != "fuzzy_name" {
		t.Fatalf("fuzzy suggestion was not ranked after exact matches: %#v", suggestions[2])
	}
	if suggestions[0].LogoURL == nil || *suggestions[0].LogoURL != "https://logos.test/tvg.png" {
		t.Fatalf("source metadata was not preserved: %#v", suggestions[0])
	}
	if suggestions[0].RunnerUpScore == nil || *suggestions[0].RunnerUpScore != suggestions[1].Score {
		t.Fatalf("runner-up confidence was not calculated: %#v", suggestions[0])
	}
}

func TestGetSourceChannelsCanHideSourcesUsedByTheActiveLineup(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main'), (2, 'Other')`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'Main group', 0, NULL), (20, 'Other group', 0, NULL)`)
	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 1), (2, 20, 1)`)
	db.MustExec(`INSERT INTO templatechannel VALUES
		(1, 'Used here', NULL, 0, 'used-here'),
		(2, 'Used elsewhere', NULL, 0, 'used-elsewhere'),
		(3, 'Also used here', NULL, 0, 'also-used-here')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES (10, 1, 1), (10, 3, 2), (20, 2, 1)`)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES
		(30, 'Partly available', 1, 1),
		(31, 'Fully used', 1, 1),
		(32, 'Empty', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, NULL, 'Used here', NULL, 'Used here', 30, 1),
		(101, NULL, 'Used elsewhere', NULL, 'Used elsewhere', 30, 1),
		(102, NULL, 'Unused', NULL, 'Unused', 30, 1),
		(103, NULL, 'Also used here', NULL, 'Also used here', 31, 1)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES
		(1, 1, 100, 1, 'manual', 1, NULL, 2, 1),
		(2, 2, 101, 1, 'manual', 1, NULL, 2, 1),
		(3, 3, 103, 1, 'manual', 1, NULL, 2, 1)`)

	lineupID := int64(1)
	items, total, err := NewExperienceQueries(db).GetSourceChannels(context.Background(), nil, nil, &lineupID, true, false, "", 50, 0)
	if err != nil {
		t.Fatalf("GetSourceChannels failed: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected two sources unused by this lineup, total=%d items=%#v", total, items)
	}
	if items[0].ID != 102 || items[1].ID != 101 {
		t.Fatalf("filter hid a source used only by another lineup or retained one used here: %#v", items)
	}

	all, allTotal, err := NewExperienceQueries(db).GetSourceChannels(context.Background(), nil, nil, &lineupID, false, false, "", 50, 0)
	if err != nil || allTotal != 4 || len(all) != 4 {
		t.Fatalf("unfiltered source catalog changed: total=%d items=%#v err=%v", allTotal, all, err)
	}

	groups, groupTotal, err := NewExperienceQueries(db).GetSourceGroups(context.Background(), nil, &lineupID, true, false, "", 50, 0)
	if err != nil {
		t.Fatalf("GetSourceGroups failed: %v", err)
	}
	if groupTotal != 1 || len(groups) != 1 || groups[0].ID != 30 || groups[0].ChannelCount != 2 {
		t.Fatalf("expected only the partly available group with two unused sources, total=%d groups=%#v", groupTotal, groups)
	}

	allGroups, allGroupTotal, err := NewExperienceQueries(db).GetSourceGroups(context.Background(), nil, &lineupID, false, false, "", 50, 0)
	if err != nil || allGroupTotal != 2 || len(allGroups) != 2 {
		t.Fatalf("expected non-empty groups when the usage filter is off, total=%d groups=%#v err=%v", allGroupTotal, allGroups, err)
	}
	if allGroups[0].ChannelCount != 1 || allGroups[1].ChannelCount != 3 {
		t.Fatalf("unfiltered group counts changed: %#v", allGroups)
	}
}

func TestSourceBrowserExcludesDisabledGroupsAndChannels(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES
		(10, 'Mixed', 1, 1),
		(20, 'Disabled group', 1, 0),
		(30, 'Only disabled channels', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, NULL, 'Visible', NULL, 'Visible', 10, 1),
		(101, NULL, 'Disabled channel', NULL, 'Disabled channel', 10, 0),
		(200, NULL, 'Hidden with group', NULL, 'Hidden with group', 20, 1),
		(300, NULL, 'Only disabled', NULL, 'Only disabled', 30, 0)`)

	query := NewExperienceQueries(db)
	channels, channelTotal, err := query.GetSourceChannels(context.Background(), nil, nil, nil, false, true, "", 50, 0)
	if err != nil {
		t.Fatalf("GetSourceChannels failed: %v", err)
	}
	if channelTotal != 1 || len(channels) != 1 || channels[0].ID != 100 {
		t.Fatalf("expected only the enabled channel in an enabled group, total=%d channels=%#v", channelTotal, channels)
	}

	groups, groupTotal, err := query.GetSourceGroups(context.Background(), nil, nil, false, true, "", 50, 0)
	if err != nil {
		t.Fatalf("GetSourceGroups failed: %v", err)
	}
	if groupTotal != 1 || len(groups) != 1 || groups[0].ID != 10 || groups[0].ChannelCount != 1 {
		t.Fatalf("expected only the enabled non-empty group with its visible count, total=%d groups=%#v", groupTotal, groups)
	}

	allChannels, allChannelTotal, err := query.GetSourceChannels(context.Background(), nil, nil, nil, false, false, "", 50, 0)
	if err != nil || allChannelTotal != 4 || len(allChannels) != 4 {
		t.Fatalf("management catalog must retain disabled channels, total=%d channels=%#v err=%v", allChannelTotal, allChannels, err)
	}
	allGroups, allGroupTotal, err := query.GetSourceGroups(context.Background(), nil, nil, false, false, "", 50, 0)
	if err != nil || allGroupTotal != 3 || len(allGroups) != 3 {
		t.Fatalf("management catalog must retain disabled groups, total=%d groups=%#v err=%v", allGroupTotal, allGroups, err)
	}
}

func TestGetMatchSuggestionsRequiresExistingChannel(t *testing.T) {
	_, _, err := NewExperienceQueries(newExperienceTestDB(t)).GetMatchSuggestions(context.Background(), 999, 5)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for a missing channel, got %v", err)
	}
}

func TestGetMatchSuggestionsIncludesBackupFromRepresentedPlaylist(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'TYT Network', 'tyt.us', 0, 'tyt')`)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (10, 'News', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, 'tyt.us', 'TYT Network', NULL, 'TYT Network', 10, 1),
		(101, 'tyt.us', 'TYT Network Backup', NULL, 'TYT Network Backup', 10, 1),
		(102, 'tcm.us', 'TCM', NULL, 'TCM Cinema', 10, 1)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES (1, 1, 100, 1, 'manual', 1, NULL, 2, 1)`)

	suggestions, total, err := NewExperienceQueries(db).GetMatchSuggestions(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("GetMatchSuggestions failed: %v", err)
	}
	if total != 2 || len(suggestions) != 2 || suggestions[0].SourceChannelID != 101 {
		t.Fatalf("expected both eligible sources with the closest first, total=%d suggestions=%#v", total, suggestions)
	}
	if suggestions[0].Method != "exact_tvg_id_name" || suggestions[0].Score != 1 {
		t.Fatalf("expected TVG-ID backup confidence, got %#v", suggestions[0])
	}
	if suggestions[1].SourceChannelID != 102 || suggestions[1].Score >= suggestions[0].Score {
		t.Fatalf("expected the weak source to remain ranked after the backup, got %#v", suggestions)
	}
}

func TestGetMatchSuggestionsReturnsFiveOfAllEligibleSources(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'US TYT NETWORK', NULL, 0, 'tyt')`)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (10, 'Mixed', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, NULL, 'TYT Network', NULL, 'TYT Network', 10, 1),
		(101, NULL, 'TCM', NULL, 'TCM', 10, 1),
		(102, NULL, 'BBC News', NULL, 'BBC News', 10, 1),
		(103, NULL, 'ESPN 2', NULL, 'ESPN 2', 10, 1),
		(104, NULL, 'Cartoon Network', NULL, 'Cartoon Network', 10, 1),
		(105, NULL, 'Discovery', NULL, 'Discovery', 10, 1)`)

	suggestions, total, err := NewExperienceQueries(db).GetMatchSuggestions(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("GetMatchSuggestions failed: %v", err)
	}
	if total != 6 || len(suggestions) != 5 {
		t.Fatalf("expected top five of six eligible sources, total=%d suggestions=%#v", total, suggestions)
	}
	if suggestions[0].SourceChannelID != 100 {
		t.Fatalf("closest suggestion did not rank first: %#v", suggestions)
	}
}

func TestManualMatchStackAllowsSamePlaylistAndCanBeReordered(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`CREATE TRIGGER templatechannelitem_one_automatic_source_per_playlist
		BEFORE INSERT ON templatechannelitem
		WHEN COALESCE(NEW.manual_locked, false) = false AND EXISTS (
			SELECT 1 FROM templatechannelitem existing
			JOIN playlistchannel existing_pc ON existing_pc.id = existing.playlist_channel_id
			JOIN playlistgroup existing_pg ON existing_pg.id = existing_pc.group_id
			JOIN playlistchannel new_pc ON new_pc.id = NEW.playlist_channel_id
			JOIN playlistgroup new_pg ON new_pg.id = new_pc.group_id
			WHERE existing.channel_id = NEW.channel_id AND existing_pg.playlist_id = new_pg.playlist_id
		) BEGIN SELECT RAISE(IGNORE); END`)
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'News', NULL, 0, 'news')`)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (10, 'News', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, 'news.tv', 'News primary', NULL, 'News primary', 10, 1),
		(101, 'news.tv', 'News backup', NULL, 'News backup', 10, 1),
		(102, 'news.tv', 'News automatic', NULL, 'News automatic', 10, 1)`)

	query := NewExperienceQueries(db)
	if err := query.AttachManualMatch(context.Background(), 1, 100); err != nil {
		t.Fatalf("attaching the primary failed: %v", err)
	}
	if err := query.AttachManualMatch(context.Background(), 1, 101); err != nil {
		t.Fatalf("attaching a same-playlist backup failed: %v", err)
	}

	primary := int64(100)
	if err := query.MoveMatchVariant(context.Background(), 1, 101, &primary, nil); err != nil {
		t.Fatalf("moving the backup to primary failed: %v", err)
	}
	var ordered []int64
	if err := db.Select(&ordered, `SELECT playlist_channel_id FROM templatechannelitem WHERE channel_id = 1 ORDER BY orderr, id`); err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 2 || ordered[0] != 101 || ordered[1] != 100 {
		t.Fatalf("unexpected failover order: %v", ordered)
	}

	db.MustExec(`INSERT INTO templatechannelitem
		(channel_id, playlist_channel_id, orderr, match_method, matcher_version, manual_locked)
		VALUES (1, 102, 3, 'exact_name', 2, false)`)
	var count int
	db.Get(&count, `SELECT COUNT(*) FROM templatechannelitem WHERE channel_id = 1`)
	if count != 2 {
		t.Fatalf("automatic matching added a second source from the represented playlist; count=%d", count)
	}
	if err := NewTemplateQueries(db).DeleteTmplChannelItem(&models.TemplateChannelItem{
		ChannelId: 1, PlaylistChannelId: 101,
	}); err != nil {
		t.Fatalf("removing the primary variant failed: %v", err)
	}
	var remainingOrder int64
	db.Get(&remainingOrder, `SELECT orderr FROM templatechannelitem WHERE channel_id = 1 AND playlist_channel_id = 100`)
	if remainingOrder != 1 {
		t.Fatalf("remaining variant order was not compacted: %d", remainingOrder)
	}
}

func TestSourceGroupEnablementPreservesChannelsAndPausesLinkedSnapshots(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (20, 'News', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, 'alpha.tv', 'Alpha', NULL, 'Alpha', 20, 1),
		(101, 'beta.tv', 'Beta', NULL, 'Beta', 20, 0)`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL)`)
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'Alpha', 'alpha.tv', 0, 'alpha')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES (10, 1, 1)`)
	db.MustExec(`INSERT INTO lineup_group_source_link
		(group_id, playlist_id, source_group_id, source_group_name, status)
		VALUES (10, 1, 20, 'News', 'active')`)
	db.MustExec(`INSERT INTO lineup_group_source_member
		(group_id, template_channel_id, source_channel_id, source_identity, last_source_name)
		VALUES (10, 1, 100, 'tvg:alpha.tv', 'Alpha')`)

	query := NewExperienceQueries(db)
	if err := query.SetSourceGroupEnabled(context.Background(), 20, false); err != nil {
		t.Fatalf("disabling source group failed: %v", err)
	}
	var groupEnabled bool
	db.Get(&groupEnabled, `SELECT enabled FROM playlistgroup WHERE id = 20`)
	if groupEnabled {
		t.Fatal("source group remained enabled")
	}
	channelStates := []bool{}
	db.Select(&channelStates, `SELECT enabled FROM playlistchannel WHERE group_id = 20 ORDER BY id`)
	if len(channelStates) != 2 || !channelStates[0] || channelStates[1] {
		t.Fatalf("channel-level choices were overwritten: %#v", channelStates)
	}
	if err := NewCleanupQueries(db).CleanPlaylistChannels(context.Background(), 1); err != nil {
		t.Fatalf("disabled-group cleanup failed: %v", err)
	}
	var retainedSourceChannels int
	db.Get(&retainedSourceChannels, `SELECT COUNT(*) FROM playlistchannel WHERE group_id = 20`)
	if retainedSourceChannels != 2 {
		t.Fatalf("disabled source group lost its retained snapshot: %d channels", retainedSourceChannels)
	}
	link, err := query.GetSourceGroupLink(context.Background(), 10)
	if err != nil || link.Status != "paused" || !strings.Contains(link.LastError, "previous lineup snapshot") {
		t.Fatalf("linked snapshot was not paused clearly: link=%#v err=%v", link, err)
	}
	results, err := query.SyncSourceGroupsForPlaylist(context.Background(), 1)
	if err != nil || len(results) != 0 {
		t.Fatalf("playlist refresh did not skip its paused source group: results=%#v err=%v", results, err)
	}
	if _, err := query.SyncSourceGroup(context.Background(), 10); !errors.Is(err, ErrSourceGroupDisabled) {
		t.Fatalf("disabled source group was allowed to sync: %v", err)
	}
	var memberCount int
	db.Get(&memberCount, `SELECT COUNT(*) FROM lineup_group_source_member WHERE group_id = 10`)
	if memberCount != 1 {
		t.Fatalf("paused sync changed the retained lineup snapshot: %d members", memberCount)
	}

	if err := query.SetSourceGroupEnabled(context.Background(), 20, true); err != nil {
		t.Fatalf("re-enabling source group failed: %v", err)
	}
	link, err = query.GetSourceGroupLink(context.Background(), 10)
	if err != nil || link.Status != "pending" || link.LastError != "" {
		t.Fatalf("re-enabled source group was not ready to resume: link=%#v err=%v", link, err)
	}
	if err := query.SetSourceGroupEnabled(context.Background(), 999, false); !errors.Is(err, ErrSourceGroupNotFound) {
		t.Fatalf("missing source group returned the wrong error: %v", err)
	}
}

func TestStreamSourcesFollowVariantThenURLOrder(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'News', NULL, 0, 'news-uuid')`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (10, 'News', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, NULL, 'Backup', NULL, 'Backup', 10, 1),
		(101, NULL, 'Primary', NULL, 'Primary', 10, 1)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES
		(1, 1, 100, 2, 'manual', 1, NULL, 2, 1),
		(2, 1, 101, 1, 'manual', 1, NULL, 2, 1)`)
	db.MustExec(`INSERT INTO channelurl VALUES
		(1, 'primary-second', 101, 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(2, 'backup-first', 100, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(3, 'primary-first', 101, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	channels, err := NewStreamQueries(db).GetChannelsbyUuid(context.Background(), "news-uuid")
	if err != nil {
		t.Fatalf("GetChannelsbyUuid failed: %v", err)
	}
	if len(*channels) != 3 || (*channels)[0].Url != "primary-first" || (*channels)[1].Url != "primary-second" || (*channels)[2].Url != "backup-first" {
		t.Fatalf("unexpected source order: %#v", *channels)
	}
}

func TestStreamSourcesSkipDisabledChannelsAndGroups(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'News', NULL, 0, 'news-uuid')`)
	db.MustExec(`INSERT INTO playlistgroup VALUES
		(10, 'Available', 1, 1),
		(20, 'Paused', 1, 0)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, NULL, 'Available', NULL, 'Available', 10, 1),
		(101, NULL, 'Channel disabled', NULL, 'Channel disabled', 10, 0),
		(102, NULL, 'Group disabled', NULL, 'Group disabled', 20, 1)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES
		(1, 1, 100, 1, 'manual', 1, NULL, 2, 1),
		(2, 1, 101, 2, 'manual', 1, NULL, 2, 1),
		(3, 1, 102, 3, 'manual', 1, NULL, 2, 1)`)
	db.MustExec(`INSERT INTO channelurl VALUES
		(1, 'available', 100, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(2, 'channel-disabled', 101, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(3, 'group-disabled', 102, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	channels, err := NewStreamQueries(db).GetChannelsbyUuid(context.Background(), "news-uuid")
	if err != nil {
		t.Fatalf("GetChannelsbyUuid failed: %v", err)
	}
	if len(*channels) != 1 || (*channels)[0].Url != "available" {
		t.Fatalf("disabled source variants reached playback: %#v", *channels)
	}
}

func TestMatchRejectionRestoreResolvesOnlyUniqueEligibleSource(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO playlist VALUES
		(1, 'Resolvable', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(2, 'Ambiguous', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		(3, 'Already represented', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES
		(10, 'News', 1, 1), (20, 'News', 2, 1), (30, 'News', 3, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES
		(100, 'alpha.tv', 'Alpha', 'https://logos.test/alpha.png', 'Alpha', 10, 1),
		(200, 'beta.tv', 'Beta one', NULL, 'Beta', 20, 1),
		(201, 'beta.tv', 'Beta two', NULL, 'Beta', 20, 1),
		(300, 'gamma.tv', 'Gamma', NULL, 'Gamma', 30, 1)`)
	db.MustExec(`INSERT INTO templatechannel VALUES (1, 'Target', NULL, 0, 'target')`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES (1, 1, 300, 1, 'manual', 1, NULL, 2, 1)`)
	db.MustExec(`INSERT INTO channelmatchrejection (id, channel_id, playlist_id, tvg_id_norm, name_norm) VALUES
		(1, 1, 1, 'alpha.tv', 'alpha'),
		(2, 1, 2, 'beta.tv', 'beta'),
		(3, 1, 3, 'gamma.tv', 'gamma')`)

	query := NewExperienceQueries(db)
	rejections, err := query.GetMatchRejections(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetMatchRejections failed: %v", err)
	}
	if len(rejections) != 3 {
		t.Fatalf("expected three rejections, got %#v", rejections)
	}
	byID := make(map[int64]models.MatchRejection, len(rejections))
	for _, rejection := range rejections {
		byID[rejection.ID] = rejection
	}
	if byID[1].SourceChannelID == nil || *byID[1].SourceChannelID != 100 || byID[1].SourceName == nil || *byID[1].SourceName != "Alpha" {
		t.Fatalf("unique current source was not resolved: %#v", byID[1])
	}
	if byID[1].LogoURL == nil || *byID[1].LogoURL != "https://logos.test/alpha.png" {
		t.Fatalf("resolved source logo was not preserved: %#v", byID[1])
	}
	if byID[2].SourceChannelID != nil {
		t.Fatalf("ambiguous source should not be attachable: %#v", byID[2])
	}
	if byID[3].SourceChannelID != nil {
		t.Fatalf("the exact source already attached should not be attachable again: %#v", byID[3])
	}

	deleted, err := query.DeleteMatchRejection(context.Background(), 1, 1)
	if err != nil || !deleted {
		t.Fatalf("DeleteMatchRejection failed: deleted=%v err=%v", deleted, err)
	}
	deleted, err = query.DeleteMatchRejection(context.Background(), 1, 1)
	if err != nil || deleted {
		t.Fatalf("repeated restore should report not found: deleted=%v err=%v", deleted, err)
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

func TestMoveWorkspaceGroupRequiresOneAnchor(t *testing.T) {
	db := newExperienceTestDB(t)
	query := NewExperienceQueries(db)
	if err := query.MoveWorkspaceGroup(context.Background(), 1, 1, nil, nil); err == nil {
		t.Fatal("expected a validation error without before_id or after_id")
	}
	anchor := int64(2)
	if err := query.MoveWorkspaceGroup(context.Background(), 1, 1, &anchor, &anchor); err == nil {
		t.Fatal("expected a validation error with both anchors")
	}
}

func TestMoveWorkspaceGroupReindexesOnlyItsLineup(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main'), (2, 'Other')`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'One', 0, NULL), (20, 'Two', 0, NULL), (30, 'Three', 0, NULL)`)
	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 10), (1, 20, 20), (1, 30, 30), (2, 10, 99)`)
	query := NewExperienceQueries(db)
	third := int64(30)
	if err := query.MoveWorkspaceGroup(context.Background(), 1, 10, nil, &third); err != nil {
		t.Fatalf("moving after a later group failed: %v", err)
	}
	var ids []int64
	db.Select(&ids, `SELECT group_id FROM template_group_item WHERE template_id = 1 ORDER BY orderr`)
	if got, want := ids, []int64{20, 30, 10}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("unexpected order after rightward move: %v", got)
	}
	var otherOrder int64
	db.Get(&otherOrder, `SELECT orderr FROM template_group_item WHERE template_id = 2 AND group_id = 10`)
	if otherOrder != 99 {
		t.Fatalf("another lineup's position changed to %d", otherOrder)
	}
	second := int64(20)
	if err := query.MoveWorkspaceGroup(context.Background(), 1, 10, &second, nil); err != nil {
		t.Fatalf("moving before an earlier group failed: %v", err)
	}
	ids = nil
	db.Select(&ids, `SELECT group_id FROM template_group_item WHERE template_id = 1 ORDER BY orderr`)
	if got, want := ids, []int64{10, 20, 30}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("unexpected order after leftward move: %v", got)
	}
}

func TestMoveWorkspaceGroupRejectsAnchorOutsideLineup(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO template VALUES (1, 'Main'), (2, 'Other')`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'Main group', 0, NULL), (20, 'Other group', 0, NULL)`)
	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 1), (2, 20, 1)`)
	anchor := int64(20)
	err := NewExperienceQueries(db).MoveWorkspaceGroup(context.Background(), 1, 10, &anchor, nil)
	if !errors.Is(err, ErrStudioGroupNotFound) {
		t.Fatalf("expected a lineup-scoped group error, got %v", err)
	}
	var order int64
	db.Get(&order, `SELECT orderr FROM template_group_item WHERE template_id = 1 AND group_id = 10`)
	if order != 1 {
		t.Fatalf("failed move changed the saved order to %d", order)
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

func TestSourceGroupSyncImportsSourceLogoWithoutOverwritingManualChoice(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', 'https://example.test/list.m3u', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (20, 'Provider News', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES (100, 'alpha.tv', 'Provider Alpha', 'https://cdn.example.test/Alpha Logo.svg?size=512', 'Alpha', 20, 1)`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL)`)
	db.MustExec(`INSERT INTO logo VALUES (0, 'xivi_channel'), (42, 'manual-alpha')`)

	query := NewExperienceQueries(db)
	var importedURL, importedName string
	query.storeSourceLogo = func(_ context.Context, rawURL, name string) error {
		importedURL, importedName = rawURL, name
		return nil
	}
	if err := query.SetSourceGroupLink(context.Background(), 10, models.SourceGroupLinkRequest{SourceGroupID: 20, FollowGroupName: true, FollowChannelNames: true}); err != nil {
		t.Fatalf("SetSourceGroupLink failed: %v", err)
	}
	if _, err := query.SyncSourceGroup(context.Background(), 10); err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}
	var channel struct {
		ID       int64  `db:"id"`
		LogoID   int64  `db:"logoid"`
		LogoName string `db:"logo_name"`
	}
	if err := db.Get(&channel, `SELECT tc.id, tc.logoid, l.name AS logo_name FROM templatechannel tc JOIN logo l ON l.id = tc.logoid`); err != nil {
		t.Fatalf("imported channel logo was unavailable: %v", err)
	}
	wantName := logoassets.SourceLogoName(importedURL)
	if importedURL != "https://cdn.example.test/Alpha Logo.svg?size=512" || importedName != wantName || channel.LogoID == 0 || channel.LogoName != wantName {
		t.Fatalf("source logo was not adopted: channel=%#v url=%q name=%q want=%q", channel, importedURL, importedName, wantName)
	}

	db.MustExec(`UPDATE templatechannel SET logoid = 42 WHERE id = ?`, channel.ID)
	db.MustExec(`UPDATE playlistchannel SET tvg_logo = 'https://cdn.example.test/replacement.png' WHERE id = 100`)
	if _, err := query.SyncSourceGroup(context.Background(), 10); err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	var logoID int64
	db.Get(&logoID, `SELECT logoid FROM templatechannel WHERE id = ?`, channel.ID)
	if logoID != 42 {
		t.Fatalf("source sync replaced the manually selected logo with %d", logoID)
	}
}

func TestSourceGroupSyncKeepsDefaultWhenSourceLogoDownloadFails(t *testing.T) {
	db := newExperienceTestDB(t)
	db.MustExec(`INSERT INTO playlist VALUES (1, 'Provider', 'https://example.test/list.m3u', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	db.MustExec(`INSERT INTO playlistgroup VALUES (20, 'Provider News', 1, 1)`)
	db.MustExec(`INSERT INTO playlistchannel VALUES (100, 'alpha.tv', 'Provider Alpha', 'https://cdn.example.test/missing.png', 'Alpha', 20, 1)`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL)`)
	db.MustExec(`INSERT INTO logo VALUES (0, 'xivi_channel')`)

	query := NewExperienceQueries(db)
	query.storeSourceLogo = func(context.Context, string, string) error { return errors.New("download failed") }
	if err := query.SetSourceGroupLink(context.Background(), 10, models.SourceGroupLinkRequest{SourceGroupID: 20, FollowGroupName: true, FollowChannelNames: true}); err != nil {
		t.Fatalf("SetSourceGroupLink failed: %v", err)
	}
	if _, err := query.SyncSourceGroup(context.Background(), 10); err != nil {
		t.Fatalf("a broken source logo should not fail membership sync: %v", err)
	}
	var logoID, imported int64
	db.Get(&logoID, `SELECT logoid FROM templatechannel`)
	db.Get(&imported, `SELECT COUNT(*) FROM logo WHERE id != 0`)
	if logoID != 0 || imported != 0 {
		t.Fatalf("failed logo import left partial state: logo_id=%d imported=%d", logoID, imported)
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
