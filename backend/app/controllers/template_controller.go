package controllers

import (
	"strconv"
	"time"

	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
)

// GetTemplates func gets all existing templates.
// @Description Get all existing templates.
// @Summary get all existing templates
// @Tags Templates
// @Accept json
// @Produce json
// @Success 200 {array} models.Template
// @Router /templates [get]

func GetTemplates(c *fiber.Ctx) error {
	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get all templates.
	templates, err := db.GetTemplates()
	if err != nil {
		// Return, if templates not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":     true,
			"msg":       "No templates found",
			"count":     0,
			"templates": nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":     false,
		"msg":       nil,
		"count":     len(templates),
		"templates": templates,
	})
}

// GetTemplate func gets template by given ID or 404 error.
// @Description Get template by given ID.
// @Summary get template by given ID
// @Tags Template
// @Accept json
// @Produce json
// @Param id path string true "Template ID"
// @Success 200 {object} models.Template
// @Router /template/{id} [get]
func GetTemplate(c *fiber.Ctx) error {
	// Catch template ID from URL.
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
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

	// Get template by ID.
	template, err := db.GetTemplate(id)
	if err != nil {
		// Return, if template not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":    true,
			"msg":      "template with the given ID is not found",
			"template": nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"template": template,
	})
}

// CreateTemplate func for creating a new template.
// @Summary Create a new template
// @Description Create a new template.
// @Tags Template
// @Accept json
// @Produce json
// @Param template body models.TemplateCreateParam true "Template"
// @Success 200 {object} models.Template
// @Security ApiKeyAuth
// @Router /template [post]
func CreateTemplate(c *fiber.Ctx) error {
	// Get now time.
	/*
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

		// Set expiration time from JWT data of current template.
		expires := claims.Expires

		// Checking, if now time greather than expiration from JWT.
		if now > expires {
			// Return status 401 and unauthorized error message.
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": true,
				"msg":   "unauthorized, check expiration time of your token",
			})
		}
	*/

	// Create new Template struct
	template := &models.Template{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(template); err != nil {
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

	// Create a new validator for a Template model.
	validate := utils.NewValidator()

	// Validate template fields.
	if err := validate.Struct(template); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Create template.
	if err := db.CreateTemplate(template); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"template": template,
	})
}

// UpdateTemplate func for updates template by given ID.
// @Description Update template.
// @Summary update template
// @Tags Template
// @Accept json
// @Produce json
// @Param id body string true "Template ID"
// @Param name body string true "Name"
// @Param url body string true "URL"
// @Success 201 {string} status "ok"
// @Security ApiKeyAuth
// @Router /template [put]
func UpdateTemplate(c *fiber.Ctx) error {
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

	// Set expiration time from JWT data of current template.
	expires := claims.Expires

	// Checking, if now time greather than expiration from JWT.
	if now > expires {
		// Return status 401 and unauthorized error message.
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "unauthorized, check expiration time of your token",
		})
	}

	// Create new Template struct
	template := &models.Template{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(template); err != nil {
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

	// Checking, if template with given ID is exists.
	foundTemplate, err := db.GetTemplate(template.ID)
	if err != nil {
		// Return status 404 and template not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "template with this ID not found",
		})
	}

	// Create a new validator for a Template model.
	validate := utils.NewValidator()

	// Validate template fields.
	if err := validate.Struct(template); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Update template by given ID.
	if err := db.UpdateTemplate(foundTemplate.ID, template); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 201.
	return c.SendStatus(fiber.StatusCreated)
}

// DeleteTemplate func to delete a template by given ID.
// @Description Delete a template by given ID.
// @Summary delete a template by given ID
// @Tags Template
// @Accept json
// @Produce json
// @Param id body string true "Template ID"
// @Success 204 {string} status "ok"
// @Security ApiKeyAuth
// @Router /template [delete]
func DeleteTemplate(c *fiber.Ctx) error {
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

	// Set expiration time from JWT data of current template.
	expires := claims.Expires

	// Checking, if now time greather than expiration from JWT.
	if now > expires {
		// Return status 401 and unauthorized error message.
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true,
			"msg":   "unauthorized, check expiration time of your token",
		})
	}

	// Create new Template struct
	template := &models.Template{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(template); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create a new validator for a Template model.
	validate := utils.NewValidator()

	// Validate only one template field ID.
	if err := validate.StructPartial(template, "id"); err != nil {
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

	// Checking, if template with given ID is exists.
	foundedTemplate, err := db.GetTemplate(template.ID)
	if err != nil {
		// Return status 404 and template not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "template with this ID not found",
		})
	}

	// Delete template by given ID.
	if err := db.DeleteTemplate(foundedTemplate.ID); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}

// DeleteTemplateItem func to delete a template Item by given ID.
// @Description Delete template item by given ID.
// @Summary delete template by given ID
// @Tags Template item
// @Accept json
// @Produce json
// @Param id path string true "Template Item ID"
// @Success 204 {string} status "ok"
// @Router /template/item/{id} [delete]
func DeleteTemplateItem(c *fiber.Ctx) error {

	// Catch templateItem ID from URL.
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
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

	// Checking, if template with given ID is exists.
	foundTemplateItem, err := db.GetTmplItem(id)
	if err != nil {
		// Return status 404 and template not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "template item with this ID not found",
		})
	}

	// Delete template by given ID.
	if err := db.DeleteTmplItem(foundTemplateItem.ID); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	m3uTools := utils.M3uTools{Db: db}
	go m3uTools.RemoveM3uItems(foundTemplateItem)

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}
