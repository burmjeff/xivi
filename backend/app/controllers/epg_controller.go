package controllers

import (
	"context"
	"strconv"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/cron"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// GetEpgs func gets all epgs.
// @Description Get all epgs.
// @Summary get all epgs
// @Tags Epg
// @Accept json
// @Produce json
// @Success 200 {array} models.Epg
// @Router /epgs [get]
func GetEpgs(c *fiber.Ctx) error {
	ctx := context.Background()
	// Get all epgs.
	epgs, err := database.Db.GetEpgs(ctx)
	if err != nil {
		// Return, if epgs not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No epgs found",
			"count": 0,
			"epgs":  nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"count": len(*epgs),
		"epgs":  epgs,
	})
}

// CreateEPG func generates a new epg file from template.
// @Description CreateEPG func generates new epg xml file from template.
// @Summary CreateEPG func generates new epg xml file from template.
// @Tags Epg
// @Accept json
// @Produce json
// @Param id path string true "Template ID"
// @Success 200 {object} models.Template
// @Router /epg/create/{id} [post]
func CreateEpg(c *fiber.Ctx) error {
	// Catch template ID from URL.
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Checking, if template with given ID is exists.
	template, err := database.Db.GetTemplate(id)
	if err != nil {
		// Return status 404 and template not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "template with this ID not found",
		})
	}

	go utils.CreateEpgXML(*template)

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"template": template,
	})
}

// AddEpg func for createing a new epg.
// @Summary Add a new epg
// @Description Add a new epg and parse.
// @Tags Epg
// @Accept json
// @Produce json
// @Param epg body models.EpgAddParam true "Epg"
// @Success 200 {object} models.Epg
// @Router /epg [post]
func AddEpg(c *fiber.Ctx) error {
	ctx := context.Background()
	// Create new Epg struct
	epg := &models.Epg{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(epg); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	epg.CreatedAt = time.Now()

	// Create a new validator for a Epg model.
	validate := utils.NewValidator()

	// Validate epg fields.
	if err := validate.Struct(epg); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Create epg.
	id, err := database.Db.CreateEpg(ctx, epg)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}
	epg.ID = id

	//TODO async Parse m3u and insert channels
	go utils.ParseEpg(epg)

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"epg":   epg,
	})
}

// UpdateEpg func for updates epg by given ID.
// @Description Update epg.
// @Summary update epg
// @Tags Epg
// @Accept json
// @Produce json
// @Param epg body models.Epg true "Epg"
// @Success 201 {string} status "ok"
// @Router /epg [put]
func UpdateEpg(c *fiber.Ctx) error {
	ctx := context.Background()
	// Create new Epg struct
	epg := &models.Epg{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(epg); err != nil {
		// Return status 400 and error message.
		log.Error().Msgf("UpdateEpg Error: %v", err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create a new validator for a Epg model.
	validate := utils.NewValidator()

	// Validate epg fields.
	if err := validate.Struct(epg); err != nil {
		log.Error().Msgf("UpdateEpg Error: %v", err)
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Checking, if epg with given ID is exists.
	foundEpg, err := database.Db.GetEpg(ctx, epg.ID)
	if err != nil {
		log.Error().Msgf("UpdateEpg Error: %v", err)
		// Return status 404 and epg not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "epg with this ID not found",
		})
	}

	// Update epg by given ID.
	if err := database.Db.UpdateEpg(ctx, foundEpg.ID, epg); err != nil {
		log.Error().Msgf("UpdateEpg Error: %v", err)
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if epg.URL != foundEpg.URL {
		go utils.ParseEpg(epg)
	}

	// Return status 201.
	return c.SendStatus(fiber.StatusCreated)
}

// DeleteEpg func to delete a epg by given ID.
// @Description Delete epg by given ID.
// @Summary delete epg by given ID
// @Tags Epg
// @Param id body string true "Epg ID"
// @Success 204 {string} status "ok"
// @Router /epg/{epg_id} [delete]
func DeleteEpg(c *fiber.Ctx) error {
	ctx := context.Background()
	// Catch epg ID from URL.
	epg_id, err := strconv.ParseInt(c.Params("epg_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Checking, if epg with given ID is exists.
	foundEpg, err := database.Db.GetEpg(ctx, epg_id)
	if err != nil {
		// Return status 404 and epg not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "epg with this ID not found",
		})
	}

	go utils.RemoveEpg(foundEpg)

	// Delete epg by given ID.
	if err := database.Db.DeleteEpg(ctx, foundEpg.ID); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}

// RefreshEpg func to manually refresh/update an EPG by given ID.
// @Description Manually refresh/update an EPG by given ID.
// @Summary manually refresh/update an EPG by given ID
// @Tags Epg
// @Produce json
// @Param id path string true "EPG ID"
// @Success 200 {string} status "ok"
// @Router /epg/{epg_id}/refresh [post]
func RefreshEpg(c *fiber.Ctx) error {
	ctx := context.Background()
	// Catch EPG ID from URL.
	epg_id, err := strconv.ParseInt(c.Params("epg_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get the EPG by ID
	epg, err := database.Db.GetEpg(ctx, epg_id)
	if err != nil {
		// Return status 404 and EPG not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "EPG with this ID not found",
		})
	}

	// Parse the EPG file to update the EPG
	go func() {
		utils.ParseEpg(epg)
		cron.CleanupOldEpgProgrammes()
	}()

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   "EPG refresh started",
	})
}

// GetEpgTvgids
// @Description Get all epgchannel tvgids
// @Summary get all epgchannel tvgids
// @Tags Epg
// @Produce json
// @Success 200 {array} string
// @Router /epg/tvgids [get]
func GetEpgTvgids(c *fiber.Ctx) error {
	ctx := context.Background()
	tvgids, err := database.Db.GetEpgTvgids(ctx)
	if err != nil {
		log.Warn().Msgf("GetEpgTvgids: %v", err)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":  true,
			"msg":    "No Epg tvgids found",
			"tvgids": nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":  false,
		"msg":    nil,
		"tvgids": tvgids,
	})
}
