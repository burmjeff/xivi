package streaming

import (
	"fmt"
	"xivi/backend/app"
	"xivi/backend/app/models"

	"github.com/go-gst/go-glib/glib"
	log "github.com/sirupsen/logrus"
)

func StartStream(stream_id string, channels []models.ChannelUrl) (string, int) {
	appSettings := models.Settings{
		Proxy:      true,
		Buffer:     false,
		BufferTime: 0,
	}

	switch appSettings.Proxy {

	case false:
		//TODO LOOP CHECK STREAM STATUS UNTIL 302
		return channels[0].Url, 302

	case true:
		s := Stream{
			settings: &settings{
				userAgent: "Xivi 1.0",
			},
			state: &state{},
		}

		switch appSettings.Buffer {

		case false:
			s.settings.buffer = 0
		case true:
			s.settings.buffer = appSettings.BufferTime
		}

		for _, channel := range channels {
			s.settings.src = channel.Url
			s.settings.sink = fmt.Sprintf("%s/%s", app.STREAM_FILEPATH, stream_id)

			err := func(loop *glib.MainLoop) error {
				err := s.createPipeline()
				if err != nil {
					return err
				}
				return s.StartStream(loop)
			}
			if err != nil {
				log.Error(err)
				continue
			}
			return "", 302
		}
		//log.Infoln("Streaming URL:" + channel.Url)

	}
	return "", 404
}
