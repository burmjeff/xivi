package database

import (
	"fmt"
	"os"
	"xivi/backend/app/queries"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	log "github.com/sirupsen/logrus"
)

// Queries struct for collect all app queries.
type Queries struct {
	*queries.PlaylistQueries // load queries from Playlist model
	*queries.TemplateQueries // load queries from Template model
	*queries.EpgQueries      // load queries from Epg model
	*queries.VectorQueries   // load queries from Vector model
}

// OpenDBConnection func for opening database connection.
func OpenDBConnection() (*Queries, error) {
	// Define a new PostgreSQL connection.
	db, err := getDB()
	if err != nil {
		return nil, err
	}
	return &Queries{
		// Set queries from models:
		PlaylistQueries: &queries.PlaylistQueries{DB: db}, // from Playlist model
		TemplateQueries: &queries.TemplateQueries{DB: db}, // from Template model
		EpgQueries:      &queries.EpgQueries{DB: db},      // from Epg model
		VectorQueries:   &queries.VectorQueries{DB: db},   // from Vector model
	}, nil
}

func getDB() (*sqlx.DB, error) {
	return sqlx.Open("sqlite3", fmt.Sprintf("%s/xivi.db?parseTime=true", os.Getenv("CONFIG_PATH")))
}

func InitDB() error {
	db, err := getDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	driver, err := sqlite3.WithInstance(db.DB, &sqlite3.Config{})
	if err != nil {
		log.Fatalf("Failed to create database driver: %v", err)
	}

	fsrc, err := (&file.File{}).Open("file://./backend/platform/database/migrations")
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance(
		"file",
		fsrc,
		"xivi",
		driver)
	if err != nil {
		log.Fatalf("Failed to create migration: %v", err)
	}

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Println("Migrations ran successfully")
	return nil
}
