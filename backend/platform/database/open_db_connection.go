package database

import (
	"fmt"
	"xivi/backend/app/queries"
	"xivi/backend/platform/settings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
)

var Db *Queries

// Queries struct for collect all app queries.
type Queries struct {
	*queries.PlaylistQueries // load queries from Playlist model
	*queries.TemplateQueries // load queries from Template model
	*queries.EpgQueries      // load queries from Epg model
	*queries.VectorQueries   // load queries from Vector model
	*queries.LogoQueries     // load queries from Logo model
	*queries.StreamQueries   // load queries from Stream model
}

// OpenDBConnection func for opening database connection.
func OpenDBConnection() (*Queries, error) {
	// Define a new PostgreSQL connection.
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	InitDB(db)

	return &Queries{
		// Set queries from models:
		PlaylistQueries: &queries.PlaylistQueries{DB: db}, // from Playlist model
		TemplateQueries: &queries.TemplateQueries{DB: db}, // from Template model
		EpgQueries:      &queries.EpgQueries{DB: db},      // from Epg model
		VectorQueries:   &queries.VectorQueries{DB: db},   // from Vector model
		LogoQueries:     &queries.LogoQueries{DB: db},     // from Logo model
		StreamQueries:   &queries.StreamQueries{DB: db},   // from Stream model
	}, nil
}

func getDB() (*sqlx.DB, error) {
	return sqlx.Open("sqlite3", fmt.Sprintf("%s/xivi.db?_journal_mode=WAL", settings.CONFIG_PATH))
}

func InitDB(db *sqlx.DB) error {
	//db, err := getDB()
	//if err != nil {
	//	log.Fatal().Msgf("Failed to connect to database: %v", err)
	//}

	driver, err := sqlite3.WithInstance(db.DB, &sqlite3.Config{})
	if err != nil {
		log.Fatal().Msgf("Failed to create database driver: %v", err)
	}

	fsrc, err := (&file.File{}).Open("file://./backend/platform/database/migrations")
	if err != nil {
		fsrc, err = (&file.File{}).Open("file://./database_migrations")
		if err != nil {
			return err
		}
	}

	m, err := migrate.NewWithInstance(
		"file",
		fsrc,
		"xivi",
		driver)
	if err != nil {
		log.Fatal().Msgf("Failed to create migration: %v", err)
	}

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		log.Fatal().Msgf("Failed to run migrations: %v", err)
	}

	log.Print("Migrations ran successfully")
	return nil
}
