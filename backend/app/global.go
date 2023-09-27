package app

import (
	"fmt"
	"os"
)

var SERVER_PATH = ""
var CONFIG_PATH = os.Getenv("CONFIG_PATH")
var SERVE_PATH = os.Getenv("SERVE_PATH")
var M3U_FILEPATH = fmt.Sprintf("%s/m3u", SERVE_PATH)
var EPG_FILEPATH = fmt.Sprintf("%s/epg", SERVE_PATH)
var LOGO_FILEPATH = fmt.Sprintf("%s/logo", SERVE_PATH)
var STREAM_FILEPATH = fmt.Sprintf("%s/stream", SERVE_PATH)
var MODEL_PATH = os.Getenv("MODEL_PATH")
