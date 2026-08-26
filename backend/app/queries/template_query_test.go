package queries

import (
	"testing"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestTemplateVirtualTunerIsOptInAndPersists(t *testing.T) {
	db := sqlx.MustConnect("sqlite3", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE template (
		id INTEGER PRIMARY KEY, name TEXT NOT NULL,
		virtual_tuner_enabled BOOLEAN NOT NULL DEFAULT false,
		fill_missing_guide_slots BOOLEAN NOT NULL DEFAULT true
	)`)
	queries := NewTemplateQueries(db)
	id, err := queries.CreateTemplate(&models.Template{Name: "Local news"})
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := queries.GetTemplate(id)
	if err != nil {
		t.Fatal(err)
	}
	if lineup.VirtualTunerEnabled {
		t.Fatal("new lineup virtual tuner should be disabled")
	}
	if !lineup.FillMissingGuideSlots {
		t.Fatal("new lineup should fill missing XMLTV guide slots by default")
	}
	if err := queries.SetTemplateVirtualTunerEnabled(id, true); err != nil {
		t.Fatal(err)
	}
	lineup, err = queries.GetTemplate(id)
	if err != nil {
		t.Fatal(err)
	}
	if !lineup.VirtualTunerEnabled {
		t.Fatal("enabled virtual tuner state was not persisted")
	}
	if err := queries.SetTemplateFillMissingGuideSlots(id, false); err != nil {
		t.Fatal(err)
	}
	lineup, err = queries.GetTemplate(id)
	if err != nil {
		t.Fatal(err)
	}
	if lineup.FillMissingGuideSlots {
		t.Fatal("disabled guide fill state was not persisted")
	}
}

func TestGetPlayableTmplChannelsUsesGroupThenChannelOrder(t *testing.T) {
	db := sqlx.MustConnect("sqlite3", ":memory:")
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE templatechannel (
		id INTEGER PRIMARY KEY, name TEXT, tvgid TEXT, logoid INTEGER, uuid TEXT
	)`)
	db.MustExec(`CREATE TABLE template_group_channel (group_id INTEGER, channel_id INTEGER, orderr INTEGER)`)
	db.MustExec(`CREATE TABLE template_group_item (template_id INTEGER, group_id INTEGER, orderr INTEGER)`)
	db.MustExec(`CREATE TABLE templatechannelitem (id INTEGER PRIMARY KEY, channel_id INTEGER)`)

	db.MustExec(`INSERT INTO template_group_item VALUES (1, 10, 2), (1, 20, 1)`)
	db.MustExec(`INSERT INTO templatechannel VALUES
		(1, 'Later group', NULL, 0, 'one'),
		(2, 'Second in first group', NULL, 0, 'two'),
		(3, 'First in first group', NULL, 0, 'three'),
		(4, 'No source', NULL, 0, 'four')`)
	db.MustExec(`INSERT INTO template_group_channel VALUES
		(10, 1, 1), (20, 2, 2), (20, 3, 1), (20, 4, 3)`)
	db.MustExec(`INSERT INTO templatechannelitem VALUES (1, 1), (2, 2), (3, 3)`)

	channels, err := NewTemplateQueries(db).GetPlayableTmplChannels(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 3 {
		t.Fatalf("channel count = %d, want 3", len(channels))
	}
	want := []string{"First in first group", "Second in first group", "Later group"}
	for index := range want {
		if channels[index].Name != want[index] {
			t.Fatalf("channel %d = %q, want %q", index, channels[index].Name, want[index])
		}
	}
}
