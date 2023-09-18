package app

import (
	"fmt"
	"os"
)

var SERVER_PATH = ""
var CONFIG_PATH = os.Getenv("CONFIG_PATH")
var STREAM_PATH = os.Getenv("STREAM_PATH")
var M3U_FILEPATH = fmt.Sprintf("%s/m3u", STREAM_PATH)
var EPG_FILEPATH = fmt.Sprintf("%s/epg", STREAM_PATH)
var LOGO_FILEPATH = fmt.Sprintf("%s/logo", STREAM_PATH)
var MODEL_PATH = os.Getenv("MODEL_PATH")
