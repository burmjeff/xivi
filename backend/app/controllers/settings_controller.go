package controllers

import (
	"xivi/backend/pkg/utils"
	"xivi/backend/pkg/virtualtuner"
	"xivi/backend/platform/cron"
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
		"settings": settings.Current(),
		"deployment_managed": []string{
			"application.servepath",
			"server.host",
			"server.port",
			"server.readtimeout",
		},
	})
}

// UpdateSettings
// @Description Update Settings.
// @Summary update Settings
// @Tags Settings
// @Accept json
// @Produce json
// @Param settings body array true "Settings"
// @Success 201 {string} status "ok"
// @Router /settings [put]
func UpdateSettings(c *fiber.Ctx) error {

	// Create new Settings struct
	newSettings := &settings.AppSettings{}

	// Check, if received JSON data is valid.
	if err := decodeStrict(c, newSettings); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	settings.PreserveDeploymentSettings(newSettings)

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
	if err := cron.ValidateConfiguration(newSettings); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	previous := settings.Current()
	if err := settings.WriteSettings(newSettings); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	rollback := func() {
		_ = settings.WriteSettings(previous)
		_ = cron.Reconfigure()
		_ = virtualtuner.Reconfigure()
	}
	if err := cron.Reconfigure(); err != nil {
		rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "The background schedule could not be applied.",
		})
	}
	if err := virtualtuner.Reconfigure(); err != nil {
		rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Virtual tuner discovery could not be reconfigured.",
		})
	}
	// Return status 201.
	return c.SendStatus(fiber.StatusCreated)
}
