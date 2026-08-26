package utils

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xivi/backend/app/models"
	appqueries "xivi/backend/app/queries"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func useExportTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db := sqlx.MustOpen("sqlite3", ":memory:")
	original := database.Db
	database.Db = &database.Queries{
		TemplateQueries: appqueries.NewTemplateQueries(db),
		EpgQueries:      appqueries.NewEpgQueries(db),
		LogoQueries:     appqueries.NewLogoQueries(db),
	}
	t.Cleanup(func() {
		database.Db = original
		_ = db.Close()
	})
	return db
}

func TestCreateM3uReturnsDatabaseError(t *testing.T) {
	useExportTestDB(t)

	err := NewM3uTools().CreateM3u(models.Template{ID: 1, Name: "test"})
	if err == nil {
		t.Fatal("expected M3U generation to return its database error")
	}
	if !strings.Contains(err.Error(), "could not build the M3U export") {
		t.Fatalf("expected an actionable M3U error, got %q", err)
	}
}

func TestCreateEpgXMLReturnsDatabaseError(t *testing.T) {
	useExportTestDB(t)

	err := CreateEpgXML(models.Template{ID: 1, Name: "test"})
	if err == nil {
		t.Fatal("expected XMLTV generation to return its database error")
	}
	if !strings.Contains(err.Error(), "could not load lineup channels for XMLTV") {
		t.Fatalf("expected an actionable XMLTV error, got %q", err)
	}
}

func TestCreateM3uRejectsLineupWithoutPlayableChannels(t *testing.T) {
	db := useExportTestDB(t)
	db.MustExec(`CREATE TABLE templategroup (id INTEGER PRIMARY KEY, name TEXT, dynamic BOOLEAN, dynamicgroup INTEGER)`)
	db.MustExec(`CREATE TABLE template_group_item (template_id INTEGER, group_id INTEGER, orderr INTEGER)`)

	err := NewM3uTools().CreateM3u(models.Template{ID: 1, Name: "empty"})
	if err == nil {
		t.Fatal("expected a lineup without playable source variants to fail publication")
	}
	if !strings.Contains(err.Error(), "no playable channels with source variants") {
		t.Fatalf("expected the missing-source reason, got %q", err)
	}
}

func TestM3uUsesSavedGroupAndChannelOrder(t *testing.T) {
	db := useExportTestDB(t)
	db.MustExec(`CREATE TABLE templategroup (id INTEGER PRIMARY KEY, name TEXT, dynamic BOOLEAN, dynamicgroup INTEGER)`)
	db.MustExec(`CREATE TABLE template_group_item (template_id INTEGER, group_id INTEGER, orderr INTEGER)`)
	db.MustExec(`CREATE TABLE templatechannel (id INTEGER PRIMARY KEY, name TEXT, tvgid TEXT, logoid INTEGER, uuid TEXT)`)
	db.MustExec(`CREATE TABLE template_group_channel (group_id INTEGER, channel_id INTEGER, orderr INTEGER)`)
	db.MustExec(`CREATE TABLE templatechannelitem (id INTEGER PRIMARY KEY, channel_id INTEGER, playlist_channel_id INTEGER, orderr INTEGER, match_method TEXT, match_score REAL, runner_up_score REAL, matcher_version INTEGER, manual_locked BOOLEAN)`)
	db.MustExec(`CREATE TABLE logo (id INTEGER PRIMARY KEY, name TEXT)`)
	db.MustExec(`INSERT INTO logo VALUES (1, 'xivi_channel')`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL), (20, 'Sports', 0, NULL)`)
	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 20), (1, 20, 10)`)
	db.MustExec(`INSERT INTO templatechannel VALUES
		(101, 'News first', 'news.first', 1, 'news-first'),
		(102, 'News second', 'news.second', 1, 'news-second'),
		(201, 'Sports first', 'sports.first', 1, 'sports-first'),
		(202, 'Sports second', 'sports.second', 1, 'sports-second')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES
		(10, 101, 1), (10, 102, 2),
		(20, 201, 1), (20, 202, 2)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES
		(1, 101, 1001, 1, 'manual', 1, NULL, 2, 1),
		(2, 102, 1002, 1, 'manual', 1, NULL, 2, 1),
		(3, 201, 1003, 1, 'manual', 1, NULL, 2, 1),
		(4, 202, 1004, 1, 'manual', 1, NULL, 2, 1)`)

	tool := NewM3uTools()
	tool.template = models.Template{ID: 1, Name: "ordered"}
	tool.host = "xivi.test"
	tool.port = 3000
	buffer := &bytes.Buffer{}
	if err := tool.marshallInto(bufio.NewWriter(buffer)); err != nil {
		t.Fatalf("building ordered M3U failed: %v", err)
	}
	content := buffer.String()
	want := []string{"Sports first", "Sports second", "News first", "News second"}
	previous := -1
	for _, name := range want {
		index := strings.Index(content, ","+name+"\n")
		if index < 0 {
			t.Fatalf("M3U omitted %q:\n%s", name, content)
		}
		if index <= previous {
			t.Fatalf("M3U did not preserve group/channel order %v:\n%s", want, content)
		}
		previous = index
	}
}

func setupXMLTVExportTest(t *testing.T) *sqlx.DB {
	t.Helper()
	db := useExportTestDB(t)
	db.MustExec(`CREATE TABLE templategroup (id INTEGER PRIMARY KEY, name TEXT, dynamic BOOLEAN, dynamicgroup INTEGER)`)
	db.MustExec(`CREATE TABLE template_group_item (template_id INTEGER, group_id INTEGER, orderr INTEGER)`)
	db.MustExec(`CREATE TABLE templatechannel (id INTEGER PRIMARY KEY, name TEXT, tvgid TEXT, logoid INTEGER, uuid TEXT)`)
	db.MustExec(`CREATE TABLE template_group_channel (group_id INTEGER, channel_id INTEGER, orderr INTEGER)`)
	db.MustExec(`CREATE TABLE templatechannelitem (id INTEGER PRIMARY KEY, channel_id INTEGER, playlist_channel_id INTEGER, orderr INTEGER)`)
	db.MustExec(`CREATE TABLE logo (id INTEGER PRIMARY KEY, name TEXT)`)
	db.MustExec(`CREATE TABLE epgprogramme (
		id INTEGER PRIMARY KEY, start DATETIME NOT NULL, stop DATETIME NOT NULL, channel TEXT NOT NULL,
		"title.value" TEXT, "title.lang" TEXT, subtitle TEXT, desc TEXT, categories TEXT,
		"icon.src" TEXT, directors TEXT, presenters TEXT, producers TEXT, actors TEXT,
		"episodenumber.system" TEXT, "episodenumber.value" TEXT,
		"rating.system" TEXT, "rating.value" TEXT, "video.quality" TEXT, date TEXT
	)`)
	db.MustExec(`INSERT INTO logo VALUES (1, 'xivi_channel')`)
	db.MustExec(`INSERT INTO templategroup VALUES (10, 'News', 0, NULL)`)
	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 1)`)
	db.MustExec(`INSERT INTO templatechannel VALUES
		(101, 'Unmapped News', NULL, 1, 'unmapped-news'),
		(102, 'Mapped News', 'mapped.news', 1, 'mapped-news'),
		(103, 'Mapped News Backup', 'mapped.news', 1, 'mapped-news-backup'),
		(104, 'Legacy Unmapped News', 'xivi', 1, 'legacy-unmapped-news')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES (10, 101, 1), (10, 102, 2), (10, 103, 3), (10, 104, 4)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES (1, 101, 1001, 1), (2, 102, 1002, 1), (3, 103, 1003, 1), (4, 104, 1004, 1)`)

	db.MustExec(`INSERT INTO epgprogramme (id, start, stop, channel, "title.value") VALUES (?, ?, ?, 'mapped.news', 'Expired')`,
		1, time.Now().Add(-48*time.Hour), time.Now().Add(-47*time.Hour))
	db.MustExec(`INSERT INTO epgprogramme (id, start, stop, channel, "title.value") VALUES (?, ?, ?, 'mapped.news', 'Scheduled news')`,
		2, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour))

	originalPath := settings.EPG_FILEPATH
	originalHost := settings.APP_SETTINGS.Server.Host
	originalPort := settings.APP_SETTINGS.Server.Port
	originalTZ := settings.APP_SETTINGS.Application.TZ
	settings.EPG_FILEPATH = t.TempDir()
	settings.APP_SETTINGS.Server.Host = "xivi.test"
	settings.APP_SETTINGS.Server.Port = 3000
	settings.APP_SETTINGS.Application.TZ = "UTC"
	t.Cleanup(func() {
		settings.EPG_FILEPATH = originalPath
		settings.APP_SETTINGS.Server.Host = originalHost
		settings.APP_SETTINGS.Server.Port = originalPort
		settings.APP_SETTINGS.Application.TZ = originalTZ
	})
	return db
}

func readXMLTVExport(t *testing.T, name string) models.EpgItem {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(settings.EPG_FILEPATH, name+".xml"))
	if err != nil {
		t.Fatal(err)
	}
	var item models.EpgItem
	if err := xml.Unmarshal(content, &item); err != nil {
		t.Fatal(err)
	}
	return item
}

func TestXMLTVExportUsesStableFallbackIDsAndDeduplicatesMappedChannels(t *testing.T) {
	db := setupXMLTVExportTest(t)
	lineup := models.Template{ID: 1, Name: "filled", FillMissingGuideSlots: true}
	if err := CreateEpgXML(lineup); err != nil {
		t.Fatal(err)
	}

	item := readXMLTVExport(t, lineup.Name)
	if len(item.Channels) != 3 {
		t.Fatalf("XMLTV channel count = %d, want 3 unique channel ids", len(item.Channels))
	}
	wantIDs := map[string]bool{
		"xivi.1.unmapped-news":        false,
		"xivi.1.legacy-unmapped-news": false,
		"mapped.news":                 false,
	}
	for _, channel := range item.Channels {
		if _, ok := wantIDs[channel.ChannelId]; !ok {
			t.Fatalf("unexpected XMLTV channel id %q", channel.ChannelId)
		}
		wantIDs[channel.ChannelId] = true
	}
	for id, found := range wantIDs {
		if !found {
			t.Fatalf("XMLTV omitted channel id %q", id)
		}
	}

	programmeCounts := map[string]int{}
	realProgrammeCount := 0
	for _, programme := range item.Programmes {
		programmeCounts[programme.Channel]++
		if programme.Title.Value == "Scheduled news" {
			realProgrammeCount++
			continue
		}
		if programme.Desc != xmlTVPlaceholderDescription {
			t.Fatalf("stale programme leaked into export: %#v", programme)
		}
	}
	if programmeCounts["xivi.1.unmapped-news"] == 0 || programmeCounts["xivi.1.legacy-unmapped-news"] == 0 || programmeCounts["mapped.news"] == 0 {
		t.Fatalf("placeholder coverage missing: %#v", programmeCounts)
	}
	if realProgrammeCount != 1 {
		t.Fatalf("real programme count = %d, want one deduplicated row", realProgrammeCount)
	}
	var storedProgrammeCount int
	if err := db.Get(&storedProgrammeCount, `SELECT COUNT(*) FROM epgprogramme`); err != nil {
		t.Fatal(err)
	}
	if storedProgrammeCount != 2 {
		t.Fatalf("XMLTV publication changed stored guide data: count = %d, want 2", storedProgrammeCount)
	}

	tool := NewM3uTools()
	tool.template = lineup
	tool.host = "xivi.test"
	tool.port = 3000
	buffer := &bytes.Buffer{}
	if err := tool.marshallInto(bufio.NewWriter(buffer)); err != nil {
		t.Fatal(err)
	}
	content := buffer.String()
	if !strings.Contains(content, `tvg-id="xivi.1.unmapped-news"`) {
		t.Fatalf("M3U did not use the stable fallback id:\n%s", content)
	}
	if strings.Contains(content, `tvg-id="xivi"`) {
		t.Fatalf("M3U retained the shared legacy fallback id:\n%s", content)
	}
}

func TestXMLTVExportCanLeaveGuideGapsBlank(t *testing.T) {
	setupXMLTVExportTest(t)
	lineup := models.Template{ID: 1, Name: "blank", FillMissingGuideSlots: false}
	if err := CreateEpgXML(lineup); err != nil {
		t.Fatal(err)
	}
	item := readXMLTVExport(t, lineup.Name)
	if len(item.Channels) != 3 {
		t.Fatalf("XMLTV channel count = %d, want 3", len(item.Channels))
	}
	if len(item.Programmes) != 1 || item.Programmes[0].Title.Value != "Scheduled news" {
		t.Fatalf("XMLTV should retain only the real programme with guide filling disabled: %#v", item.Programmes)
	}
}

func TestProgrammesForXMLTVFillsOnlyUncoveredIntervals(t *testing.T) {
	from := time.Date(2026, time.August, 25, 0, 0, 0, 0, time.UTC)
	to := from.Add(12 * time.Hour)
	real := []models.EpgProgramme{
		{Start: &models.Time{Time: from.Add(4 * time.Hour)}, Stop: &models.Time{Time: from.Add(6 * time.Hour)}, Channel: "source", Title: models.Title{Value: "Morning news"}},
		{Start: &models.Time{Time: from.Add(8 * time.Hour)}, Stop: &models.Time{Time: from.Add(10 * time.Hour)}, Channel: "source", Title: models.Title{Value: "Late news"}},
	}
	programmes := programmesForXMLTV("export.news", "News", real, from, to, true)
	if len(programmes) != 5 {
		t.Fatalf("programme count = %d, want 5", len(programmes))
	}
	cursor := from
	placeholderCount := 0
	for _, programme := range programmes {
		if !programme.Start.Equal(cursor) {
			t.Fatalf("gap or overlap before %s: next programme starts %s", cursor, programme.Start)
		}
		if programme.Channel != "export.news" {
			t.Fatalf("programme channel = %q, want export.news", programme.Channel)
		}
		if programme.Desc == xmlTVPlaceholderDescription {
			placeholderCount++
		}
		cursor = programme.Stop.Time
	}
	if !cursor.Equal(to) {
		t.Fatalf("coverage ended at %s, want %s", cursor, to)
	}
	if placeholderCount != 3 {
		t.Fatalf("placeholder count = %d, want 3", placeholderCount)
	}
}
