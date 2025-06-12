package queries

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// SQL query constants
const (
	vacuumDBQuery = `PRAGMA incremental_vacuum;`
	// Clean empty playlist groups for the specified playlist (groups with no channels)
	cleanPlaylistGroupsQuery = `DELETE FROM playlistgroup
		WHERE playlist_id = ?
		AND id NOT IN (
			SELECT DISTINCT group_id FROM playlistchannel
			WHERE group_id IS NOT NULL
		)`
	// Clean playlist channels that belong to disabled groups in the specified playlist
	cleanPlaylistChannelsQuery = `DELETE FROM playlistchannel
		WHERE group_id IN (
			SELECT id FROM playlistgroup
			WHERE playlist_id = ? AND enabled = false
		)`
	cleanOldEpgProgrammesQuery = `DELETE FROM epgprogramme
		WHERE stop < datetime('now', 'localtime', '-1 day')`
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

// CleanOldEpgProgrammes deletes EPG programmes that are older than 1 day past their end time
func (q *CleanupQueries) CleanOldEpgProgrammes(ctx context.Context) error {
	return q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(cleanOldEpgProgrammesQuery)
		if err != nil {
			return err
		}

		res, err := stmt.ExecContext(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to clean old EPG programmes")
			return err
		}

		rowsAffected, err := res.RowsAffected()
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get rows affected count for EPG cleanup")
		} else {
			log.Info().Int64("rows_deleted", rowsAffected).Msg("Cleaned up old EPG programmes")
		}

		return nil
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
