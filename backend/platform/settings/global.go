package settings

import (
	"fmt"
)

var APP_SETTINGS = &AppSettings{
	Application: Application{},
	Server:      Server{},
	Playlist:    Playlist{},
	Streaming:   Streaming{},
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
}

type Application struct {
	AppName    string `yaml:"appname" json:"appname"`
	AppVersion string `yaml:"appversion" json:"appversion"`
	TZ         string `yaml:"tz" json:"tz"`
	ServePath  string `yaml:"servepath" json:"servepath"`
	LogLevel   int    `yaml:"loglevel" json:"loglevel"`
	UpdateCron string `yaml:"updatecron" json:"updatecron"`
	Ssdp       bool   `yaml:"ssdp" json:"ssdp"`
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
