package app

import (
	"context"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/streaming"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

// streamObserver keeps database work off media and HTTP delivery goroutines.
// A full queue drops an audit update rather than ever stalling live video.
type streamObserver struct {
	queue chan streaming.Observation
	done  chan struct{}
}

func newStreamObserver() *streamObserver {
	observer := &streamObserver{queue: make(chan streaming.Observation, 2048), done: make(chan struct{})}
	go observer.run()
	return observer
}

func (o *streamObserver) Observe(observation streaming.Observation) {
	select {
	case o.queue <- observation:
	default:
		log.Warn().Msg("Stream observability queue is full; dropping a non-critical audit update")
	}
}

func (o *streamObserver) Close() {
	close(o.queue)
	<-o.done
}

func (o *streamObserver) run() {
	defer close(o.done)
	for observation := range o.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var err error
		switch {
		case observation.Session != nil:
			record := observation.Session
			var channelID *int64
			channelName := ""
			if identity, identityErr := database.Db.GetStreamIdentity(ctx, record.StreamID); identityErr == nil {
				channelID, channelName = &identity.ID, identity.Name
			}
			err = database.Db.UpsertStreamSession(ctx, models.StreamSessionHistory{
				IncidentID: record.IncidentID, StreamID: record.StreamID, State: string(record.State),
				ChannelID: channelID, ChannelName: channelName,
				SourceCount: record.SourceCount, Reconnects: record.Reconnects,
				BytesIngested: record.BytesIngested, BytesDelivered: record.BytesDelivered,
				SlowClientDrops: record.SlowClientDrops, StartedAt: record.StartedAt,
				FirstMediaAt: record.FirstMediaAt, EndedAt: record.EndedAt, EndReason: record.EndReason,
				ErrorCode: record.ErrorCode, LastError: record.LastError,
			})
		case observation.Connection != nil:
			record := observation.Connection
			err = database.Db.UpsertStreamConnection(ctx, models.StreamConnectionHistory{
				ID: record.ID, IncidentID: record.IncidentID, StreamID: record.StreamID,
				Protocol: boundedAuditText(record.Protocol, 16), RemoteIP: boundedAuditText(record.RemoteIP, 64),
				Method: boundedAuditText(record.Method, 32), UserAgent: boundedAuditText(record.UserAgent, 512),
				StartedAt: record.StartedAt, LastSeenAt: record.LastSeenAt,
				EndedAt: record.EndedAt, BytesDelivered: record.BytesDelivered, EndReason: record.EndReason,
			})
		case observation.Event != nil:
			record := observation.Event
			var connectionID *string
			if record.ConnectionID != "" {
				connectionID = &record.ConnectionID
			}
			err = database.Db.InsertStreamEvent(ctx, models.StreamEvent{
				IncidentID: record.IncidentID, ConnectionID: connectionID, StreamID: record.StreamID,
				Severity: boundedAuditText(record.Severity, 16), Code: boundedAuditText(record.Code, 64),
				Message: boundedAuditText(record.Message, 1024), SourcePosition: record.SourcePosition,
				Details: boundedAuditText(record.Details, 8192), CreatedAt: record.CreatedAt,
			})
		}
		cancel()
		if err != nil {
			log.Error().Err(err).Msg("Failed to persist stream observability record")
		}
	}
}

func boundedAuditText(value string, maximumRunes int) string {
	if len(value) <= maximumRunes {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maximumRunes {
		return value
	}
	return string(runes[:maximumRunes-1]) + "…"
}
