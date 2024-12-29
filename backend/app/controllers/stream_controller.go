package controllers

import (
	"fmt"
	"net/http"
	"os"
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

var isRunning bool = false

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
	if isRunning {
		raceCheck()
	}
	isRunning = true

	// Catch stream ID from URL.
	stream_id := c.Params("stream_id")
	if stream_id == "" {
		log.Error().Msg("No stream ID found")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Empty Stream UUID",
		})
	}

	// Get channels by UUID.
	channels, err := database.Db.GetChannelsbyUuid(stream_id)
	if err != nil {
		log.Error().Msgf("No stream channels found: %v", err)
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
			found := false
			for _, stream := range streaming.Streams {
				if stream.Settings.Uuid == stream_id {
					found = true
					stream.Mu.Lock()
					stream.LastAccess = time.Now()
					stream.Mu.Unlock()
					if settings.APP_SETTINGS.Streaming.Type != "hls" {
						if err := stream.NewMP2TSink(c); err != nil {
							isRunning = false
							log.Error().Msgf("FAILED TO CREATE MP2T SINK: %v", err)
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
								"error": true,
								"msg":   fmt.Sprintf("FAILED TO CREATE MP2T SINK: %v", err),
							})
						}
					} else if !stream.HlsExists {
						if err := stream.CreateHlsDir(); err != nil {
							isRunning = false
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
								"error": true,
								"msg":   err,
							})
						}
						if err := stream.NewHLSSink(c); err != nil {
							isRunning = false
							log.Error().Msgf("FAILED TO CREATE HLS SINK: %v", err)
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
								"error": true,
								"msg":   fmt.Sprintf("FAILED TO CREATE HLS SINK: %v", err),
							})
						}
					}
					isRunning = false
				}
			}
			if !found {
				s := streaming.NewStream(stream_id, (*channels)[0].Url)
				streaming.AddStream(s)

				if err := s.StartStream(c); err != nil {
					isRunning = false
					log.Error().Msgf("FAILED TO START STREAM: %v", err)
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
						"error": true,
						"msg":   err,
					})
				}
				isRunning = false
			}
		}
	} else {
		// Return, if no channels found.
		isRunning = false
		log.Error().Msgf("NO STREAMS FOR CHANNEL FOUND: %v", stream_id)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Stream Error: No streamable channels found",
		})
	}

	if settings.APP_SETTINGS.Streaming.Type == "hls" {
		exists := false
		loop := 0
		for !exists {
			if _, err := os.Stat(fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, stream_id, "playlist.m3u8")); err == nil {
				exists = true
			} else if loop >= 100 {
				log.Error().Msg("Stream Error: Playlist M3U8 not found")
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": true,
					"msg":   "Stream Error: M3U8 not found",
				})
			} else {
				time.Sleep(200 * time.Millisecond)
				loop += 1
			}
		}
		return c.SendFile(fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, stream_id, "playlist.m3u8"))
	} else {
		return nil
	}
}

// GetHlsStream func gets an HLS stream.
// @Description Get Hls stream by given UUID.
// @Summary get Hls stream by given UUID
// @Tags Stream
// @Accept json
// @Produce octet-stream
// @Param stream_id path string true "Stream ID"
// @Router /stream/hls/{stream_id} [get]
func GetHlsStream(c *fiber.Ctx) error {
	if isRunning {
		raceCheck()
	}
	isRunning = true

	// Catch stream ID from URL.
	stream_id := c.Params("stream_id")
	if stream_id == "" {
		isRunning = false
		log.Error().Msg("No stream ID found")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Empty Stream UUID",
		})
	}

	// Get channels by UUID.
	channels, err := database.Db.GetChannelsbyUuid(stream_id)
	if err != nil {
		isRunning = false
		log.Error().Msgf("No stream channels found: %v", err)
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
			found := false
			for _, stream := range streaming.Streams {
				if stream.Settings.Uuid == stream_id {
					found = true
					stream.Mu.Lock()
					stream.LastAccess = time.Now()
					stream.Mu.Unlock()
					if !stream.HlsExists {
						if err := stream.CreateHlsDir(); err != nil {
							isRunning = false
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
								"error": true,
								"msg":   err,
							})
						}
						if err := stream.NewHLSSink(c); err != nil {
							isRunning = false
							log.Error().Msgf("FAILED TO CREATE HLS SINK: %v", err)
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
								"error": true,
								"msg":   fmt.Sprintf("FAILED TO CREATE HLS SINK: %v", err),
							})
						}
					}
					isRunning = false
				}
			}
			if !found {
				s := streaming.NewStream(stream_id, (*channels)[0].Url)
				streaming.AddStream(s)
				s.HlsExists = true

				if err := s.StartStream(c); err != nil {
					isRunning = false
					log.Error().Msgf("FAILED TO START STREAM: %v", err)
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
						"error": true,
						"msg":   err,
					})
				}
				isRunning = false
			}
		}
	} else {
		isRunning = false
		// Return, if no channels found.
		log.Error().Msgf("NO STREAMS FOR CHANNEL FOUND: %v", stream_id)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Stream Error: No streamable channels found",
		})
	}

	exists := false
	loop := 0
	for !exists {
		if _, err := os.Stat(fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, stream_id, "playlist.m3u8")); err == nil {
			exists = true
		} else if loop >= 100 {
			log.Error().Msg("Stream Error: Playlist M3U8 not found")
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": true,
				"msg":   "Stream Error: M3U8 not found",
			})
		} else {
			time.Sleep(200 * time.Millisecond)
			loop += 1
		}
	}
	return c.SendFile(fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, stream_id, "playlist.m3u8"))
}

// GetChannels func gets Hls channels by group.
// @Description Get Hls channels by group.
// @Summary get Hls channels by group.
// @Tags Stream
// @Accept json
// @Produce json
// @Param group_id path string true "Group ID"
// @Router /channels/hls/{group_id} [get]
func GetHlsChannels(c *fiber.Ctx) error {
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
	for _, tmplChannel := range tmplChannels {
		if items, err := database.Db.GetTmplChannelItemsByCh(tmplChannel.ID); items != nil && len(*items) > 0 {
			channel := models.LiveChannel{}
			channel.ID = tmplChannel.ID
			channel.Name = tmplChannel.Name

			// Get logo.
			logo, err := database.Db.GetLogo(tmplChannel.LogoId)
			if err != nil {
				log.Warn().Msgf("LiveChannel: No Logo found")
			}
			channel.Logo = utils.GetLogoUrl(logo.Name)

			channel.Stream = fmt.Sprintf("http://%s:%d/stream/hls/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, tmplChannel.Uuid)

			if tmplChannel.TvgID != nil {
				if epgProgramme, err := database.Db.GetProgrammeByTime(*tmplChannel.TvgID, time.Now()); err != nil {
					log.Warn().Msgf("LiveChannel: No EPG Programme found")
				} else {
					channel.Programme = epgProgramme.Title.Value
					channel.Start = epgProgramme.Start.String()
					channel.End = epgProgramme.Stop.String()

					epgProgrammeNext, err := database.Db.GetProgrammeByTime(*tmplChannel.TvgID, epgProgramme.Start.Time)
					if err != nil {
						log.Warn().Msgf("LiveChannel: No EPG Next Programme found")
					}
					channel.Next = epgProgrammeNext.Title.Value
				}
			}
			channels = append(channels, channel)
		} else {
			log.Debug().Msgf("No Channel URL found... Not adding channel to channel list, %v", err)
		}
	}

	// Return status 200 OK.
	return c.JSON(fiber.Map{
		"error":    false,
		"msg":      nil,
		"channels": channels,
	})
}

func raceCheck() {
	checkInterval := 50 * time.Millisecond
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Debug().Msg("Waiting... Stream start race check...")
			if !isRunning {
				return
			}
		}
	}
}
