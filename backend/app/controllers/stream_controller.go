package controllers

import (
	"xivi/backend/pkg/streaming"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
)

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

	// Get template by ID.
	channels, err := db.GetChannelsbyUuid(stream_id)
	if err != nil {
		// Return, if template not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":    true,
			"msg":      "Stream Error",
			"template": nil,
		})
	}

	// Return status redirect.
	loc, status := streaming.StartStream(stream_id, channels)
	if loc == "" {
		loc = c.OriginalURL()
	}
	return c.Redirect(loc, status)
}
