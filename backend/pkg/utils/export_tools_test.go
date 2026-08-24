package utils

import (
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
