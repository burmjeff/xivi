package settings

import (
	"fmt"
)

var APP_SETTINGS = &AppSettings{
	Application: Application{},
	Server:      Server{},
	Playlist:    Playlist{},
	Streaming:   Streaming{},
	Vector:      Vector{},
	UPnP:        UPnP{},
}
var SERVER_PATH = ""
var CONFIG_PATH = "./config"
var SERVE_PATH = "./serve"
var M3U_FILEPATH = fmt.Sprintf("%s/m3u", SERVE_PATH)
var EPG_FILEPATH = fmt.Sprintf("%s/epg", SERVE_PATH)
var LOGO_FILEPATH = fmt.Sprintf("%s/logo", SERVE_PATH)
var STREAM_FILEPATH = fmt.Sprintf("%s/stream", SERVE_PATH)

type AppSettings struct {
	Application `yaml:"application" json:"application"`
	Server      `yaml:"server" json:"server"`
	Playlist    `yaml:"playlist" json:"playlist"`
	Streaming   `yaml:"streaming" json:"streaming"`
	Vector      `yaml:"vector" json:"vector"`
	UPnP        `yaml:"upnp" json:"upnp"`
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
	Type      string `yaml:"type" json:"type"`
	Proxy     bool   `yaml:"proxy" json:"proxy"`
	Buffer    int    `yaml:"buffer" json:"buffer"`
	RetryEOS  int    `yaml:"retryeos" json:"retryeos"`
	UserAgent string `yaml:"useragent" json:"useragent"`
}

type Vector struct {
	BatchSize       int `yaml:"batch_size" json:"batch_size"`
	ParallelBatches int `yaml:"parallel_batches" json:"parallel_batches"`
	Timeout         int `yaml:"timeout" json:"timeout"`
	CacheSize       int `yaml:"cache_size" json:"cache_size"`
}

type UPnP struct {
	Enabled         bool   `yaml:"enabled" json:"enabled"`
	Manufacturer    string `yaml:"manufacturer" json:"manufacturer"`
	ModelName       string `yaml:"model_name" json:"model_name"`
	ModelNumber     string `yaml:"model_number" json:"model_number"`
	FirmwareName    string `yaml:"firmware_name" json:"firmware_name"`
	FirmwareVersion string `yaml:"firmware_version" json:"firmware_version"`
	DeviceAuth      string `yaml:"device_auth" json:"device_auth"`
	TunerCount      int    `yaml:"tuner_count" json:"tuner_count"`
}
