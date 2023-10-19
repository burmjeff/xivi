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
	Application `yaml:"application"`
	Server      `yaml:"server"`
	Streaming   `yaml:"streaming"`
}

type Application struct {
	AppName    string `yaml:"appname"`
	AppVersion string `yaml:"appversion"`
	TZ         string `yaml:"tz"`
	ServePath  string `yaml:"servepath"`
	Model      string `yaml:"model"`
	GST_DEBUG  int    `yaml:"gstdebug"`
}

type Server struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	ReadTimeout int    `yaml:"readtimeout"`
}

type Streaming struct {
	Proxy     bool   `yaml:"proxy"`
	Buffer    int    `yaml:"buffer"`
	UserAgent string `yaml:"useragent"`
}
