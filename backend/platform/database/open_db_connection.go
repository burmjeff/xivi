package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"xivi/backend/app/queries"
	"xivi/backend/platform/settings"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	gosqlite "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
)

var (
	Db         *Queries
	dbInstance *sqlx.DB     // Store the database instance for reinitialization
	dbMutex    sync.RWMutex // Mutex to protect database operations during reinitialization
	driverOnce sync.Once
)

// Queries struct for collect all app queries.
type Queries struct {
	*queries.PlaylistQueries   // load queries from Playlist model
	*queries.TemplateQueries   // load queries from Template model
	*queries.EpgQueries        // load queries from Epg model
	*queries.VectorQueries     // load queries from Vector model
	*queries.LogoQueries       // load queries from Logo model
	*queries.StreamQueries     // load queries from Stream model
	*queries.CleanupQueries    // load Cleanup queries
	*queries.ExperienceQueries // load queries for the v2 product interface
	*queries.SecurityQueries   // authentication, authorization, and media credentials
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
	for _, path := range databaseFiles() {
		if err := os.Chmod(path, 0600); err != nil && !os.IsNotExist(err) {
			_ = db.Close()
			return nil, fmt.Errorf("secure database file: %w", err)
		}
	}

	if err := InitDB(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database migration failed: %w", err)
	}
	if err := encryptExistingProviderURLs(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("provider URL encryption migration failed: %w", err)
	}

	return &Queries{
		// Set queries from models using the new constructors:
		PlaylistQueries:   queries.NewPlaylistQueries(db), // from Playlist model
		TemplateQueries:   queries.NewTemplateQueries(db), // from Template model
		EpgQueries:        queries.NewEpgQueries(db),      // from Epg model
		VectorQueries:     queries.NewVectorQueries(db),   // from Vector model
		LogoQueries:       queries.NewLogoQueries(db),     // from Logo model
		StreamQueries:     queries.NewStreamQueries(db),   // from Stream model
		CleanupQueries:    queries.NewCleanupQueries(db),  // from Cleanup model
		ExperienceQueries: queries.NewExperienceQueries(db),
		SecurityQueries:   queries.NewSecurityQueries(db),
	}, nil
}

func databaseFiles() []string {
	base := filepath.Join(settings.CONFIG_PATH, "xivi.db")
	return []string{base, base + "-wal", base + "-shm"}
}

func getDB() (*sqlx.DB, error) {
	driverOnce.Do(func() {
		sql.Register("sqlite3_xivi", &gosqlite.SQLiteDriver{ConnectHook: func(connection *gosqlite.SQLiteConn) error {
			if _, err := connection.Exec("PRAGMA trusted_schema = OFF;", nil); err != nil {
				return err
			}
			_, err := connection.Exec("PRAGMA secure_delete = FAST;", nil)
			return err
		}})
	})
	// Begin explicit transactions with a write reservation. Several Studio
	// operations validate their target before mutating it; deferred transactions
	// can otherwise fail immediately while upgrading that read transaction to a
	// writer, even when a busy timeout is configured.
	connStr := fmt.Sprintf("%s/xivi.db?_journal_mode=WAL&_foreign_keys=on&_shared_cache=true&_recursive_triggers=false&_secure_delete=FAST&_busy_timeout=10000&_txlock=immediate", settings.CONFIG_PATH)

	db, err := sqlx.Open("sqlite3_xivi", connStr)
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
		"PRAGMA secure_delete = FAST;",           // Scrub sensitive rows without a full-page rewrite
		"PRAGMA trusted_schema = OFF;",           // Do not trust application-defined schema functions
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

	log.Info().Msg("SQLite connection optimized for high concurrency and performance")
}

func InitDB(db *sqlx.DB) error {
	driver, err := migratesqlite.WithInstance(db.DB, &migratesqlite.Config{})
	if err != nil {
		return fmt.Errorf("create database migration driver: %w", err)
	}

	fsrc, err := (&file.File{}).Open("file://./backend/platform/database/migrations")
	if err != nil {
		fsrc, err = (&file.File{}).Open("file://./migrations")
	}
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
		return fmt.Errorf("create database migration: %w", err)
	}
	// migrate.Close also closes the sql.DB supplied through WithInstance. Xivi
	// still needs that shared pool for the encryption pass and the running
	// application, so close only the file source owned by this function.
	defer func() { _ = fsrc.Close() }()

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run database migrations: %w", err)
	}

	log.Print("Migrations ran successfully")
	return nil
}

// CloseDBConnection closes the database connection.
func CloseDBConnection() error {
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
