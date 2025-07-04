package controllers

import (
	"fmt"
	"xivi/backend/pkg/upnp"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// Serves discovery data
func GetDiscover(c *fiber.Ctx) error {
	// Get the primary template (first available template)
	templateID, err := getPrimaryTemplateID()
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No templates available",
		})
	}

	// Get template information
	template, err := database.Db.GetTemplate(templateID)
	if err != nil {
		log.Error().Msgf("Failed to get template %d: %v", templateID, err)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Template not found",
		})
	}

	// Generate discovery data
	discoverData := upnp.GenerateDiscoverData(templateID, template.Name, "")

	return c.JSON(discoverData)
}

// Serves channel lineup status
func GetLineupStatus(c *fiber.Ctx) error {
	// Generate lineup status
	status := upnp.GenerateLineupStatus()
	return c.JSON(status)
}

// GetLineup serves channel lineup (aggregates all templates)
func GetLineup(c *fiber.Ctx) error {
	// Get all templates
	templates, err := database.Db.GetTemplates()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to get templates",
		})
	}

	if len(*templates) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No templates available",
		})
	}

	// Aggregate channels from all templates
	allChannels := make([]upnp.Channel, 0)
	channelNumber := 1

	for _, template := range *templates {
		// Get template channels
		templateChannels, err := database.Db.GetTmplChannels(template.ID)
		if err != nil {
			log.Error().Msgf("Failed to get template channels for template %d: %v", template.ID, err)
			continue // Skip this template but continue with others
		}

		// Convert to lineup format
		for _, tc := range templateChannels {
			channel := upnp.Channel{
				ID:     tc.ID,
				Name:   fmt.Sprintf("%s - %s", template.Name, tc.Name), // Prefix with template name
				UUID:   tc.Uuid,
				Number: channelNumber,
			}
			allChannels = append(allChannels, channel)
			channelNumber++
		}
	}

	// Generate lineup with aggregated channels (use first template ID for compatibility)
	templateID := (*templates)[0].ID
	lineup := upnp.GenerateLineup(templateID, allChannels)

	return c.JSON(lineup)
}

// PostLineup handles lineup POST requests at root level
func PostLineup(c *fiber.Ctx) error {
	// Clients expect this endpoint to exist but it can return empty
	return c.SendString("")
}

// getPrimaryTemplateID returns the ID of the primary template (first available)
func getPrimaryTemplateID() (int64, error) {
	templates, err := database.Db.GetTemplates()
	if err != nil {
		return 0, err
	}

	if len(*templates) == 0 {
		return 0, fmt.Errorf("no templates available")
	}

	// Return the first template as primary
	return (*templates)[0].ID, nil
}
