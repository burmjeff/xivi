package streaming

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"regexp"
	"strings"
	"time"
	"xivi/backend/pkg/security"
)

var diagnosticURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

type ClientMetadata struct {
	ID          string
	Protocol    string
	RemoteIP    string
	Method      string
	UserAgent   string
	AuthKind    string
	AuthID      int64
	OwnerUserID int64
}

type ClientSnapshot struct {
	ID             string    `json:"id"`
	Protocol       string    `json:"protocol"`
	RemoteIP       string    `json:"remote_ip"`
	Method         string    `json:"method"`
	UserAgent      string    `json:"user_agent"`
	StartedAt      time.Time `json:"started_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	BytesDelivered uint64    `json:"bytes_delivered"`
	BitrateBPS     uint64    `json:"bitrate_bps"`
	EndReason      string    `json:"end_reason,omitempty"`
	AuthKind       string    `json:"auth_kind,omitempty"`
}

type MetricSample struct {
	At            time.Time `json:"at"`
	IngressBPS    uint64    `json:"ingress_bps"`
	EgressBPS     uint64    `json:"egress_bps"`
	ActiveClients int       `json:"active_clients"`
}

type SessionRecord struct {
	IncidentID      string
	StreamID        string
	State           State
	SourceCount     int
	Reconnects      uint64
	BytesIngested   uint64
	BytesDelivered  uint64
	SlowClientDrops uint64
	StartedAt       time.Time
	FirstMediaAt    *time.Time
	EndedAt         *time.Time
	EndReason       string
	ErrorCode       string
	LastError       string
}

type ConnectionRecord struct {
	ID             string
	IncidentID     string
	StreamID       string
	Protocol       string
	RemoteIP       string
	Method         string
	UserAgent      string
	StartedAt      time.Time
	LastSeenAt     time.Time
	EndedAt        *time.Time
	BytesDelivered uint64
	EndReason      string
}

type EventRecord struct {
	IncidentID     string
	ConnectionID   string
	StreamID       string
	Severity       string
	Code           string
	Message        string
	SourcePosition int
	Details        string
	CreatedAt      time.Time
}

type Observation struct {
	Session    *SessionRecord
	Connection *ConnectionRecord
	Event      *EventRecord
}

type Observer interface {
	Observe(Observation)
}

func randomIdentifier(prefix string) string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return prefix + time.Now().UTC().Format("20060102150405.000000000")
	}
	return prefix + hex.EncodeToString(value)
}

func ClassifyError(err error) (code string, retryable bool) {
	if err == nil {
		return "unknown", false
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "producer cleanup"):
		return "producer_cleanup_timeout", false
	case strings.Contains(message, "no such host"), strings.Contains(message, "server misbehaving"), strings.Contains(message, "lookup"):
		return "dns_failure", true
	case strings.Contains(message, "connection refused"):
		return "connection_refused", true
	case strings.Contains(message, "connection limit"):
		return "source_connection_limit", true
	case strings.Contains(message, "hls") || strings.Contains(message, "playlist") || strings.Contains(message, "segment"):
		return "hls_not_ready", true
	case strings.Contains(message, "timed out"), strings.Contains(message, "timeout"), strings.Contains(message, "deadline exceeded"):
		return "upstream_timeout", true
	case strings.Contains(message, "certificate"), strings.Contains(message, "tls"):
		return "tls_failure", false
	case strings.Contains(message, "401"), strings.Contains(message, "403"), strings.Contains(message, "unauthorized"), strings.Contains(message, "forbidden"):
		return "upstream_authorization", false
	case strings.Contains(message, "404"), strings.Contains(message, "not found"):
		return "upstream_not_found", false
	case strings.Contains(message, "mpeg-ts"), strings.Contains(message, "mpegts"), strings.Contains(message, "pat"), strings.Contains(message, "pmt"):
		return "invalid_transport_stream", true
	case strings.Contains(message, "unsupported"), strings.Contains(message, "decode"):
		return "unsupported_media", false
	case strings.Contains(message, "stall"), strings.Contains(message, "no media"):
		return "media_stalled", true
	case strings.Contains(message, "end-of-stream"), strings.Contains(message, "eos"):
		return "upstream_ended", true
	case strings.Contains(message, "manual"):
		return "manual_failover", true
	default:
		return "upstream_failure", true
	}
}

func SanitizeDiagnostic(message string) string {
	message = diagnosticURLPattern.ReplaceAllStringFunc(message, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[upstream URL]"
		}
		return parsed.Scheme + "://" + parsed.Host + "/…"
	})
	return security.RedactSensitiveText(message)
}
