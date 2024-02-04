package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/streaming"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

type AppSettings struct {
	Proxy      bool `json:"proxy,omitempty"`
	Buffer     bool `json:"buffer,omitempty"`
	BufferTime int  `json:"buffertime,omitempty"`
}

// GetStream func gets stream.
// @Description Get stream by given UUID.
// @Summary get stream by given UUID
// @Tags Stream
// @Accept json
// @Produce octet-stream
// @Param stream_id path string true "Stream ID"
// @Router /stream/{stream_id} [get]
func GetStream(c *fiber.Ctx) error {
	// Catch stream ID from URL.
	stream_id := c.Params("stream_id")
	if stream_id == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Empty Stream UUID",
		})
	}

	// Get channels by UUID.
	channels, err := database.Db.GetChannelsbyUuid(stream_id)
	if err != nil {
		// Return, if no channels found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":    true,
			"msg":      "Stream Error",
			"channels": nil,
		})
	}

	if len(*channels) > 0 {
		switch settings.APP_SETTINGS.Streaming.Proxy {

		case false:
			//TODO LOOP CHECK STREAM STATUS UNTIL 302
			return c.Redirect((*channels)[0].Url, http.StatusTemporaryRedirect)

		case true:
			for _, stream := range streaming.Streams {
				if stream.Settings.Src == (*channels)[0].Url {
					if err := stream.NewSink(c); err != nil {
						return err
					}
					return nil
				}

			}

			s := streaming.NewStreamer()
			if err := s.StartStream((*channels)[0]); err != nil {
				return err
			}
			streaming.AddStream(s)

			if err := s.NewSink(c); err != nil {
				s.Close(nil, nil)
				return err
			}

		}
	} else {
		// Return, if no channels found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":    true,
			"msg":      "Stream Error",
			"channels": nil,
		})
	}
	return nil
}

// GetChannels func gets live channels by group.
// @Description Get live channels by group.
// @Summary get live channels by group.
// @Tags Stream
// @Accept json
// @Produce json
// @Param group_id path string true "Group ID"
// @Router /live/channels/{group_id} [get]
func GetChannels(c *fiber.Ctx) error {
	group_id, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   err.Error(),
		})
	}

	// Get Template channels by group.
	tmplChannels, err := database.Db.GetTmplChannelsByGroup(group_id)
	if err != nil {
		// Return, if tchannels not found.
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":    true,
			"msg":      "No channels found",
			"channels": nil,
		})
	}

	channels := []models.LiveChannel{}
	for _, tmplChannel := range *tmplChannels {
		channel := models.LiveChannel{}
		channel.ID = tmplChannel.ID
		channel.Name = tmplChannel.Name

		// Get logo.
		logo, err := database.Db.GetLogo(tmplChannel.LogoId)
		if err != nil {
			log.Warn().Msgf("LiveChannel: No Logo found")
		}
		channel.Logo = utils.GetLogoUrl(logo.Name)

		channel.Stream = fmt.Sprintf("http://%s:%d/stream/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, tmplChannel.Uuid)

		if epgProgramme, err := database.Db.GetProgrammeByTime(tmplChannel.TvgID, time.Now()); err != nil {
			log.Warn().Msgf("LiveChannel: No EPG Programme found")
		} else {
			channel.Programme = epgProgramme.Title.Value
			channel.Start = epgProgramme.Start.String()
			channel.End = epgProgramme.Stop.String()

			epgProgrammeNext, err := database.Db.GetProgrammeByTime(tmplChannel.TvgID, epgProgramme.Start.Time)
			if err != nil {
				log.Warn().Msgf("LiveChannel: No EPG Next Programme found")
			}
			channel.Next = epgProgrammeNext.Title.Value
		}
		channels = append(channels, channel)
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"channels": channels,
	})
}
