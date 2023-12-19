package controllers

import (
	"fmt"
	"xivi/backend/app/models"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
)

// GetSSDPDiscovery func SSDP Device Discovery.
// @Description Get SSDP Device Discovery.
// @Summary get SSDP Device Discovery
// @Tags SSDP
// @Produce json
// @Success 200 {array} models.DiscoveryData
// @Router /discover.json [get]
func GetSSDPDiscovery(c *fiber.Ctx, playlist string) error {

	discoveryData := models.DiscoveryData{
		FriendlyName:    settings.APP_SETTINGS.AppName,
		Manufacturer:    settings.APP_SETTINGS.AppName,
		ModelNumber:     fmt.Sprintf("%s-%s", settings.APP_SETTINGS.AppName, settings.APP_SETTINGS.AppVersion),
		FirmwareName:    fmt.Sprintf("bin_%s", settings.APP_SETTINGS.AppName),
		TunerCount:      3, //TODO TUNER COUNT PER PLAYLIST
		FirmwareVersion: settings.APP_SETTINGS.AppVersion,
		DeviceID:        fmt.Sprintf("%s-%s-%s", settings.APP_SETTINGS.AppName, settings.APP_SETTINGS.AppVersion, playlist),
		DeviceAuth:      settings.APP_SETTINGS.AppName,
		BaseURL:         fmt.Sprintf("http://%s:%d/%s", settings.APP_SETTINGS.Host, settings.APP_SETTINGS.Port, playlist),
		LineupURL:       fmt.Sprintf("http://%s:%d/%s/lineup.json", settings.APP_SETTINGS.Host, settings.APP_SETTINGS.Port, playlist),
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"":      discoveryData,
	})
}
