package controllers

import (
	"net/http"
	"xivi/backend/pkg/streaming"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

type AppSettings struct {
	Proxy      bool `json:"proxy,omitempty"`
	Buffer     bool `json:"buffer,omitempty"`
	BufferTime int  `json:"buffertime,omitempty"`
}

// GetStream func gets stream.
// @Description Get stream by given UUID.
// @Summary get stream by given UUID
// @Tags Stream
// @Accept json
// @Produce octet-stream
// @Param stream_id path string true "Stream ID"
// @Router /stream/{stream_id} [get]
func GetStream(c *fiber.Ctx) error {
	// Catch stream ID from URL.
	stream_id := c.Params("stream_id")
	if stream_id == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Empty Stream UUID",
		})
	}

	// Get channels by UUID.
	channels, err := database.Db.GetChannelsbyUuid(stream_id)
	if err != nil {
		// Return, if no channels found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":    true,
			"msg":      "Stream Error",
			"channels": nil,
		})
	}

	if len(channels) > 0 {
		switch settings.APP_SETTINGS.Streaming.Proxy {

		case false:
			//TODO LOOP CHECK STREAM STATUS UNTIL 302
			return c.Redirect(channels[0].Url, http.StatusTemporaryRedirect)

		case true:
			for _, stream := range streaming.Streams {
				if stream.Settings.Src == channels[0].Url {
					if err := stream.NewSink(c.Context()); err != nil {
						return err
					}
					return nil
				}

			}

			s := streaming.NewStreamer()
			if err := s.StartStream(channels[0].Url); err != nil {
				return err
			}
			streaming.AddStream(s)

			if err := s.NewSink(c.Context()); err != nil {
				s.Close(nil, nil)
				return err
			}

		}
	} else {
		// Return, if no channels found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":    true,
			"msg":      "Stream Error",
			"channels": nil,
		})
	}
	return nil
}
