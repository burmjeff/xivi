package database

import (
	"fmt"
	"time"
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
	// Define a new SQLite connection.
	db, err := getDB()
	if err != nil {
		return nil, err
	}

	// Apply additional PRAGMA optimizations
	optimizeDBConnection(db)

	InitDB(db)

	// Start periodic database optimization in background
	go startPeriodicDatabaseOptimization(db)

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
	// - Larger cache size (1GB)
	// - Incremental vacuum
	// - Busy timeout to wait for locks (increased to 120000ms)
	// - Normal synchronous mode for better performance
	// - Memory-mapped I/O for better performance (increased to 1GB)
	// - Shared cache enabled for better concurrency
	// - Recursive triggers disabled for better performance
	connStr := fmt.Sprintf("%s/xivi.db?_journal_mode=WAL&_foreign_keys=on&_shared_cache=true&_recursive_triggers=false", settings.CONFIG_PATH)

	db, err := sqlx.Open("sqlite3", connStr)
	if err != nil {
		return nil, err
	}

	// Set connection pool settings
	db.SetMaxOpenConns(0)                   // Increased from 50 to 100
	db.SetConnMaxLifetime(0)                // Connections reused forever
	db.SetConnMaxIdleTime(10 * time.Minute) // Close idle connections after 10 minutes

	return db, nil
}

// Apply additional PRAGMA optimizations that can't be set via connection string
func optimizeDBConnection(db *sqlx.DB) {
	// Execute pragmas in a transaction for better performance
	tx, err := db.Begin()
	if err != nil {
		log.Error().Err(err).Msg("Failed to begin transaction for PRAGMA settings")
		return
	}

	pragmas := []string{
		"PRAGMA temp_store = MEMORY;", // Store temporary tables in memory
		"PRAGMA page_size = 65536;",   // Increased page size for better performance (from 32768 to 65536)
		"PRAGMA locking_mode = NORMAL;",
		"PRAGMA analysis_limit = 10000;",         // Increased limit for query analysis (from 5000 to 10000)
		"PRAGMA optimize;",                       // Run automatic optimization
		"PRAGMA wal_autocheckpoint = 4000;",      // Increase WAL checkpoint interval (from 2000 to 4000)
		"PRAGMA secure_delete = OFF;",            // Disable secure delete for performance
		"PRAGMA query_only = OFF;",               // Allow write operations
		"PRAGMA mmap_size = 1073741824;",         // Set memory-mapped I/O size (1GB)
		"PRAGMA cache_size = -1048576;",          // Set cache size (1GB)
		"PRAGMA journal_size_limit = 134217728;", // Limit WAL journal size to 128MB (from 64MB)
		"PRAGMA synchronous = NORMAL;",           // Normal synchronous mode for better performance
		"PRAGMA case_sensitive_like = OFF;",      // Case-insensitive LIKE for better performance
		"PRAGMA auto_vacuum = INCREMENTAL;",      // Incremental vacuum for better performance
		"PRAGMA busy_timeout = 300000;",          // Set busy timeout to 300 seconds (5 minutes)
		"PRAGMA timeout = 300000;",               // Set timeout to 300 seconds (5 minutes)
		"PRAGMA wal_checkpoint(PASSIVE);",        // Perform a passive checkpoint
	}

	for _, pragma := range pragmas {
		if _, err := tx.Exec(pragma); err != nil {
			log.Warn().Msgf("Failed to execute PRAGMA: %s, error: %v", pragma, err)
		}
	}

	// Execute ANALYZE to update statistics
	if _, err := tx.Exec("ANALYZE;"); err != nil {
		log.Warn().Msgf("Failed to execute ANALYZE: %v", err)
	}

	// Execute VACUUM to compact the database
	if _, err := tx.Exec("VACUUM;"); err != nil {
		log.Warn().Msgf("Failed to execute VACUUM: %v", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction for PRAGMA settings")
		_ = tx.Rollback()
		return
	}

	log.Info().Msg("SQLite connection optimized for high concurrency and performance")
}

// Periodically optimize the database to maintain performance
func startPeriodicDatabaseOptimization(db *sqlx.DB) {
	// Run optimization every 30 minutes
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Info().Msg("Running periodic database optimization")

			// Run ANALYZE to update statistics
			if _, err := db.Exec("ANALYZE;"); err != nil {
				log.Error().Err(err).Msg("Failed to execute periodic ANALYZE")
			}

			// Run PRAGMA optimize to optimize the database
			if _, err := db.Exec("PRAGMA optimize;"); err != nil {
				log.Error().Err(err).Msg("Failed to execute periodic PRAGMA optimize")
			}

			// Run incremental VACUUM to reclaim space
			if _, err := db.Exec("PRAGMA incremental_vacuum;"); err != nil {
				log.Error().Err(err).Msg("Failed to execute periodic incremental VACUUM")
			}

			log.Info().Msg("Periodic database optimization completed")
		}
	}
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
