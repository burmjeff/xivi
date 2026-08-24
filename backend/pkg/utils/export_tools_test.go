package utils

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"xivi/backend/app/models"
	appqueries "xivi/backend/app/queries"
	"xivi/backend/platform/database"

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
