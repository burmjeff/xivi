package controllers

import (
	"context"
	"strconv"

	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
)

// GetLogos func gets all logos.
// @Description Get all logos.
// @Summary get all logos
// @Tags Logo
// @Accept json
// @Produce json
// @Success 200 {array} models.Logo
// @Router /logos [get]
func GetLogos(c *fiber.Ctx) error {
	ctx := context.Background()
	// Get all logos.
	logos, err := database.Db.GetLogos(ctx)
	if err != nil {
		// Return, if logos not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No logos found",
			"count": 0,
			"logos": nil,
		})
	}

	for i, logo := range *logos {
		url := utils.GetLogoUrl(logo.Name)
		(*logos)[i].Image = url
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"count": len(*logos),
		"logos": logos,
	})
}

// GetLogo func gets logo by given ID or 404 error.
// @Description Get logo by given ID.
// @Summary get logo by given ID
// @Tags Logo
// @Accept json
// @Produce json
// @Param logoid path string true "Logo ID"
// @Success 200 {object} models.Logo
// @Router /logo/{logoid} [get]
func GetLogo(c *fiber.Ctx) error {
	ctx := context.Background()
	// Catch logo ID from URL.
	logoid, err := strconv.ParseInt(c.Params("logoid"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get logo.
	logo, err := database.Db.GetLogo(ctx, logoid)
	if err != nil {
		// Return, if logos not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No logo found",
			"count": 0,
			"logos": nil,
		})
	}

	logoPath := models.Logo{
		ID:    logo.ID,
		Name:  logo.Name,
		Image: utils.GetLogoUrl(logo.Name),
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"logo":  logoPath,
	})
}

// UploadLogo func for creating a new logo.
// @Summary Upload a new logo
// @Description Upload a new logo.
// @Tags Logo
// @Accept json
// @Produce json
// @Param logo body models.Logo true "Logo"
// @Success 200 {object} models.Logo
// @Router /logo [post]
func UploadLogo(c *fiber.Ctx) error {
	ctx := context.Background()
	// Create new Logo struct
	logo := &models.Logo{}

	// Check, if received JSON data is valid.
	if err := decodeStrict(c, logo); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	logo.ID = 0

	// Create a new validator for a Logo model.
	validate := utils.NewValidator()

	// Validate logo fields.
	if err := validate.Struct(logo); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Create logo.
	logoid, err := utils.UploadLogo(logo)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get logo.
	foundLogo, err := database.Db.GetLogo(ctx, logoid)
	if err != nil {
		// Return, if logos not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No logo found",
			"count": 0,
			"logos": nil,
		})
	}

	foundLogo.Image = utils.GetLogoUrl(foundLogo.Name)

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"logo":  foundLogo,
	})
}

// UpdateLogo func for updates logo by given ID.
// @Description Update logo.
// @Summary update logo
// @Tags Logo
// @Accept json
// @Produce json
// @Param id body string true "Logo ID"
// @Param name body string true "Name"
// @Param url body string true "URL"
// @Success 201 {string} status "ok"
// @Router /logo [put]
func UpdateLogo(c *fiber.Ctx) error {
	ctx := context.Background()
	// Create new Logo struct
	logo := &models.Logo{}

	// Check, if received JSON data is valid.
	if err := decodeStrict(c, logo); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Checking, if logo with given ID is exists.
	foundLogo, err := database.Db.GetLogo(ctx, logo.ID)
	if err != nil {
		// Return status 404 and logo not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "logo with this ID not found",
		})
	}

	// Create a new validator for a Logo model.
	validate := utils.NewValidator()

	// Validate logo fields.
	if err := validate.Struct(logo); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Update logo by given ID.
	if err := database.Db.UpdateLogo(ctx, foundLogo.ID, logo); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 201.
	return c.SendStatus(fiber.StatusCreated)
}

// DeleteLogo func to delete a logo by given ID.
// @Description Delete a logo by given ID.
// @Summary delete a logo by given ID
// @Tags Logo
// @Accept json
// @Produce json
// @Param id body string true "Logo ID"
// @Success 204 {string} status "ok"
// @Router /logo [delete]
func DeleteLogo(c *fiber.Ctx) error {
	ctx := context.Background()

	// Create new Logo struct
	logo := &models.Logo{}

	// Check, if received JSON data is valid.
	if err := decodeStrict(c, logo); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create a new validator for a Logo model.
	validate := utils.NewValidator()

	// Validate only one logo field ID.
	if err := validate.StructPartial(logo, "id"); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Checking, if logo with given ID is exists.
	foundedLogo, err := database.Db.GetLogo(ctx, logo.ID)
	if err != nil {
		// Return status 404 and logo not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "logo with this ID not found",
		})
	}

	// Delete logo by given ID.
	if err := database.Db.DeleteLogo(ctx, foundedLogo.ID); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}
