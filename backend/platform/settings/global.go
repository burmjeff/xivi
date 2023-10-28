package settings

import (
	"fmt"
)

var APP_SETTINGS = &AppSettings{
	Application: Application{},
	Server:      Server{},
	Streaming:   Streaming{},
}
var SERVER_PATH = ""
var CONFIG_PATH = "./config"
var SERVE_PATH = "./serve"
var M3U_FILEPATH = fmt.Sprintf("%s/m3u", SERVE_PATH)
var EPG_FILEPATH = fmt.Sprintf("%s/epg", SERVE_PATH)
var LOGO_FILEPATH = fmt.Sprintf("%s/logo", SERVE_PATH)
var STREAM_FILEPATH = fmt.Sprintf("%s/stream", SERVE_PATH)
var MODEL_PATH = fmt.Sprintf("%s/models", CONFIG_PATH)

type AppSettings struct {
	Application `yaml:"application" json:"application"`
	Server      `yaml:"server" json:"server"`
	Streaming   `yaml:"streaming" json:"streaming"`
}

type Application struct {
	AppName    string `yaml:"appname" json:"appname"`
	AppVersion string `yaml:"appversion" json:"appversion"`
	TZ         string `yaml:"tz" json:"tz"`
	ServePath  string `yaml:"servepath" json:"servepath"`
	Model      string `yaml:"model" json:"model"`
	LogLevel   int    `yaml:"loglevel" json:"loglevel"`
	UpdateCron string `yaml:"updatecron" json:"updatecron"`
}

type Server struct {
	Host        string `yaml:"host" json:"host"`
	Port        int    `yaml:"port" json:"port"`
	ReadTimeout int    `yaml:"readtimeout" json:"readtimeout"`
}

type Streaming struct {
	Proxy     bool   `yaml:"proxy" json:"proxy"`
	Buffer    int    `yaml:"buffer" json:"buffer"`
	UserAgent string `yaml:"useragent" json:"useragent"`
}
