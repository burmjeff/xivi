package controllers

import (
	"strconv"

	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
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
	// Get all templates.
	templates, err := database.Db.GetTemplates()
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

	// Get template by ID.
	template, err := database.Db.GetTemplate(template_id)
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
	id, err := database.Db.CreateTemplate(template)
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

	// Checking, if template with given ID is exists.
	oldTemplate, err := database.Db.GetTemplate(template.ID)
	if err != nil {
		// Return status 404 and template not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "template with this ID not found",
		})
	}
	m3uTools := utils.M3uTools{}
	go m3uTools.RenameTemplate(template, oldTemplate.Name)

	// Update template by given ID.
	if err := database.Db.UpdateTemplate(template); err != nil {
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

	m3uTools := utils.M3uTools{}

	// Get template
	template, err := database.Db.GetTemplate(template_id)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	go m3uTools.RemoveTemplate(&template)

	// Delete template by given ID.
	if err := database.Db.DeleteTemplate(template_id); err != nil {
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

	// Get Template groups by Template.
	groups, err := database.Db.GetTmplGroups(template_id)
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

	// Get Template groups.
	groups, err := database.Db.GetAllTmplGroups()
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

	// Get Template channels by group.
	channels, err := database.Db.GetTmplChannelsByGroup(group_id)
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
		logo, err := database.Db.GetLogo(channel.LogoId)
		if err != nil {
			continue
		}
		channelLogo.Logo = utils.GetLogoUrl(logo.Name)
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
	id, err := database.Db.CreateTmplGroup(templateGroup)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	templateGroup.ID = id

	if templateGroup.Dynamic {
		go utils.UpdateDynamicGroup(*templateGroup)
	}

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

	// Create TemplateGroupItem
	templateGroupItem := &models.TemplateGroupItem{TemplateId: template_id, GroupId: group_id}
	if err := database.Db.CreateTmplGroupItem(templateGroupItem); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	m3uTools := utils.M3uTools{}
	template, err := database.Db.GetTemplate(template_id)
	if err != nil {
		log.Error().Msgf("Failed to find template: %s", err)
	} else {
		go m3uTools.CreateM3u(template)
		go utils.CreateEpgXML(template)
	}

	// Return status 201 OK.
	return c.SendStatus(fiber.StatusCreated)
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

	templateGroupItem := &models.TemplateGroupItem{TemplateId: template_id, GroupId: group_id}

	// Get template
	template, err := database.Db.GetTemplate(templateGroupItem.TemplateId)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get Group
	group, err := database.Db.GetTmplGroup(templateGroupItem.GroupId)
	if err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Delete template by given ID.
	if err := database.Db.DeleteTmplGroupItem(templateGroupItem); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	m3uTools := utils.M3uTools{}
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

	if templateGroupItems, err := database.Db.GetTmplGroupItemsByGroup(group_id); err == nil {
		for _, templateGroupItem := range templateGroupItems {
			// Get template
			template, err := database.Db.GetTemplate(templateGroupItem.TemplateId)
			if err != nil {
				// Return status 500 and error message.
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": true,
					"msg":   err.Error(),
				})
			}

			m3uTools := utils.M3uTools{}
			// Get Group
			group, err := database.Db.GetTmplGroup(templateGroupItem.GroupId)
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
	if err := database.Db.DeleteTmplGroup(group_id); err != nil {
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

	oldGroup, err := database.Db.GetTmplGroup(templateGroup.ID)
	if err != nil {
		log.Err(err)
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Update template group.
	if err := database.Db.UpdateTmplGroup(templateGroup); err != nil {
		log.Err(err)
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	if (templateGroup.Dynamic != oldGroup.Dynamic && templateGroup.Dynamic) ||
		(templateGroup.PlaylistGroup != oldGroup.PlaylistGroup && templateGroup.Dynamic) {
		utils.UpdateDynamicGroup(*templateGroup)
	}

	m3uTools := utils.M3uTools{}
	if templateGroupItems, err := database.Db.GetTmplGroupItemsByGroup(templateGroup.ID); err == nil {
		go func() {
			for _, templateGroupItem := range templateGroupItems {
				// Get template
				template, err := database.Db.GetTemplate(templateGroupItem.TemplateId)
				if err != nil {
					continue
				}
				if templateGroup.Dynamic == oldGroup.Dynamic {
					go m3uTools.UpdateGroup(&template, templateGroup, oldGroup)
				} else {
					go m3uTools.CreateM3u(template)
					go utils.CreateEpgXML(template)
				}
			}
		}()
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

	// Create template channel.
	id, err := database.Db.CreateTmplChannel(templateChannel)
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
	if err := database.Db.CreateTmplGroupChannel(templategroupchannel); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	foundLogo, err := database.Db.GetLogo(templateChannel.LogoId)
	if err != nil {
		// Return status 404 and logo not found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "logo with this ID not found",
		})
	}

	templateChannelLogo := &models.TemplateChannelLogo{
		TemplateChannel: *templateChannel,
		Logo:            utils.GetLogoUrl(foundLogo.Name),
	}

	m3uTools := utils.M3uTools{}
	go m3uTools.AddChannel(templateChannel, group_id)

	go func() {
		utils.UpdateTemplateVector(templateChannel)
		utils.MatchTemplateChannel(templateChannel)
	}()

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":           false,
		"msg":             nil,
		"templatechannel": templateChannelLogo,
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

	m3uTools := utils.M3uTools{}
	if templateGroupChannels, err := database.Db.GetTmplGroupChannelsByChannel(templateChannelLogo.ID); err == nil {
		oldChannel, err := database.Db.GetTmplChannel(templateChannelLogo.ID)
		if err != nil {
			// Return status 500 and error message.
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   err.Error(),
			})
		}
		go func() {
			for _, item := range templateGroupChannels {
				if templateGroupItems, err := database.Db.GetTmplGroupItemsByGroup(item.GroupId); err == nil {
					for _, templateGroupItem := range templateGroupItems {
						// Get template
						template, err := database.Db.GetTemplate(templateGroupItem.TemplateId)
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
	if err := database.Db.UpdateTmplChannel(&templateChannelLogo.TemplateChannel); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	go utils.UpdateTemplateVector(&templateChannelLogo.TemplateChannel)

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

	if templateGroupChannels, err := database.Db.GetTmplGroupChannelsByChannel(channel_id); err == nil {
		channel, err := database.Db.GetTmplChannel(channel_id)
		if err != nil {
			// Return status 500 and error message.
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   err.Error(),
			})
		}

		m3uTools := utils.M3uTools{}
		go func() {
			for _, item := range templateGroupChannels {
				if templateGroupItems, err := database.Db.GetTmplGroupItemsByGroup(item.GroupId); err == nil {
					group, err := database.Db.GetTmplGroup(item.GroupId)
					if err != nil {
						continue
					}
					for _, templateGroupItem := range templateGroupItems {
						// Get template
						template, err := database.Db.GetTemplate(templateGroupItem.TemplateId)
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
	if err := database.Db.DeleteTmplChannel(channel_id); err != nil {
		// Return status 500 and error message.
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}

// GetTemplateChannelItems func gets template channel items by channel ID.
// @Description Get template channel items by channel ID
// @Summary Get template channel items by channel ID
// @Tags Template Channel
// @Produce json
// @Param channel_id path string true "Channel ID"
// @Success 200 {array} models.TemplateChannel
// @Router /template/channel/{channel_id}/items [get]
func GetTemplateChannelItems(c *fiber.Ctx) error {
	// Catch group ID from URL.
	channel_id, err := strconv.ParseInt(c.Params("channel_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get Template channels by group.
	channels, err := database.Db.GetTmplChannelItemsByCh(channel_id)
	if err != nil {
		log.Err(err)
		// Return, if template not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No template channel items found",
		})
	}

	playlistChannels := []models.PlaylistChannel{}
	for _, channel := range channels {
		playlistChannel, err := database.Db.GetPlChannel(channel.PlaylistChannelId)
		if err != nil {
			continue
		}
		playlistChannels = append(playlistChannels, playlistChannel)
	}
	if len(playlistChannels) <= 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No template channel items found",
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":            false,
		"msg":              nil,
		"playlistchannels": playlistChannels,
	})
}

// DeleteTemplateChannelItems func gets template channel item.
// @Description Delete template channel item
// @Summary Delete template channel item
// @Tags Template Channel
// @Accept json
// @Param templatechannelitem body models.TemplateChannelItem true "Template Item"
// @Success 204 {string} status "ok"
// @Router /template/chan/item [delete]
func DeleteTemplateChannelItem(c *fiber.Ctx) error {
	templateItem := &models.TemplateChannelItem{}

	// Check, if received JSON data is valid.
	if err := c.BodyParser(templateItem); err != nil {
		// Return status 400 and error message.
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get Template channels by group.
	if err := database.Db.DeleteTmplChannelItem(templateItem); err != nil {
		log.Err(err)
		// Return, if template not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No template channel items found",
		})
	}

	// Return status 204 no content.
	return c.SendStatus(fiber.StatusNoContent)
}

// GetTemplateChannelMatches func gets playlist channel matches by template channel ID.
// @Description Get top 5 playlist channel matches by template channel ID.
// @Summary Get top 5 playlist channel matches by template channel ID.
// @Tags Template Channel
// @Produce json
// @Param channel_id path string true "Channel ID"
// @Success 200 {array} models.VectorMatch
// @Router /template/channel/{channel_id}/matches [get]
func GetTemplateChannelMatches(c *fiber.Ctx) error {
	// Catch group ID from URL.
	channel_id, err := strconv.ParseInt(c.Params("channel_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	channel, err := database.Db.GetTmplChannel(channel_id)
	if err != nil {
		log.Err(err)
		// Return, if template not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "No template channel found",
		})
	}

	vectorMatches, err := utils.TopChannelMatches(&channel)
	if err != nil {
		log.Err(err)
		// Return, if template not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Failed to find channel matches",
		})
	}

	for idx, vectorMatch := range vectorMatches {
		if channel, err := database.Db.GetPlChannel(vectorMatch.Id); err != nil {
			log.Err(err)
			// Return, if template not found.
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": true,
				"msg":   "Failed to find channel matches",
			})
		} else {
			vectorMatches[idx].Name = channel.Title
			vectorMatches[idx].Tvgid = channel.TvgID
		}

	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":         false,
		"msg":           nil,
		"vectormatches": vectorMatches,
	})
}

// AddChannelMatch func to match playlist channel to a template channel.
// @Description match playlist channel to a template channel.
// @Summary match playlist channel to a template channel
// @Tags Template Channel
// @Produce json
// @Param channel_id path string true "Channel ID"
// @Param match_id path string true "Match ID"
// @Success 200 {object} models.PlaylistChannel
// @Router /template/channel/${channel_id}/match/${match_id} [post]
func AddChannelMatch(c *fiber.Ctx) error {

	channel_id, err := strconv.ParseInt(c.Params("channel_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	match_id, err := strconv.ParseInt(c.Params("match_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	channelItem := models.TemplateChannelItem{
		ChannelId:         channel_id,
		PlaylistChannelId: match_id,
	}

	// Create template channel.
	if _, err := database.Db.GetTmplChannelItem(channelItem); err != nil {
		if _, err := database.Db.CreateTmplChannelItem(channelItem); err != nil {
			log.Error().Msgf("Failed to create channelItem:, %v", err)
			// Return status 500 and error message.
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": true,
				"msg":   err.Error(),
			})
		}
	}

	playlistChannel, err := database.Db.GetPlChannel(match_id)
	if err != nil {
		// Return status 404 and playlist channel snot found error.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "playlist channel with this ID not found",
		})
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":           false,
		"msg":             nil,
		"playlistchannel": playlistChannel,
	})
}
