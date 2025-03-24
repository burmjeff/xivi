package queries

import (
	"context"
	"time"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// SQL query constants
const (
	selectChannelsByUuidQuery = `
		SELECT channelurl.* FROM channelurl
		JOIN templatechannelitem ON channelurl.channel_id = templatechannelitem.playlist_channel_id
		JOIN templatechannel ON templatechannel.id = templatechannelitem.channel_id
		WHERE templatechannel.uuid = ? 
		ORDER BY channelurl.orderr ASC
		-- FORCE INDEX (idx_templatechannel_uuid) -- Hint to use index on uuid
		LIMIT 1000 -- Safety limit
	`
)

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
