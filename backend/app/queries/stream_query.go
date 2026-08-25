package queries

import (
	"context"
	"sync/atomic"
	"time"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

var streamEventRetentionCounter atomic.Uint64

// SQL query constants
const (
	selectChannelsByUuidQuery = `
		SELECT channelurl.*,
			COALESCE(playlistchannel.title, '') AS source_name,
			COALESCE(playlistgroup.name, '') AS group_name,
			COALESCE(playlist.name, '') AS playlist_name,
			COALESCE(playlist.id, 0) AS playlist_id,
			COALESCE(playlist.connection_limit, 1) AS connection_limit
		FROM templatechannelitem
		JOIN templatechannel ON templatechannel.id = templatechannelitem.channel_id
		JOIN playlistchannel ON playlistchannel.id = templatechannelitem.playlist_channel_id
		JOIN playlistgroup ON playlistgroup.id = playlistchannel.group_id
		LEFT JOIN playlist ON playlist.id = playlistgroup.playlist_id
		JOIN channelurl ON channelurl.channel_id = templatechannelitem.playlist_channel_id
		WHERE templatechannel.uuid = ?
			AND playlistchannel.enabled = true
			AND playlistgroup.enabled = true
		ORDER BY templatechannelitem.orderr ASC, templatechannelitem.id ASC,
			channelurl.orderr ASC, channelurl.id ASC
		-- FORCE INDEX (idx_templatechannel_uuid) -- Hint to use index on uuid
		LIMIT 1000 -- Safety limit
	`
)

const selectStreamIdentityQuery = `
	SELECT templatechannel.id, templatechannel.name, templatechannel.tvgid,
		COALESCE(logo.name, '') AS logo_name
	FROM templatechannel
	LEFT JOIN logo ON logo.id = templatechannel.logoid
	WHERE templatechannel.uuid = ?
	LIMIT 1
`

type StreamQueries struct {
	BaseQueries
}

// NewStreamQueries creates a new StreamQueries instance
func NewStreamQueries(db *sqlx.DB) *StreamQueries {
	return &StreamQueries{
		BaseQueries: NewBaseQueries(db),
	}
}

func (q *StreamQueries) GetChannelsbyUuid(ctx context.Context, uuid string) (*[]models.ChannelUrl, error) {
	channels := &[]models.ChannelUrl{}

	err := q.WithContext(ctx, func(ctx context.Context) error {
		stmt, err := q.GetPreparedStmt(selectChannelsByUuidQuery)
		if err != nil {
			return err
		}

		start := time.Now()
		err = stmt.SelectContext(ctx, channels, uuid)
		duration := time.Since(start)

		if duration > 100*time.Millisecond {
			log.Debug().
				Str("uuid", uuid).
				Float64("duration_ms", float64(duration.Milliseconds())).
				Msg("slow GetChannelsbyUuid query")
		}

		return err
	})

	if err != nil {
		log.Error().Err(err).Str("uuid", uuid).Msg("Error fetching channels by UUID")
		return nil, err
	}

	return channels, nil
}

func (q *StreamQueries) GetStreamIdentity(ctx context.Context, uuid string) (*models.StreamIdentity, error) {
	identity := &models.StreamIdentity{}
	err := q.WithContext(ctx, func(ctx context.Context) error {
		return q.DB.GetContext(ctx, identity, selectStreamIdentityQuery, uuid)
	})
	return identity, err
}

func (q *StreamQueries) UpsertStreamSession(ctx context.Context, session models.StreamSessionHistory) error {
	_, err := q.DB.NamedExecContext(ctx, `
		INSERT INTO stream_session_history (
			incident_id, stream_id, channel_id, channel_name, state, source_count, reconnects, bytes_ingested,
			bytes_delivered, slow_client_drops, started_at, first_media_at, ended_at,
			end_reason, error_code, last_error
		) VALUES (
			:incident_id, :stream_id, :channel_id, :channel_name, :state, :source_count, :reconnects, :bytes_ingested,
			:bytes_delivered, :slow_client_drops, :started_at, :first_media_at, :ended_at,
			:end_reason, :error_code, :last_error
		)
		ON CONFLICT(incident_id) DO UPDATE SET
			channel_id = COALESCE(stream_session_history.channel_id, excluded.channel_id),
			channel_name = CASE WHEN stream_session_history.channel_name = '' THEN excluded.channel_name ELSE stream_session_history.channel_name END,
			state = excluded.state, reconnects = excluded.reconnects,
			bytes_ingested = excluded.bytes_ingested, bytes_delivered = excluded.bytes_delivered,
			slow_client_drops = excluded.slow_client_drops,
			first_media_at = COALESCE(stream_session_history.first_media_at, excluded.first_media_at),
			ended_at = excluded.ended_at, end_reason = excluded.end_reason,
			error_code = excluded.error_code, last_error = excluded.last_error
	`, session)
	return err
}

func (q *StreamQueries) UpsertStreamConnection(ctx context.Context, connection models.StreamConnectionHistory) error {
	_, err := q.DB.NamedExecContext(ctx, `
		INSERT INTO stream_connection_history (
			id, incident_id, stream_id, protocol, remote_ip, method, user_agent,
			started_at, last_seen_at, ended_at, bytes_delivered, end_reason
		) VALUES (
			:id, :incident_id, :stream_id, :protocol, :remote_ip, :method, :user_agent,
			:started_at, :last_seen_at, :ended_at, :bytes_delivered, :end_reason
		)
		ON CONFLICT(id) DO UPDATE SET last_seen_at = excluded.last_seen_at,
			ended_at = excluded.ended_at, bytes_delivered = excluded.bytes_delivered,
			end_reason = excluded.end_reason
	`, connection)
	return err
}

func (q *StreamQueries) InsertStreamEvent(ctx context.Context, event models.StreamEvent) error {
	_, err := q.DB.NamedExecContext(ctx, `
		INSERT INTO stream_event (
			incident_id, connection_id, stream_id, severity, code, message,
			source_position, details, created_at
		) VALUES (
			:incident_id, :connection_id, :stream_id, substr(:severity, 1, 16),
			substr(:code, 1, 64), substr(:message, 1, 1024), :source_position,
			substr(:details, 1, 8192), :created_at
		)
	`, event)
	if err != nil {
		return err
	}
	// Amortize the ceiling check so event-heavy playback cannot grow without
	// bound while avoiding an indexed delete query for every telemetry row.
	if streamEventRetentionCounter.Add(1)%100 == 0 {
		if _, cleanupErr := q.DB.ExecContext(ctx, `DELETE FROM stream_event WHERE id IN (
			SELECT id FROM stream_event WHERE incident_id = ?
			ORDER BY created_at DESC, id DESC LIMIT -1 OFFSET 1000
		)`, event.IncidentID); cleanupErr != nil {
			log.Warn().Err(cleanupErr).Str("incident_id", event.IncidentID).Msg("Deferred stream event ceiling cleanup failed")
		}
	}
	return nil
}

func (q *StreamQueries) ListStreamSessions(ctx context.Context, limit int) ([]models.StreamSessionHistory, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	items := []models.StreamSessionHistory{}
	err := q.DB.SelectContext(ctx, &items, `SELECT * FROM stream_session_history ORDER BY started_at DESC LIMIT ?`, limit)
	return items, err
}

func (q *StreamQueries) GetLatestStreamSession(ctx context.Context, streamID string) (*models.StreamSessionHistory, error) {
	item := &models.StreamSessionHistory{}
	err := q.DB.GetContext(ctx, item, `SELECT * FROM stream_session_history WHERE stream_id = ? ORDER BY started_at DESC LIMIT 1`, streamID)
	return item, err
}

func (q *StreamQueries) ListStreamConnections(ctx context.Context, incidentID string) ([]models.StreamConnectionHistory, error) {
	items := []models.StreamConnectionHistory{}
	err := q.DB.SelectContext(ctx, &items, `SELECT * FROM stream_connection_history WHERE incident_id = ? ORDER BY started_at DESC`, incidentID)
	return items, err
}

func (q *StreamQueries) ListStreamEvents(ctx context.Context, incidentID string, limit int) ([]models.StreamEvent, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	items := []models.StreamEvent{}
	query := `SELECT * FROM stream_event`
	args := []any{}
	if incidentID != "" {
		query += ` WHERE incident_id = ?`
		args = append(args, incidentID)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	err := q.DB.SelectContext(ctx, &items, query, args...)
	return items, err
}

func (q *StreamQueries) ReconcileStreamSessions(ctx context.Context) (int64, error) {
	result, err := q.DB.ExecContext(ctx, `
		UPDATE stream_session_history SET state = 'stopped', ended_at = CURRENT_TIMESTAMP,
			end_reason = 'server_restart', error_code = CASE WHEN error_code = '' THEN 'server_restart' ELSE error_code END,
			last_error = CASE WHEN last_error = '' THEN 'The server stopped before this stream session closed cleanly.' ELSE last_error END
		WHERE ended_at IS NULL
	`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (q *StreamQueries) PruneStreamHistory(ctx context.Context, before time.Time) error {
	_, err := q.DB.ExecContext(ctx, `DELETE FROM stream_session_history
		WHERE ended_at IS NOT NULL AND datetime(ended_at) < datetime(?)`, before)
	return err
}
