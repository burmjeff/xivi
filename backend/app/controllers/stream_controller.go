package controllers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/streaming"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// streamLock provides thread-safe access to stream operations
var streamLock sync.Mutex

// streamOperationTimeout is the maximum time to wait for a stream operation to complete
const streamOperationTimeout = 30 * time.Second

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
	// Create a context with timeout for the entire operation
	ctx, cancel := context.WithTimeout(context.Background(), streamOperationTimeout)
	defer cancel()

	// Acquire lock to prevent concurrent stream operations
	streamLock.Lock()
	defer streamLock.Unlock()

	// Catch stream ID from URL
	stream_id := c.Params("stream_id")
	if stream_id == "" {
		log.Error().Msg("No stream ID found")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Empty Stream UUID",
		})
	}

	// Get channels by UUID.
	channels, err := database.Db.GetChannelsbyUuid(ctx, stream_id)
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
							log.Error().Msgf("Failed to create MP2T sink: %v", err)
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
								"error": true,
								"msg":   fmt.Sprintf("Failed to create MP2T sink: %v", err),
							})
						}
					} else if !stream.HlsExists {
						if err := stream.CreateHlsDir(); err != nil {
							log.Error().Msgf("Failed to create HLS directory: %v", err)
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
								"error": true,
								"msg":   err,
							})
						}
						if err := stream.NewHLSSink(c); err != nil {
							log.Error().Msgf("Failed to create HLS sink: %v", err)
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
								"error": true,
								"msg":   fmt.Sprintf("Failed to create HLS sink: %v", err),
							})
						}
					}
				}
			}
			if !found {
				s := streaming.NewStream(stream_id, (*channels)[0].Url)
				streaming.AddStream(s)

				if err := s.StartStream(c); err != nil {
					log.Error().Msgf("Failed to start stream: %v", err)
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
						"error": true,
						"msg":   err,
					})
				}
			}
		}
	} else {
		// Return, if no channels found.
		log.Error().Msgf("No streams found for channel: %v", stream_id)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Stream Error: No streamable channels found",
		})
	}

	if settings.APP_SETTINGS.Streaming.Type == "hls" {
		// Use the optimized waitForPlaylist function instead of manual polling
		playlistPath := fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, stream_id, "playlist.m3u8")
		playlistTimeout := 10 * time.Second // Reduced timeout for faster response

		if !waitForPlaylist(playlistPath, playlistTimeout) {
			log.Error().Msgf("HLS playlist not found after waiting: %s", playlistPath)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": true,
				"msg":   "Stream Error: HLS playlist not found after timeout",
			})
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
	// Create a context with timeout for the entire operation
	ctx, cancel := context.WithTimeout(context.Background(), streamOperationTimeout)
	defer cancel()

	// Acquire lock to prevent concurrent stream operations
	streamLock.Lock()
	defer streamLock.Unlock()

	// Catch stream ID from URL
	stream_id := c.Params("stream_id")
	if stream_id == "" {
		log.Error().Msg("No stream ID found")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true,
			"msg":   "Empty Stream UUID",
		})
	}

	// Get channels by UUID.
	channels, err := database.Db.GetChannelsbyUuid(ctx, stream_id)
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
					if !stream.HlsExists {
						if err := stream.CreateHlsDir(); err != nil {
							log.Error().Msgf("Failed to create HLS directory: %v", err)
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
								"error": true,
								"msg":   err,
							})
						}
						if err := stream.NewHLSSink(c); err != nil {
							log.Error().Msgf("Failed to create HLS sink: %v", err)
							return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
								"error": true,
								"msg":   fmt.Sprintf("Failed to create HLS sink: %v", err),
							})
						}
					}
				}
			}
			if !found {
				s := streaming.NewStream(stream_id, (*channels)[0].Url)
				streaming.AddStream(s)
				s.HlsExists = true

				if err := s.StartStream(c); err != nil {
					log.Error().Msgf("Failed to start stream: %v", err)
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
						"error": true,
						"msg":   err,
					})
				}
			}
		}
	} else {
		// Return, if no channels found.
		log.Error().Msgf("No streams found for channel: %v", stream_id)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Stream Error: No streamable channels found",
		})
	}

	// Wait for the HLS playlist file to be created with a timeout
	playlistPath := fmt.Sprintf("%s/%s/%s", settings.STREAM_FILEPATH, stream_id, "playlist.m3u8")
	playlistTimeout := 10 * time.Second // Reduced timeout for faster response

	if !waitForPlaylist(playlistPath, playlistTimeout) {
		log.Error().Msgf("HLS playlist not found after waiting: %s", playlistPath)
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true,
			"msg":   "Stream Error: HLS playlist not found after timeout",
		})
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
	ctx := context.Background()
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
			logo, err := database.Db.GetLogo(ctx, tmplChannel.LogoId)
			if err != nil {
				log.Warn().Msgf("LiveChannel: No Logo found")
			}
			channel.Logo = utils.GetLogoUrl(logo.Name)

			channel.Stream = fmt.Sprintf("http://%s:%d/stream/hls/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, tmplChannel.Uuid)

			if tmplChannel.TvgID != nil {
				if epgProgramme, err := database.Db.GetProgrammeByTime(ctx, *tmplChannel.TvgID, time.Now()); err != nil {
					log.Warn().Msgf("LiveChannel: No EPG Programme found")
				} else {
					channel.Programme = epgProgramme.Title.Value
					channel.Start = epgProgramme.Start.String()
					channel.End = epgProgramme.Stop.String()

					epgProgrammeNext, err := database.Db.GetProgrammeByTime(ctx, *tmplChannel.TvgID, epgProgramme.Start.Time)
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

// waitForPlaylist waits for the HLS playlist file to be created
func waitForPlaylist(playlistPath string, maxWaitTime time.Duration) bool {
	// Use a more aggressive polling interval for faster detection
	checkInterval := 50 * time.Millisecond // Reduced from 50ms for faster detection
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	timeout := time.After(maxWaitTime)

	// Also check immediately before entering the loop
	if _, err := os.Stat(playlistPath); err == nil {
		log.Debug().Msgf("HLS playlist found immediately: %s", playlistPath)
		return true
	}

	for {
		select {
		case <-ticker.C:
			// Reduce logging frequency to avoid log spam
			if _, err := os.Stat(playlistPath); err == nil {
				log.Debug().Msgf("HLS playlist found: %s", playlistPath)

				// Verify the playlist is valid by checking its size
				if fileInfo, err := os.Stat(playlistPath); err == nil && fileInfo.Size() > 0 {
					return true
				} else {
					// File exists but is empty or can't be read, wait a bit longer
					log.Debug().Msgf("HLS playlist found but may be incomplete, waiting a bit longer")
					time.Sleep(50 * time.Millisecond)
				}
			}
		case <-timeout:
			log.Warn().Msgf("Timeout waiting for HLS playlist: %s", playlistPath)
			return false
		}
	}
}
