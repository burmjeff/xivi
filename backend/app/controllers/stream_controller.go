package controllers

import (
	"net/http"
	"xivi/backend/app/models"
	"xivi/backend/pkg/streaming"
	"xivi/backend/platform/database"

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

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get channels by UUID.
	channels, err := db.GetChannelsbyUuid(stream_id)
	if err != nil {
		// Return, if no channels found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":    true,
			"msg":      "Stream Error",
			"channels": nil,
		})
	}

	appSettings := models.Settings{
		Proxy:      true,
		Buffer:     false,
		BufferTime: 0,
	}

	switch appSettings.Proxy {

	case false:
		//TODO LOOP CHECK STREAM STATUS UNTIL 302
		return c.Redirect(channels[0].Url, http.StatusTemporaryRedirect)

	case true:
		s := streaming.NewHTTPStreamer(c.Context())

		if err := s.StartStream(channels[0].Url, c); err != nil {
			return err
		}

	}
	return nil
}
