package models

import "time"

type StreamIdentity struct {
	ID       int64   `db:"id" json:"id"`
	Name     string  `db:"name" json:"name"`
	TvgID    *string `db:"tvgid" json:"tvg_id"`
	LogoName string  `db:"logo_name" json:"logo_name"`
}

type StreamSessionHistory struct {
	IncidentID      string     `db:"incident_id" json:"incident_id"`
	StreamID        string     `db:"stream_id" json:"stream_id"`
	ChannelID       *int64     `db:"channel_id" json:"channel_id,omitempty"`
	ChannelName     string     `db:"channel_name" json:"channel_name,omitempty"`
	State           string     `db:"state" json:"state"`
	SourceCount     int        `db:"source_count" json:"source_count"`
	Reconnects      uint64     `db:"reconnects" json:"reconnects"`
	BytesIngested   uint64     `db:"bytes_ingested" json:"bytes_ingested"`
	BytesDelivered  uint64     `db:"bytes_delivered" json:"bytes_delivered"`
	SlowClientDrops uint64     `db:"slow_client_drops" json:"slow_client_drops"`
	StartedAt       time.Time  `db:"started_at" json:"started_at"`
	FirstMediaAt    *time.Time `db:"first_media_at" json:"first_media_at,omitempty"`
	EndedAt         *time.Time `db:"ended_at" json:"ended_at,omitempty"`
	EndReason       string     `db:"end_reason" json:"end_reason,omitempty"`
	ErrorCode       string     `db:"error_code" json:"error_code,omitempty"`
	LastError       string     `db:"last_error" json:"last_error,omitempty"`
}

type StreamConnectionHistory struct {
	ID             string     `db:"id" json:"id"`
	IncidentID     string     `db:"incident_id" json:"incident_id"`
	StreamID       string     `db:"stream_id" json:"stream_id"`
	Protocol       string     `db:"protocol" json:"protocol"`
	RemoteIP       string     `db:"remote_ip" json:"remote_ip"`
	Method         string     `db:"method" json:"method"`
	UserAgent      string     `db:"user_agent" json:"user_agent"`
	StartedAt      time.Time  `db:"started_at" json:"started_at"`
	LastSeenAt     time.Time  `db:"last_seen_at" json:"last_seen_at"`
	EndedAt        *time.Time `db:"ended_at" json:"ended_at,omitempty"`
	BytesDelivered uint64     `db:"bytes_delivered" json:"bytes_delivered"`
	EndReason      string     `db:"end_reason" json:"end_reason,omitempty"`
}

type StreamEvent struct {
	ID             int64     `db:"id" json:"id"`
	IncidentID     string    `db:"incident_id" json:"incident_id"`
	ConnectionID   *string   `db:"connection_id" json:"connection_id,omitempty"`
	StreamID       string    `db:"stream_id" json:"stream_id"`
	Severity       string    `db:"severity" json:"severity"`
	Code           string    `db:"code" json:"code"`
	Message        string    `db:"message" json:"message"`
	SourcePosition int       `db:"source_position" json:"source_position"`
	Details        string    `db:"details" json:"details,omitempty"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
}
