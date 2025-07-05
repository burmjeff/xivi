package controllers

import (
	"strconv"
	"xivi/backend/pkg/ssdp"
	"xivi/backend/pkg/upnp"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// GetTemplateDiscover serves discovery data for a specific template
func GetTemplateDiscover(c *fiber.Ctx) error {
	templateID, err := strconv.ParseInt(c.Params("template_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid template ID",
		})
	}

	template, err := database.Db.GetTemplate(templateID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Template not found",
		})
	}

	// Generate template-specific discovery data
	deviceUUID := ssdp.GenerateTemplateUUID(templateID)
	discoverData := upnp.GenerateDiscoverData(templateID, template.Name, deviceUUID)

	return c.JSON(discoverData)
}

// GetTemplateLineupStatus serves lineup status for a specific template
func GetTemplateLineupStatus(c *fiber.Ctx) error {
	templateID, err := strconv.ParseInt(c.Params("template_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid template ID",
		})
	}

	// Verify template exists
	_, err = database.Db.GetTemplate(templateID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Template not found",
		})
	}

	// Generate lineup status
	status := upnp.GenerateLineupStatus()
	return c.JSON(status)
}

// GetTemplateLineup serves channel lineup for a specific template
func GetTemplateLineup(c *fiber.Ctx) error {
	templateID, err := strconv.ParseInt(c.Params("template_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid template ID",
		})
	}

	_, err = database.Db.GetTemplate(templateID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Template not found",
		})
	}

	// Get template channels
	templateChannels, err := database.Db.GetTmplChannels(templateID)
	if err != nil {
		log.Error().Msgf("Failed to get template channels for template %d: %v", templateID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get template channels",
		})
	}

	// Convert to lineup format
	channels := make([]upnp.Channel, 0, len(templateChannels))
	for i, tc := range templateChannels {
		channel := upnp.Channel{
			ID:     tc.ID,
			Name:   tc.Name, // Don't prefix with template name for template-specific endpoint
			UUID:   tc.Uuid,
			Number: i + 1, // Sequential numbering starting from 1
		}
		channels = append(channels, channel)
	}

	// Generate lineup for this template
	lineup := upnp.GenerateLineup(templateID, channels)

	return c.JSON(lineup)
}

// PostTemplateLineup handles lineup POST requests for a specific template
func PostTemplateLineup(c *fiber.Ctx) error {
	templateID, err := strconv.ParseInt(c.Params("template_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   "Invalid template ID",
		})
	}

	// Verify template exists
	_, err = database.Db.GetTemplate(templateID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Template not found",
		})
	}

	// Clients expect this endpoint to exist but it can return empty
	return c.SendString("")
}

// GetDeviceDescription serves UPnP device description XML
func GetDeviceDescription(c *fiber.Ctx) error {
	deviceUUID := c.Params("device_uuid")

	// Find device by UUID in SSDP service
	service := ssdp.GetService()
	device, exists := service.GetDevice(deviceUUID)
	if !exists {
		return c.Status(fiber.StatusNotFound).SendString("Device not found")
	}

	// Generate device description XML
	deviceXML := upnp.GenerateDeviceDescriptionXML(device)

	c.Set("Content-Type", "application/xml")
	return c.SendString(deviceXML)
}
