package queries

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// SQL query constants
const (
	vacuumDBQuery = `PRAGMA incremental_vacuum(2000);`
	// Groups will only be cleaned up manually or through explicit user actions
	cleanPlaylistGroupsQuery = `SELECT 1 WHERE 0` // No-op query that does nothing
	// Disabling a source group is reversible availability state. Its last good
	// channel snapshot is retained and stale enabled-group channels are handled
	// by CleanPlaylist after a successful refresh.
	cleanPlaylistChannelsQuery = `SELECT 1 WHERE ? >= 0`
)

const (
	defaultCleanupBatchSize = 500
	cleanupBatchPause       = 5 * time.Millisecond
)

type RetentionPolicy struct {
	OperationJobsBefore          time.Time
	StreamHistoryBefore          time.Time
	EPGProgrammesBefore          time.Time
	MaximumOperationJobs         int
	MaximumStreamSessions        int
	MaximumEventsPerSession      int
	MaximumConnectionsPerSession int
	BatchSize                    int
	PruneOrphanVectors           bool
}

type CleanupReport struct {
	OperationJobs     int64
	StreamSessions    int64
	StreamEvents      int64
	StreamConnections int64
	EPGProgrammes     int64
	EPGChannelItems   int64
	OrphanVectors     int64
}

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
	for _, statement := range []string{
		"PRAGMA wal_checkpoint(PASSIVE);",
		"PRAGMA optimize;",
		vacuumDBQuery,
	} {
		if _, err := q.ExecContext(ctx, statement); err != nil {
			log.Error().Err(err).Str("statement", statement).Msg("Failed to optimize database")
			return err
		}
	}
	return nil
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

// CleanOldEpgProgrammes deletes EPG programmes that are older than one day in
// bounded transactions so a large guide cannot monopolize SQLite's writer.
func (q *CleanupQueries) CleanOldEpgProgrammes(ctx context.Context) error {
	rows, err := q.pruneInBatches(ctx, `DELETE FROM epgprogramme WHERE id IN (
		SELECT id FROM epgprogramme WHERE datetime(stop) < datetime(?)
		ORDER BY datetime(stop), id LIMIT ?
	)`, databaseTimestamp(time.Now().Add(-24*time.Hour)), defaultCleanupBatchSize)
	if err != nil {
		log.Error().Err(err).Msg("Failed to clean old EPG programmes")
		return err
	}
	if rows > 0 {
		log.Info().Int64("rows_deleted", rows).Msg("Cleaned up old EPG programmes")
	}
	return nil
}

// PruneRetentionData applies age and count ceilings to every high-churn table.
// Each statement removes only a small batch and commits before continuing.
func (q *CleanupQueries) PruneRetentionData(ctx context.Context, policy RetentionPolicy) (CleanupReport, error) {
	policy = normalizedRetentionPolicy(policy)
	report := CleanupReport{}
	var cleanupErrors []error

	run := func(target *int64, query string, args ...any) {
		deleted, err := q.pruneInBatches(ctx, query, args...)
		*target += deleted
		if err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}

	jobCutoff := databaseTimestamp(policy.OperationJobsBefore)
	run(&report.OperationJobs, `DELETE FROM operation_job WHERE id IN (
		SELECT id FROM operation_job
		WHERE status IN ('succeeded', 'failed', 'cancelled')
			AND finished_at IS NOT NULL AND datetime(finished_at) < datetime(?)
		ORDER BY datetime(finished_at), id LIMIT ?
	)`, jobCutoff, policy.BatchSize)
	run(&report.OperationJobs, `DELETE FROM operation_job WHERE id IN (
		SELECT id FROM operation_job
		WHERE status IN ('succeeded', 'failed', 'cancelled')
			AND finished_at IS NULL AND datetime(updated_at) < datetime(?)
		ORDER BY datetime(updated_at), id LIMIT ?
	)`, jobCutoff, policy.BatchSize)
	run(&report.OperationJobs, `DELETE FROM operation_job WHERE id IN (
		SELECT id FROM operation_job
		WHERE status IN ('succeeded', 'failed', 'cancelled')
		ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?
	)`, policy.BatchSize, policy.MaximumOperationJobs)

	streamCutoff := databaseTimestamp(policy.StreamHistoryBefore)
	run(&report.StreamSessions, `DELETE FROM stream_session_history WHERE incident_id IN (
		SELECT incident_id FROM stream_session_history
		WHERE ended_at IS NOT NULL AND datetime(ended_at) < datetime(?)
		ORDER BY datetime(ended_at), incident_id LIMIT ?
	)`, streamCutoff, policy.BatchSize)
	run(&report.StreamSessions, `DELETE FROM stream_session_history WHERE incident_id IN (
		SELECT incident_id FROM stream_session_history
		WHERE ended_at IS NOT NULL
		ORDER BY ended_at DESC, started_at DESC, incident_id DESC LIMIT ? OFFSET ?
	)`, policy.BatchSize, policy.MaximumStreamSessions)

	run(&report.StreamEvents, `DELETE FROM stream_event WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (
				PARTITION BY incident_id ORDER BY created_at DESC, id DESC
			) AS retention_position
			FROM stream_event
		) WHERE retention_position > ?
		ORDER BY retention_position DESC LIMIT ?
	)`, policy.MaximumEventsPerSession, policy.BatchSize)
	run(&report.StreamConnections, `DELETE FROM stream_connection_history WHERE id IN (
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (
				PARTITION BY incident_id ORDER BY started_at DESC, id DESC
			) AS retention_position
			FROM stream_connection_history
			WHERE ended_at IS NOT NULL
		) WHERE retention_position > ?
		ORDER BY retention_position DESC LIMIT ?
	)`, policy.MaximumConnectionsPerSession, policy.BatchSize)

	run(&report.EPGProgrammes, `DELETE FROM epgprogramme WHERE id IN (
		SELECT id FROM epgprogramme WHERE datetime(stop) < datetime(?)
		ORDER BY datetime(stop), id LIMIT ?
	)`, databaseTimestamp(policy.EPGProgrammesBefore), policy.BatchSize)
	run(&report.EPGChannelItems, `DELETE FROM epgchannelitem WHERE id IN (
		SELECT item.id FROM epgchannelitem item
		LEFT JOIN epgprogramme programme ON programme.id = item.epg_programme_id
		LEFT JOIN epgchannel channel ON channel.id = item.epg_channel_id
		WHERE programme.id IS NULL OR channel.id IS NULL
		LIMIT ?
	)`, policy.BatchSize)

	if policy.PruneOrphanVectors {
		run(&report.OrphanVectors, `DELETE FROM channelvectors WHERE id IN (
			SELECT vector.id FROM channelvectors vector
			LEFT JOIN templatechannelvectors template_ref ON template_ref.vector_id = vector.id
			LEFT JOIN playlistchannelvectors playlist_ref ON playlist_ref.vector_id = vector.id
			WHERE template_ref.id IS NULL AND playlist_ref.id IS NULL
				AND NOT EXISTS (SELECT 1 FROM templatechannel channel WHERE channel.name = vector.name)
				AND NOT EXISTS (SELECT 1 FROM playlistchannel channel WHERE channel.title = vector.name)
			LIMIT ?
		)`, policy.BatchSize)
	}

	return report, errors.Join(cleanupErrors...)
}

func (q *CleanupQueries) pruneInBatches(ctx context.Context, query string, args ...any) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		result, err := q.ExecContext(ctx, query, args...)
		if err != nil {
			return total, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return total, err
		}
		total += rows
		if rows == 0 {
			return total, nil
		}
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		case <-time.After(cleanupBatchPause):
		}
	}
}

func normalizedRetentionPolicy(policy RetentionPolicy) RetentionPolicy {
	if policy.BatchSize < 1 || policy.BatchSize > 5000 {
		policy.BatchSize = defaultCleanupBatchSize
	}
	if policy.MaximumOperationJobs < 1 {
		policy.MaximumOperationJobs = 2000
	}
	if policy.MaximumStreamSessions < 1 {
		policy.MaximumStreamSessions = 2000
	}
	if policy.MaximumEventsPerSession < 1 {
		policy.MaximumEventsPerSession = 1000
	}
	if policy.MaximumConnectionsPerSession < 1 {
		policy.MaximumConnectionsPerSession = 1000
	}
	return policy
}

func databaseTimestamp(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05")
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
