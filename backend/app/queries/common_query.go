package queries

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// Common errors
var (
	ErrNotFound   = errors.New("record not found")
	ErrDBInternal = errors.New("database internal error")
)

// BaseQueries provides common functionality for all query structs
type BaseQueries struct {
	*sqlx.DB
	stmtCache map[string]*sqlx.Stmt
}

// NewBaseQueries creates a new BaseQueries instance
func NewBaseQueries(db *sqlx.DB) BaseQueries {
	return BaseQueries{
		DB:        db,
		stmtCache: make(map[string]*sqlx.Stmt),
	}
}

// GetPreparedStmt returns a cached prepared statement or creates a new one
func (q *BaseQueries) GetPreparedStmt(query string) (*sqlx.Stmt, error) {
	if stmt, ok := q.stmtCache[query]; ok {
		return stmt, nil
	}

	stmt, err := q.Preparex(query)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare statement: %w", err)
	}

	q.stmtCache[query] = stmt
	return stmt, nil
}

// WithTransaction executes a function within a transaction
func (q *BaseQueries) WithTransaction(fn func(*sqlx.Tx) error) error {
	tx, err := q.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			// Rollback on panic
			_ = tx.Rollback()
			panic(p) // Re-throw panic after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			log.Error().Err(rbErr).Msg("transaction rollback failed")
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// WithTransactionContext executes a function within a transaction with context timeout
func (q *BaseQueries) WithTransactionContext(ctx context.Context, fn func(context.Context, *sqlx.Tx) error) error {
	// Create a context with timeout for the transaction
	txCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tx, err := q.BeginTxx(txCtx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			// Rollback on panic
			_ = tx.Rollback()
			panic(p) // Re-throw panic after rollback
		}
	}()

	if err := fn(txCtx, tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			log.Error().Err(rbErr).Msg("transaction rollback failed")
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// WrapExec wraps the Exec function with error handling and logging
func (q *BaseQueries) WrapExec(query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	res, err := q.Exec(query, args...)
	duration := time.Since(start)

	if duration > 100*time.Millisecond {
		log.Debug().Float64("duration_ms", float64(duration.Milliseconds())).Msg("slow database operation detected")
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Msg("database operation failed")
		return nil, fmt.Errorf("%w: %v", ErrDBInternal, err)
	}

	return res, nil
}

// WithContext executes a query with context for timeout/cancellation
func (q *BaseQueries) WithContext(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
