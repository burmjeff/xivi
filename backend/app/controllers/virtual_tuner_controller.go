package controllers

import (
	"strconv"
	"xivi/backend/pkg/virtualtuner"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

func loadVirtualTunerDevice(c *fiber.Ctx) (virtualtuner.Device, error) {
	lineupID, err := strconv.ParseInt(c.Params("lineup_id"), 10, 64)
	if err != nil {
		return virtualtuner.Device{}, fiber.NewError(fiber.StatusBadRequest, "The lineup id is invalid.")
	}
	lineup, err := database.Db.GetTemplate(lineupID)
	if err != nil {
		return virtualtuner.Device{}, fiber.NewError(fiber.StatusNotFound, "The lineup was not found.")
	}
	if !lineup.VirtualTunerEnabled {
		return virtualtuner.Device{}, fiber.NewError(fiber.StatusNotFound, "The virtual tuner is disabled for this lineup.")
	}
	return virtualtuner.NewDevice(*lineup, c.BaseURL(), settings.APP_SETTINGS.VirtualTuner.TunerCount), nil
}

func GetVirtualTunerDiscover(c *fiber.Ctx) error {
	device, err := loadVirtualTunerDevice(c)
	if err != nil {
		return err
	}
	return c.JSON(device.Discover())
}

func GetVirtualTunerLineupStatus(c *fiber.Ctx) error {
	if _, err := loadVirtualTunerDevice(c); err != nil {
		return err
	}
	return c.JSON(virtualtuner.Status())
}

func GetVirtualTunerLineup(c *fiber.Ctx) error {
	device, err := loadVirtualTunerDevice(c)
	if err != nil {
		return err
	}
	channels, err := database.Db.GetPlayableTmplChannels(device.LineupID)
	if err != nil {
		log.Error().Err(err).Int64("lineup_id", device.LineupID).Msg("Virtual tuner lineup could not be loaded")
		return fiber.NewError(fiber.StatusInternalServerError, "The tuner lineup could not be loaded.")
	}
	return c.JSON(virtualtuner.Lineup(c.BaseURL(), channels))
}

func PostVirtualTunerLineup(c *fiber.Ctx) error {
	if _, err := loadVirtualTunerDevice(c); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusOK)
}

func GetVirtualTunerDeviceDescription(c *fiber.Ctx) error {
	device, err := loadVirtualTunerDevice(c)
	if err != nil {
		return err
	}
	description, err := device.DescriptionXML()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "The device description could not be generated.")
	}
	c.Type("xml", "utf-8")
	return c.Send(description)
}

func V2DeviceOutputs(c *fiber.Ctx) error {
	devices, err := virtualtuner.AllDevices(c.BaseURL())
	if err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "device_outputs_unavailable", "Device outputs could not be loaded.", true)
	}
	enabledCount := 0
	for _, device := range devices {
		if device.Enabled {
			enabledCount++
		}
	}
	return c.JSON(fiber.Map{
		"virtual_tuner": fiber.Map{
			"enabled_count": enabledCount,
			"running":       virtualtuner.Running(),
			"port":          65001,
			"devices":       devices,
		},
	})
}

func V2SetLineupVirtualTunerEnabled(c *fiber.Ctx) error {
	lineupID, err := strconv.ParseInt(c.Params("lineup_id"), 10, 64)
	if err != nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_lineup", "The lineup id is invalid.", false)
	}
	request := struct {
		Enabled *bool `json:"enabled"`
	}{}
	if err := c.BodyParser(&request); err != nil || request.Enabled == nil {
		return v2Error(c, fiber.StatusBadRequest, "invalid_device_output", "An enabled value is required.", false)
	}

	lineup, err := database.Db.GetTemplate(lineupID)
	if err != nil {
		return v2Error(c, fiber.StatusNotFound, "lineup_not_found", "The lineup was not found.", false)
	}
	if lineup.VirtualTunerEnabled == *request.Enabled {
		return V2DeviceOutputs(c)
	}
	if err := database.Db.SetTemplateVirtualTunerEnabled(lineupID, *request.Enabled); err != nil {
		return v2Error(c, fiber.StatusInternalServerError, "device_output_save_failed", "The device output setting could not be saved.", true)
	}
	if err := virtualtuner.Reconfigure(); err != nil {
		_ = database.Db.SetTemplateVirtualTunerEnabled(lineupID, lineup.VirtualTunerEnabled)
		_ = virtualtuner.Reconfigure()
		return v2Error(c, fiber.StatusConflict, "virtual_tuner_discovery_unavailable", err.Error(), true)
	}
	return V2DeviceOutputs(c)
}
