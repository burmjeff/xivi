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
	*queries.CleanupQueries  // load Cleanup queries
}

// OpenDBConnection func for opening database connection.
func OpenDBConnection() (*Queries, error) {
	// Define a new PostgreSQL connection.
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	// Apply additional PRAGMA optimizations
	optimizeDBConnection(db)

	InitDB(db)

	return &Queries{
		// Set queries from models using the new constructors:
		PlaylistQueries: queries.NewPlaylistQueries(db), // from Playlist model
		TemplateQueries: queries.NewTemplateQueries(db), // from Template model
		EpgQueries:      queries.NewEpgQueries(db),      // from Epg model
		VectorQueries:   queries.NewVectorQueries(db),   // from Vector model
		LogoQueries:     queries.NewLogoQueries(db),     // from Logo model
		StreamQueries:   queries.NewStreamQueries(db),   // from Stream model
		CleanupQueries:  queries.NewCleanupQueries(db),  // from Cleanup model
	}, nil
}

func getDB() (*sqlx.DB, error) {
	// Enhanced connection string with optimized settings:
	// - WAL journal mode for better concurrency
	// - Foreign keys enforcement
	// - Larger cache size (256MB)
	// - Incremental vacuum
	// - Busy timeout to wait for locks (increased from 5000ms to 30000ms)
	// - Normal synchronous mode for better performance
	// - Memory-mapped I/O for better performance
	return sqlx.Open("sqlite3", fmt.Sprintf("%s/xivi.db?_journal_mode=WAL&_foreign_keys=on&_cache_size=-262144&_auto_vacuum=1&_busy_timeout=30000&_synchronous=NORMAL&_mmap_size=268435456", settings.CONFIG_PATH))
}

// Apply additional PRAGMA optimizations that can't be set via connection string
func optimizeDBConnection(db *sqlx.DB) {
	pragmas := []string{
		"PRAGMA temp_store = MEMORY;",   // Store temporary tables in memory
		"PRAGMA page_size = 8192;",      // Larger page size
		"PRAGMA locking_mode = NORMAL;", // Exclusive locking mode for better performance
		"PRAGMA threads = 16;",          // Use multiple threads
		"PRAGMA analysis_limit = 1000;", // Limit for query analysis
		"PRAGMA optimize;",              // Run automatic optimization
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			log.Warn().Msgf("Failed to execute PRAGMA: %s, error: %v", pragma, err)
		}
	}

	log.Info().Msg("SQLite connection optimized for performance")
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
