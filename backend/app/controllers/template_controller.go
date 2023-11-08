package controllers

import (
	"strconv"

	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
)

// GetTemplates func gets all templates.
// @Description Get all templates.
// @Summary get all templates
// @Tags Template
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
// @Router /template [post]
func CreateTemplate(c *fiber.Ctx) error {
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
	id, err := db.CreateTemplate(template)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	template.ID = id

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"template": template,
	})
}

// UpdateTemplate func to update a template.
// @Description Update template.
// @Summary update template
// @Tags Template
// @Accept json
// @Produce json
// @Param template body models.Template true "Template"
// @Success 201 {string} status "ok"
// @Router /template [put]
func UpdateTemplate(c *fiber.Ctx) error {

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

	// Validate template fields.
	if err := validate.Struct(template); err != nil {
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
	oldTemplate, err := db.GetTemplate(template.ID)
	if err != nil {
		// Return status 404 and template not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "template with this ID not found",
		})
	}
	m3uTools := utils.M3uTools{Db: db}
	go m3uTools.UpdateTemplate(template, oldTemplate.Name)

	// Update template by given ID.
	if err := db.UpdateTemplate(template); err != nil {
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
// @Param id body string true "Template ID"
// @Success 204 {string} status "ok"
// @Router /template [delete]
func DeleteTemplate(c *fiber.Ctx) error {

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

	m3uTools := utils.M3uTools{Db: db}

	// Get template
	template, err := db.GetTemplate(template_id)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	go m3uTools.RemoveTemplate(&template)

	// Delete template by given ID.
	if err := db.DeleteTemplate(template_id); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

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

// GetGroups func gets all template groups.
// @Description Get all template groups
// @Summary get all template groups
// @Tags Template Group
// @Accept json
// @Produce json
// @Success 200 {array} models.TemplateGroup
// @Router /template/groups/all [get]
func GetGroups(c *fiber.Ctx) error {
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
// @Tags Template Group
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

	channelLogos := []models.TemplateChannelLogo{}
	for _, channel := range channels {
		channelLogo := models.TemplateChannelLogo{}
		channelLogo.TemplateChannel = channel
		// Get logo.
		logo, err := db.GetLogo(channel.LogoId)
		if err != nil {
			continue
		}
		channelLogo.Logo = utils.GetLogoUrl(logo.Uuid)
		channelLogos = append(channelLogos, channelLogo)
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":            false,
		"msg":              nil,
		"templatechannels": channelLogos,
	})
}

// CreateTemplateGroup func for creating a new template group.
// @Summary Create a new template group
// @Description Create a new template group.
// @Tags Template Group
// @Accept json
// @Produce json
// @Param templategroup body models.TemplateGroupCreateParam true "Template Group"
// @Success 200 {object} models.TemplateGroup
// @Router /template/group [post]
func CreateTemplateGroup(c *fiber.Ctx) error {
	// Create new Template struct
	templateGroup := &models.TemplateGroup{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(templateGroup); err != nil {
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
	if err := validate.Struct(templateGroup); err != nil {
		// Return, if some fields are not valid.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   utils.ValidatorErrors(err),
		})
	}

	// Create template.
	id, err := db.CreateTmplGroup(templateGroup)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	templateGroup.ID = id

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":         false,
		"msg":           nil,
		"templategroup": templateGroup,
	})
}

// CreateTemplateGroupItem func to create a template Group Item by given group id and template id.
// @Description Create template Group item by given ID.
// @Summary Create template Group item by given ID
// @Tags Template Group item
// @Accept json
// @Produce json
// @Param template_id path string true "Template ID"
// @Param group_id path string true "Group ID"
// @Success 200 {object} models.TemplateGroupItem
// @Router /template/{template_id}/group/{group_id}/item [post]
func CreateTemplateGroupItem(c *fiber.Ctx) error {

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
	if err := db.CreateTmplGroupItem(templateGroupItem); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	template, err := db.GetTemplate(template_id)
	if err != nil {
		log.Error("Failed to find template: %s", err)
	} else {
		m3uTools := utils.M3uTools{Db: db}
		go m3uTools.CreateM3u(template)
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":             false,
		"msg":               nil,
		"templategroupitem": templateGroupItem,
	})
}

// DeleteTemplateGroupItem func to delete a template Group Item by given group id and template id.
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

	templateGroupItem := &models.TemplateGroupItem{TemplateId: template_id, GroupId: group_id}

	// Get template
	template, err := db.GetTemplate(templateGroupItem.TemplateId)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get Group
	group, err := db.GetTmplGroup(templateGroupItem.GroupId)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Delete template by given ID.
	if err := db.DeleteTmplGroupItem(templateGroupItem); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	m3uTools := utils.M3uTools{Db: db}
	go m3uTools.RemoveGroup(&template, &group)

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}

// DeleteTemplateGroup func to delete a template Group by given group id.
// @Description Delete template Group by given ID.
// @Summary delete template Group by given ID
// @Tags Template Group
// @Accept json
// @Produce json
// @Param group_id path string true "Group ID"
// @Success 204 {string} status "ok"
// @Router /template/group/{group_id} [delete]
func DeleteTemplateGroup(c *fiber.Ctx) error {

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

	if templateGroupItems, err := db.GetTmplGroupItems(group_id); err == nil {
		m3uTools := utils.M3uTools{Db: db}
		for _, templateGroupItem := range templateGroupItems {
			// Get template
			template, err := db.GetTemplate(templateGroupItem.TemplateId)
			if err != nil {
				// Return status 500 and error message.
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": true,
					"msg":   err.Error(),
				})
			}

			// Get Group
			group, err := db.GetTmplGroup(templateGroupItem.GroupId)
			if err != nil {
				// Return status 500 and error message.
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": true,
					"msg":   err.Error(),
				})
			}
			go m3uTools.RemoveGroup(&template, &group)
		}
	}

	// Delete template group by given ID.
	if err := db.DeleteTmplGroup(group_id); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}

// UpdateTemplateGroup func to update a template group.
// @Description Update template Group.
// @Summary update template Group
// @Tags Template Group
// @Accept json
// @Produce json
// @Param templategroup body models.TemplateGroup true "Template group"
// @Success 201 {string} status "ok"
// @Router /template/group [put]
func UpdateTemplateGroup(c *fiber.Ctx) error {

	templateGroup := &models.TemplateGroup{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(templateGroup); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create a new validator for a Template model.
	validate := utils.NewValidator()

	// Validate template fields.
	if err := validate.Struct(templateGroup); err != nil {
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

	if templateGroupItems, err := db.GetTmplGroupItems(templateGroup.ID); err == nil {
		oldGroup, err := db.GetTmplGroup(templateGroup.ID)
		if err != nil {
			// Return status 500 and error message.
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   err.Error(),
			})
		}
		m3uTools := utils.M3uTools{Db: db}
		go func() {
			for _, templateGroupItem := range templateGroupItems {
				// Get template
				template, err := db.GetTemplate(templateGroupItem.TemplateId)
				if err != nil {
					// Return status 500 and error message.
					continue
				}
				go m3uTools.UpdateGroup(&template, templateGroup, oldGroup.Name)
			}
		}()
	}

	// Update template group.
	if err := db.UpdateTmplGroup(templateGroup); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 201.
	return c.SendStatus(fiber.StatusCreated)
}

// CreateTemplateChannel func to create a template channel.
// @Description Create a template channel.
// @Summary create template channel
// @Tags Template Channel
// @Accept json
// @Produce json
// @Param templatechannel body models.TemplateChannel true "Template channel"
// @Param group_id path string true "Group ID"
// @Success 200 {object} models.TemplateChannel
// @Router /template/group/{group_id}/channel [post]
func CreateTemplateChannel(c *fiber.Ctx) error {

	// Catch group ID from URL.
	group_id, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	//templateChannelCreate := &models.TemplateChannelCreateParam{}
	templateChannel := &models.TemplateChannel{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(templateChannel); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	templateChannel.ID = 0
	templateChannel.Uuid = utils.CreateUuid()

	// Create a new validator.
	validate := utils.NewValidator()

	// Validate fields.
	if err := validate.Struct(templateChannel); err != nil {
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

	// Create template channel.
	id, err := db.CreateTmplChannel(templateChannel)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	templateChannel.ID = id
	templategroupchannel := &models.TemplateGroupChannel{
		GroupId:   group_id,
		ChannelId: templateChannel.ID,
	}

	// Create template channelgroup.
	if err := db.CreateTmplGroupChannel(templategroupchannel); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	m3uTools := utils.M3uTools{Db: db}
	go m3uTools.AddChannel(templateChannel, group_id)

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":           false,
		"msg":             nil,
		"templatechannel": templateChannel,
	})
}

// UpdateTemplateChannel func to update a template channel.
// @Description Update template channel.
// @Summary update template channel
// @Tags Template Channel
// @Accept json
// @Produce json
// @Param templatechannel body models.TemplateChannel true "Template channel"
// @Success 201 {string} status "ok"
// @Router /template/channel [put]
func UpdateTemplateChannel(c *fiber.Ctx) error {

	templateChannelLogo := &models.TemplateChannelLogo{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(templateChannelLogo); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Create a new validator for a Template model.
	validate := utils.NewValidator()

	// Validate template fields.
	if err := validate.Struct(templateChannelLogo); err != nil {
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

	if templateGroupChannels, err := db.GetTmplGroupChannelsByChannel(templateChannelLogo.ID); err == nil {
		oldChannel, err := db.GetTmplChannel(templateChannelLogo.ID)
		if err != nil {
			// Return status 500 and error message.
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   err.Error(),
			})
		}
		go func() {
			for _, item := range templateGroupChannels {
				if templateGroupItems, err := db.GetTmplGroupItems(item.GroupId); err == nil {
					m3uTools := utils.M3uTools{Db: db}
					for _, templateGroupItem := range templateGroupItems {
						// Get template
						template, err := db.GetTemplate(templateGroupItem.TemplateId)
						if err != nil {
							continue
						}
						m3uTools.UpdateChannel(&template, templateChannelLogo, &oldChannel)
					}
				}
			}
		}()
	}

	// Update template channel.
	if err := db.UpdateTmplChannel(templateChannelLogo); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 201.
	return c.SendStatus(fiber.StatusCreated)
}

// DeleteTemplateChannel func to delete a template Channel by given Channel id.
// @Description Delete template Channel by given ID.
// @Summary delete template Channel by given ID
// @Tags Template Channel
// @Param channel_id path string true "Channel ID"
// @Success 204 {string} status "ok"
// @Router /template/channel/{channel_id} [delete]
func DeleteTemplateChannel(c *fiber.Ctx) error {

	// Catch channel ID from URL.
	channel_id, err := strconv.ParseInt(c.Params("channel_id"), 10, 64)
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

	if templateGroupChannels, err := db.GetTmplGroupChannelsByChannel(channel_id); err == nil {
		channel, err := db.GetTmplChannel(channel_id)
		if err != nil {
			// Return status 500 and error message.
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   err.Error(),
			})
		}

		go func() {
			for _, item := range templateGroupChannels {
				if templateGroupItems, err := db.GetTmplGroupItems(item.GroupId); err == nil {
					group, err := db.GetTmplGroup(item.GroupId)
					if err != nil {
						continue
					}
					m3uTools := utils.M3uTools{Db: db}
					for _, templateGroupItem := range templateGroupItems {
						// Get template
						template, err := db.GetTemplate(templateGroupItem.TemplateId)
						if err != nil {
							continue
						}
						m3uTools.RemoveChannel(&template, &group, &channel)
					}
				}
			}
		}()
	}

	// Delete template group by given ID.
	if err := db.DeleteTmplChannel(channel_id); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}
