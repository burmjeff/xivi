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
// @Param template_id path string true "Template ID"
// @Success 200 {object} models.Template
// @Router /template/{template_id} [get]
func GetTemplate(c *fiber.Ctx) error {
	// Catch template ID from URL.
	template_id, err := strconv.ParseInt(c.Params("template_id"), 10, 64)
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
	template, err := db.GetTemplate(template_id)
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

// DeleteTemplateGroupItem func to delete a template Group Item by given group id and templatet id.
// @Description Delete template Group item by given ID.
// @Summary delete template Group item by given ID
// @Tags Template Group item
// @Accept json
// @Produce json
// @Param template_id path string true "Template ID"
// @Param group_id path string true "Group ID"
// @Success 204 {string} status "ok"
// @Router /template/{template_id}/group/{group_id}/item [delete]
func DeleteTemplateGroupItem(c *fiber.Ctx) error {

	// Catch template ID from URL.
	template_id, err := strconv.ParseInt(c.Params("template_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Catch group ID from URL.
	group_id, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
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

	// Delete template by given ID.
	templateGroupItem := &models.TemplateGroupItem{TemplateId: template_id, GroupId: group_id}
	if err := db.DeleteTmplGroupItem(templateGroupItem); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	m3uTools := utils.M3uTools{Db: db}
	go m3uTools.RemoveM3uItems(templateGroupItem)

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}

// GetTemplateGroups func gets template groups by given template ID or 404 error.
// @Description Get template groups by given template ID
// @Summary get template groups by given template ID
// @Tags Template
// @Accept json
// @Produce json
// @Param template_id path string true "Template ID"
// @Success 200 {array} models.TemplateGroup
// @Router /template/{template_id}/groups [get]
func GetTemplateGroups(c *fiber.Ctx) error {
	// Catch template ID from URL.
	template_id, err := strconv.ParseInt(c.Params("template_id"), 10, 64)
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

	// Get Template groups by Template.
	templateGroupItem := &models.TemplateGroupItem{TemplateId: template_id}
	groups, err := db.GetTmplGroups(templateGroupItem)
	if err != nil {
		// Return, if template not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":          true,
			"msg":            "template groups not found",
			"templategroups": nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":          false,
		"msg":            nil,
		"templategroups": groups,
	})
}

// GetAllTemplateGroups func gets all template groups.
// @Description Get all template group
// @Summary get all template groups
// @Tags Template
// @Accept json
// @Produce json
// @Success 200 {array} models.TemplateGroup
// @Router /template/groups/all [get]
func GetAllTemplateGroups(c *fiber.Ctx) error {
	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		// Return status 500 and database connection error.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get Template groups.
	groups, err := db.GetAllTmplGroups()
	if err != nil {
		// Return, if template not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":          true,
			"msg":            "template groups not found",
			"templategroups": nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":          false,
		"msg":            nil,
		"templategroups": groups,
	})
}

// GetTemplateChannels func gets template channels by given group ID or 404 error.
// @Description Get template channels by given group ID
// @Summary get template channels by given group ID
// @Tags Template
// @Accept json
// @Produce json
// @Param group_id path string true "Group ID"
// @Success 200 {array} models.TemplateChannel
// @Router /template/group/{group_id}/channels [get]
func GetTemplateGroupChannels(c *fiber.Ctx) error {
	// Catch group ID from URL.
	group_id, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
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

	// Get Template channels by group.
	templateGroupChannel := &models.TemplateGroupChannel{GroupId: group_id}
	channels, err := db.GetTmplChannelsByGroup(templateGroupChannel)
	if err != nil {
		// Return, if template not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":            true,
			"msg":              "template channels not found",
			"templatechannels": nil,
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":            false,
		"msg":              nil,
		"templatechannels": channels,
	})
}
