package settings

import (
	"fmt"
	"sync/atomic"
)

var initialSettings = &AppSettings{
	Application:  Application{},
	Server:       Server{},
	Playlist:     Playlist{},
	Streaming:    Streaming{},
	Vector:       Vector{},
	Maintenance:  Maintenance{},
	VirtualTuner: VirtualTuner{},
	Security:     Security{},
}

var currentSettings atomic.Pointer[AppSettings]

func init() {
	currentSettings.Store(initialSettings)
}

// Current returns the immutable settings snapshot currently used by Xivi.
func Current() *AppSettings {
	return currentSettings.Load()
}

func publish(next *AppSettings) {
	currentSettings.Store(next)
}

var SERVER_PATH = ""
var CONFIG_PATH = "./config"
var SERVE_PATH = "./serve"
var M3U_FILEPATH = fmt.Sprintf("%s/m3u", SERVE_PATH)
var EPG_FILEPATH = fmt.Sprintf("%s/epg", SERVE_PATH)
var LOGO_FILEPATH = fmt.Sprintf("%s/logo", SERVE_PATH)
var STREAM_FILEPATH = fmt.Sprintf("%s/stream", SERVE_PATH)

type AppSettings struct {
	Application  `yaml:"application" json:"application"`
	Server       `yaml:"server" json:"server"`
	Playlist     `yaml:"playlist" json:"playlist"`
	Streaming    `yaml:"streaming" json:"streaming"`
	Vector       `yaml:"vector" json:"vector"`
	Maintenance  `yaml:"maintenance" json:"maintenance"`
	VirtualTuner `yaml:"virtual_tuner" json:"virtual_tuner"`
	Security     `yaml:"security" json:"security"`
}

type Application struct {
	AppName    string `yaml:"appname" json:"appname"`
	AppVersion string `yaml:"appversion" json:"appversion"`
	TZ         string `yaml:"tz" json:"tz"`
	ServePath  string `yaml:"servepath" json:"servepath"`
	LogLevel   int    `yaml:"loglevel" json:"loglevel"`
	UpdateCron string `yaml:"updatecron" json:"updatecron"`
}

type Server struct {
	Host        string `yaml:"host" json:"host"`
	Port        int    `yaml:"port" json:"port"`
	ReadTimeout int    `yaml:"readtimeout" json:"readtimeout"`
}

type Playlist struct {
	Tvgid_match bool    `yaml:"tvgid_match" json:"tvgid_match"`
	Name_match  bool    `yaml:"name_match" json:"name_match"`
	Name_score  float64 `yaml:"name_score" json:"name_score"`
}

type Streaming struct {
	Proxy                 bool   `yaml:"proxy" json:"proxy"`
	IngestBufferMS        int    `yaml:"ingest_buffer_ms" json:"ingest_buffer_ms"`
	StartupTimeoutSeconds int    `yaml:"startup_timeout_seconds" json:"startup_timeout_seconds"`
	StartupHedgeMS        int    `yaml:"startup_hedge_ms" json:"startup_hedge_ms"`
	StallTimeoutSeconds   int    `yaml:"stall_timeout_seconds" json:"stall_timeout_seconds"`
	HedgeTimeoutSeconds   int    `yaml:"hedge_timeout_seconds" json:"hedge_timeout_seconds"`
	IdleTimeoutSeconds    int    `yaml:"idle_timeout_seconds" json:"idle_timeout_seconds"`
	RetryLimit            int    `yaml:"retry_limit" json:"retry_limit"`
	RetryBackoffMS        int    `yaml:"retry_backoff_ms" json:"retry_backoff_ms"`
	HLSSegmentSeconds     int    `yaml:"hls_segment_seconds" json:"hls_segment_seconds"`
	HLSPlaylistLength     int    `yaml:"hls_playlist_length" json:"hls_playlist_length"`
	HLSCompatibilityMode  bool   `yaml:"hls_compatibility_mode" json:"hls_compatibility_mode"`
	ClientBufferMB        int    `yaml:"client_buffer_mb" json:"client_buffer_mb"`
	PrewarmChannels       int    `yaml:"prewarm_channels" json:"prewarm_channels"`
	TLSVerify             bool   `yaml:"tls_verify" json:"tls_verify"`
	UserAgent             string `yaml:"useragent" json:"useragent"`
}

type Vector struct {
	BatchSize       int `yaml:"batch_size" json:"batch_size"`
	ParallelBatches int `yaml:"parallel_batches" json:"parallel_batches"`
	Timeout         int `yaml:"timeout" json:"timeout"`
	CacheSize       int `yaml:"cache_size" json:"cache_size"`
}

// Maintenance controls bounded retention for generated operational data. The
// record ceilings are hard safety limits in addition to the age-based policy,
// so an unusually noisy source cannot consume unbounded storage between runs.
type Maintenance struct {
	CleanupIntervalHours      int `yaml:"cleanup_interval_hours" json:"cleanup_interval_hours"`
	OperationJobRetentionDays int `yaml:"operation_job_retention_days" json:"operation_job_retention_days"`
	StreamDiagnosticsDays     int `yaml:"stream_diagnostics_days" json:"stream_diagnostics_days"`
	MaximumOperationJobs      int `yaml:"maximum_operation_jobs" json:"maximum_operation_jobs"`
	MaximumStreamSessions     int `yaml:"maximum_stream_sessions" json:"maximum_stream_sessions"`
}

type VirtualTuner struct {
	TunerCount int `yaml:"tuner_count" json:"tuner_count"`
}

// Security contains only non-secret deployment policy. The authentication
// root key is supplied through XIVI_AUTH_KEY_FILE and is never serialized.
type Security struct {
	PublicBaseURL      string   `yaml:"public_base_url" json:"public_base_url"`
	LocalBaseURL       string   `yaml:"local_base_url" json:"local_base_url"`
	TrustedProxyCIDRs  []string `yaml:"trusted_proxy_cidrs" json:"trusted_proxy_cidrs"`
	TrustedLANCIDRs    []string `yaml:"trusted_lan_cidrs" json:"trusted_lan_cidrs"`
	AllowLANHTTP       bool     `yaml:"allow_lan_http" json:"allow_lan_http"`
	AuditRetentionDays int      `yaml:"audit_retention_days" json:"audit_retention_days"`
	MaximumAuditEvents int      `yaml:"maximum_audit_events" json:"maximum_audit_events"`
}
