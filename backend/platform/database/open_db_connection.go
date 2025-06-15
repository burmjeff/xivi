package database

import (
	"fmt"
	"sync"
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

var (
	Db                 *Queries
	dbInstance         *sqlx.DB     // Store the database instance for reinitialization
	dbMutex            sync.RWMutex // Mutex to protect database operations during reinitialization
	optimizationTicker *time.Ticker // Ticker for periodic optimization
	optimizationDone   chan bool    // Channel to signal optimization goroutine to stop
)

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

	// Store the database instance for later reinitialization
	dbInstance = db

	// Apply additional PRAGMA optimizations
	optimizeDBConnection(db)

	InitDB(db)

	// Initialize optimization control structures
	optimizationDone = make(chan bool, 1)

	// Start periodic database optimization in background
	go startPeriodicDatabaseOptimization()

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
	connStr := fmt.Sprintf("%s/xivi.db?_journal_mode=WAL&_foreign_keys=on&_shared_cache=true&_recursive_triggers=false", settings.CONFIG_PATH)

	db, err := sqlx.Open("sqlite3", connStr)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(0)
	db.SetMaxIdleConns(5)                   // Keep some idle connections for performance
	db.SetConnMaxLifetime(30 * time.Minute) // Close connections after 30 minutes to prevent leaks
	db.SetConnMaxIdleTime(5 * time.Minute)  // Close idle connections after 5 minutes

	return db, nil
}

func optimizeDBConnection(db *sqlx.DB) {
	pragmas := []string{
		"PRAGMA temp_store = MEMORY;",            // Store temporary tables in memory
		"PRAGMA page_size = 65536;",              // Increased page size for better performance
		"PRAGMA locking_mode = NORMAL;",          // Normal locking mode for better concurrency
		"PRAGMA analysis_limit = 10000;",         // Increased limit for query analysis
		"PRAGMA wal_autocheckpoint = 4000;",      // Increase WAL checkpoint interval
		"PRAGMA secure_delete = OFF;",            // Disable secure delete for performance
		"PRAGMA query_only = OFF;",               // Allow write operations
		"PRAGMA mmap_size = 1073741824;",         // Set memory-mapped I/O size (1GB)
		"PRAGMA cache_size = -1048576;",          // Set cache size (1GB)
		"PRAGMA journal_size_limit = 134217728;", // Limit WAL journal size to 128MB
		"PRAGMA synchronous = NORMAL;",           // Normal synchronous mode for better performance
		"PRAGMA case_sensitive_like = OFF;",      // Case-insensitive LIKE for better performance
		"PRAGMA auto_vacuum = INCREMENTAL;",      // Incremental vacuum for better performance
		"PRAGMA busy_timeout = 60000;",           // Set busy timeout to 60 seconds
		"PRAGMA timeout = 60000;",                // Set timeout to 60 seconds
	}

	// Execute pragmas individually to avoid transaction deadlocks
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			log.Warn().Msgf("Failed to execute PRAGMA: %s, error: %v", pragma, err)
		}
	}

	// Run optimization commands separately
	if _, err := db.Exec("PRAGMA optimize;"); err != nil {
		log.Warn().Msgf("Failed to execute PRAGMA optimize: %v", err)
	}

	// Perform a passive checkpoint
	if _, err := db.Exec("PRAGMA wal_checkpoint(PASSIVE);"); err != nil {
		log.Warn().Msgf("Failed to execute WAL checkpoint: %v", err)
	}

	go func() {
		if _, err := db.Exec("ANALYZE;"); err != nil {
			log.Warn().Msgf("Failed to execute ANALYZE: %v", err)
		}
	}()

	log.Info().Msg("SQLite connection optimized for high concurrency and performance")
}

// Periodically optimize the database to maintain performance
func startPeriodicDatabaseOptimization() {
	// Run optimization every 60 minutes
	optimizationTicker = time.NewTicker(60 * time.Minute)
	defer optimizationTicker.Stop()

	for {
		select {
		case <-optimizationTicker.C:
			log.Info().Msg("Running periodic database optimization")

			// Use read lock to safely access the database instance
			dbMutex.RLock()
			currentDB := dbInstance
			dbMutex.RUnlock()

			// Check if database instance is valid
			if currentDB == nil {
				log.Warn().Msg("Skipping periodic optimization: database instance is nil")
				continue
			}

			// Test connection before running optimization
			if err := currentDB.Ping(); err != nil {
				log.Error().Err(err).Msg("Skipping periodic optimization: database connection is not healthy")
				continue
			}

			// Run optimization commands
			go func() {
				// Run PRAGMA optimize to optimize the database
				if _, err := currentDB.Exec("PRAGMA optimize;"); err != nil {
					log.Error().Err(err).Msg("Failed to execute periodic PRAGMA optimize")
				}
			}()

			go func() {
				// Run incremental VACUUM to reclaim space
				if _, err := currentDB.Exec("PRAGMA incremental_vacuum;"); err != nil {
					log.Error().Err(err).Msg("Failed to execute periodic incremental VACUUM")
				}
			}()

			// Run ANALYZE
			go func() {
				if _, err := currentDB.Exec("ANALYZE;"); err != nil {
					log.Error().Err(err).Msg("Failed to execute periodic ANALYZE")
				}
			}()

			log.Info().Msg("Periodic database optimization started (running in background)")
		case <-optimizationDone:
			log.Info().Msg("Stopping periodic database optimization")
			return
		}
	}
}

func InitDB(db *sqlx.DB) error {
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

// ReinitializePreparedStatements reinitializes all prepared statements in the database
func ReinitializePreparedStatements() {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if dbInstance == nil {
		log.Error().Msg("Cannot reinitialize prepared statements: database instance is nil")
		return
	}

	log.Info().Msg("Reinitializing all prepared statements")

	// Reinitialize vector queries prepared statements
	if Db != nil && Db.VectorQueries != nil {
		Db.VectorQueries = queries.NewVectorQueries(dbInstance)
		log.Info().Msg("Vector queries prepared statements reinitialized")
	}

	// Force a connection reset by closing and reopening connections
	if err := dbInstance.Close(); err != nil {
		log.Error().Err(err).Msg("Error closing database connections")
		return
	}

	// Reopen the database connection
	newDb, err := getDB()
	if err != nil {
		log.Error().Err(err).Msg("Error reopening database connection")
		return
	}

	// Apply optimizations to the new connection
	optimizeDBConnection(newDb)

	// Update the global instance
	dbInstance = newDb

	// Recreate all query instances
	Db = &Queries{
		PlaylistQueries: queries.NewPlaylistQueries(newDb),
		TemplateQueries: queries.NewTemplateQueries(newDb),
		EpgQueries:      queries.NewEpgQueries(newDb),
		VectorQueries:   queries.NewVectorQueries(newDb),
		LogoQueries:     queries.NewLogoQueries(newDb),
		StreamQueries:   queries.NewStreamQueries(newDb),
		CleanupQueries:  queries.NewCleanupQueries(newDb),
	}

	log.Info().Msg("All database prepared statements successfully reinitialized")
}

// StopPeriodicOptimization stops the periodic database optimization goroutine
func StopPeriodicOptimization() {
	if optimizationDone != nil {
		select {
		case optimizationDone <- true:
			log.Info().Msg("Sent stop signal to periodic optimization goroutine")
		default:
			log.Warn().Msg("Optimization goroutine stop signal channel is full or closed")
		}
	}
}

// CloseDBConnection closes the database connection and stops optimization
func CloseDBConnection() error {
	// Stop the periodic optimization first
	StopPeriodicOptimization()

	// Acquire write lock
	dbMutex.Lock()
	defer dbMutex.Unlock()

	if dbInstance != nil {
		err := dbInstance.Close()
		dbInstance = nil
		Db = nil
		return err
	}
	return nil
}
