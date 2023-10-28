package controllers

import (
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

// GetSettings
// @Description Get all settings.
// @Summary get all settings
// @Tags Settings
// @Accept json
// @Produce json
// @Success 200 {array} settings.AppSettings
// @Router /settings [get]
func GetSettings(c *fiber.Ctx) error {

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"settings": settings.APP_SETTINGS,
	})
}

// UpdateSettings
// @Description Update playlist.
// @Summary update playlist
// @Tags Settings
// @Accept json
// @Produce json
// @Param settings body array true "Settings"
// @Success 201 {string} status "ok"
// @Security ApiKeyAuth
// @Router /settings [put]
func UpdateSettings(c *fiber.Ctx) error {

	// Create new Settings struct
	newSettings := &settings.AppSettings{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(newSettings); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create a new validator for a Settings model.
	validate := utils.NewValidator()

	// Validate playlist fields.
	if err := validate.Struct(newSettings); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Update playlist by given ID.
	if err := settings.WriteSettings(newSettings); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 201.
	return c.SendStatus(fiber.StatusCreated)
}
