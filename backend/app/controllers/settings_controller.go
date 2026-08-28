package controllers

import (
	"errors"
	"xivi/backend/pkg/utils"
	"xivi/backend/pkg/virtualtuner"
	"xivi/backend/platform/cron"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

func settingsError(c *fiber.Ctx, status int, code, message string, retryable bool, fieldErrors map[string]string) error {
	response := fiber.Map{
		"error":     true,
		"msg":       message,
		"code":      code,
		"message":   message,
		"retryable": retryable,
	}
	if len(fieldErrors) > 0 {
		response["field_errors"] = fieldErrors
	}
	return c.Status(status).JSON(response)
}

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
		return settingsError(c, fiber.StatusBadRequest, "invalid_settings_request", err.Error(), false, nil)
	}
	settings.PreserveDeploymentSettings(newSettings)

	// Create a new validator for a Settings model.
	validate := utils.NewValidator()

	// Validate playlist fields.
	if err := validate.Struct(newSettings); err != nil {
		return settingsError(c, fiber.StatusBadRequest, "invalid_settings", "Some settings fields are invalid.", false, utils.ValidatorErrors(err))
	}
	if err := cron.ValidateConfiguration(newSettings); err != nil {
		return settingsError(c, fiber.StatusBadRequest, "invalid_schedule_settings", err.Error(), false, nil)
	}
	if err := settings.ValidateStreamingSecurityLimits(newSettings.Streaming); err != nil {
		return settingsError(c, fiber.StatusBadRequest, "invalid_streaming_settings", err.Error(), false, nil)
	}
	if err := settings.ValidateSecuritySettings(newSettings.Security); err != nil {
		fieldErrors := map[string]string{}
		var validation *settings.SecurityValidationError
		if errors.As(err, &validation) {
			fieldErrors[validation.Field] = validation.Message
		}
		return settingsError(c, fiber.StatusBadRequest, "invalid_security_settings", err.Error(), false, fieldErrors)
	}

	previous := settings.Current()
	if err := settings.WriteSettings(newSettings); err != nil {
		log.Error().Err(err).Msg("Settings file could not be updated")
		return settingsError(c, fiber.StatusInternalServerError, "settings_persistence_failed",
			"The settings file could not be updated. Check that the configuration volume is writable.", true, nil)
	}
	rollback := func() {
		_ = settings.WriteSettings(previous)
		_ = cron.Reconfigure()
		_ = virtualtuner.Reconfigure()
	}
	if err := cron.Reconfigure(); err != nil {
		rollback()
		log.Error().Err(err).Msg("Updated background schedule could not be applied")
		return settingsError(c, fiber.StatusInternalServerError, "schedule_reconfigure_failed",
			"The background schedule could not be applied.", true, nil)
	}
	if err := virtualtuner.Reconfigure(); err != nil {
		rollback()
		log.Error().Err(err).Msg("Virtual tuner discovery could not be reconfigured after settings update")
		return settingsError(c, fiber.StatusInternalServerError, "virtual_tuner_reconfigure_failed",
			"Virtual tuner discovery could not be reconfigured.", true, nil)
	}
	// Return status 201.
	return c.SendStatus(fiber.StatusCreated)
}
