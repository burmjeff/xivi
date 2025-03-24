package queries

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// SQL query constants
const (
	vacuumDBQuery            = `PRAGMA incremental_vacuum;`
	cleanPlaylistGroupsQuery = `DELETE FROM playlistgroup 
		WHERE ? NOT IN (SELECT playlist_id FROM playlistgroup)`
	cleanPlaylistChannelsQuery = `DELETE FROM playlistchannel
		WHERE ROWID IN (
			SELECT playlistchannel.ROWID FROM playlistchannel
			JOIN playlistgroup ON playlistchannel.group_id = playlistgroup.id
			WHERE ? NOT IN (SELECT playlist_id FROM playlistgroup)
			OR playlistgroup.enabled = false
		)`
)

type CleanupQueries struct {
	BaseQueries
}

// NewCleanupQueries creates a new CleanupQueries instance
func NewCleanupQueries(db *sqlx.DB) *CleanupQueries {
	return &CleanupQueries{
		BaseQueries: NewBaseQueries(db),
	}
}

func (q *CleanupQueries) VacuumDB(ctx context.Context) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(vacuumDBQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to vacuum database")
		}
		return err
	})
}

func (q *CleanupQueries) CleanPlaylistGroups(ctx context.Context, id int64) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(cleanPlaylistGroupsQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, id)
		if err != nil {
			log.Error().Err(err).Int64("playlist_id", id).Msg("Failed to clean playlist groups")
		}
		return err
	})
}

func (q *CleanupQueries) CleanPlaylistChannels(ctx context.Context, id int64) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(cleanPlaylistChannelsQuery)
		if err != nil {
			return err
		}

		_, err = stmt.ExecContext(ctx, id)
		if err != nil {
			log.Error().Err(err).Int64("playlist_id", id).Msg("Failed to clean playlist channels")
		}
		return err
	})
}

// CleanAll performs all cleanup operations in a single transaction
func (q *CleanupQueries) CleanAll(ctx context.Context, id int64) error {
	return q.WithTransaction(func(tx *sqlx.Tx) error {
		// Clean playlist groups
		cleanGrpsStmt, err := tx.PreparexContext(ctx, cleanPlaylistGroupsQuery)
		if err != nil {
			return err
		}
		defer cleanGrpsStmt.Close()

		if _, err := cleanGrpsStmt.ExecContext(ctx, id); err != nil {
			log.Error().Err(err).Int64("playlist_id", id).Msg("Failed to clean playlist groups in transaction")
			return err
		}

		// Clean playlist channels
		cleanChStmt, err := tx.PreparexContext(ctx, cleanPlaylistChannelsQuery)
		if err != nil {
			return err
		}
		defer cleanChStmt.Close()

		if _, err := cleanChStmt.ExecContext(ctx, id); err != nil {
			log.Error().Err(err).Int64("playlist_id", id).Msg("Failed to clean playlist channels in transaction")
			return err
		}

		// Run vacuum last, not in transaction as it's often auto-commit in SQLite
		return nil
	})
}
