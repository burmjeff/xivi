package controllers

import (
	"strconv"
	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
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

	// Get all epgs.
	epgs, err := database.Db.GetEpgs()
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
	id, err := database.Db.CreateEpg(epg)
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

// DeleteEpg func to delete a epg by given ID.
// @Description Delete epg by given ID.
// @Summary delete epg by given ID
// @Tags Epg
// @Param id body string true "Epg ID"
// @Success 204 {string} status "ok"
// @Router /epg/{epg_id} [delete]
func DeleteEpg(c *fiber.Ctx) error {
	// Catch epg ID from URL.
	epg_id, err := strconv.ParseInt(c.Params("epg_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Checking, if epg with given ID is exists.
	foundEpg, err := database.Db.GetEpg(epg_id)
	if err != nil {
		// Return status 404 and epg not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "epg with this ID not found",
		})
	}

	go utils.RemoveEpg(foundEpg)

	// Delete epg by given ID.
	if err := database.Db.DeleteEpg(foundEpg.ID); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}
