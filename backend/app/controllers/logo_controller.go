package controllers

import (
	"strconv"
	"time"

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
	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get all logos.
	logos, err := db.GetLogos()
	if err != nil {
		// Return, if logos not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No logos found",
			"count": 0,
			"logos": nil,
		})
	}

	for _, logo := range logos {
		logo.Img = utils.GetChannelLogo(db, logo.ID)
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"count": len(logos),
		"logos": logos,
	})
}

// GetLogo func gets logo by given ID or 404 error.
// @Description Get logo by given ID.
// @Summary get logo by given ID
// @Tags Logo
// @Accept json
// @Produce json
// @Param logo_id path string true "Logo ID"
// @Success 200 {object} models.Logo
// @Router /logo/{logo_id} [get]
func GetLogo(c *fiber.Ctx) error {
	// Catch logo ID from URL.
	logo_id, err := strconv.ParseInt(c.Params("logo_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get logo by ID.
	logo, err := db.GetLogo(logo_id)
	if err != nil {
		// Return, if logo not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "logo with the given ID is not found",
			"logo":  nil,
		})
	}

	logo.Img = utils.GetChannelLogo(db, logo.ID)

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"logo":  logo,
	})
}

// CreateLogo func for creating a new logo.
// @Summary Create a new logo
// @Description Create a new logo.
// @Tags Logo
// @Accept json
// @Produce json
// @Param logo body models.LogoCreateParam true "Logo"
// @Success 200 {object} models.Logo
// @Router /logo [post]
func CreateLogo(c *fiber.Ctx) error {
	// Create new Logo struct
	logo := &models.Logo{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(logo); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
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

	// Create logo.
	if _, err := utils.CreateLogo(db, logo.Img); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error": false,
		"msg":   nil,
		"logo":  logo,
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
// @Security ApiKeyAuth
// @Router /logo [put]
func UpdateLogo(c *fiber.Ctx) error {
	// Get now time.
	now := time.Now().Unix()

	// Get claims from JWT.
	claims, err := utils.ExtractTokenMetadata(c)
	if err != nil {
		// Return status 500 and JWT parse error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Set expiration time from JWT data of current logo.
	expires := claims.Expires

	// Checking, if now time greather than expiration from JWT.
	if now > expires {
		// Return status 401 and unauthorized error message.
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "unauthorized, check expiration time of your token",
		})
	}

	// Create new Logo struct
	logo := &models.Logo{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(logo); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Checking, if logo with given ID is exists.
	foundLogo, err := db.GetLogo(logo.ID)
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
	if err := db.UpdateLogo(foundLogo.ID, logo); err != nil {
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
// @Security ApiKeyAuth
// @Router /logo [delete]
func DeleteLogo(c *fiber.Ctx) error {
	// Get now time.
	now := time.Now().Unix()

	// Get claims from JWT.
	claims, err := utils.ExtractTokenMetadata(c)
	if err != nil {
		// Return status 500 and JWT parse error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Set expiration time from JWT data of current logo.
	expires := claims.Expires

	// Checking, if now time greather than expiration from JWT.
	if now > expires {
		// Return status 401 and unauthorized error message.
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "unauthorized, check expiration time of your token",
		})
	}

	// Create new Logo struct
	logo := &models.Logo{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(logo); err != nil {
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

	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Checking, if logo with given ID is exists.
	foundedLogo, err := db.GetLogo(logo.ID)
	if err != nil {
		// Return status 404 and logo not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "logo with this ID not found",
		})
	}

	// Delete logo by given ID.
	if err := db.DeleteLogo(foundedLogo.ID); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}
