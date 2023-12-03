package controllers

import (
	"strconv"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
)

// CreateM3U func generates a new m3u file from template.
// @Description CreateM3U func generates new m3u file from template.
// @Summary CreateM3U func generates new m3u file from template.
// @Tags M3U
// @Accept json
// @Param template_id path string true "Template ID"
// @Success 200 {object} models.Template
// @Router /m3u/{template_id} [post]
func CreateM3U(c *fiber.Ctx) error {
	// Catch template ID from URL.
	template_id, err := strconv.ParseInt(c.Params("template_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
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

	// Checking, if template with given ID is exists.
	template, err := db.GetTemplate(template_id)
	if err != nil {
		// Return status 404 and template not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "template with this ID not found",
		})
	}

	m3uTools := utils.M3uTools{Db: db}
	go m3uTools.CreateM3u(template)
	go utils.CreateEpgXML(db, template)

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"template": template,
	})
}
