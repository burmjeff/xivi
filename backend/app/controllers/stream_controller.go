package controllers

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/streaming"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

const streamOperationTimeout = 30 * time.Second

func streamSources(channels []models.ChannelUrl) []string {
	sources := make([]string, 0, len(channels))
	for _, channel := range channels {
		sources = append(sources, channel.Url)
	}
	return sources
}

func acquireStreamSession(ctx context.Context, streamID string) (*streaming.Session, []models.ChannelUrl, error) {
	channels, err := database.Db.GetChannelsbyUuid(ctx, streamID)
	if err != nil {
		return nil, nil, fmt.Errorf("load ordered source variants: %w", err)
	}
	if channels == nil || len(*channels) == 0 {
		return nil, nil, fmt.Errorf("no streamable source variants are available")
	}
	if !settings.APP_SETTINGS.Streaming.Proxy {
		return nil, *channels, nil
	}
	session, err := streaming.DefaultManager.Acquire(ctx, streamID, streamSources(*channels))
	if err != nil {
		return nil, *channels, err
	}
	return session, *channels, nil
}

func streamError(c *fiber.Ctx, status int, message string, err error) error {
	log.Error().Err(err).Str("stream_id", c.Params("stream_id")).Msg(message)
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	return c.Status(status).JSON(fiber.Map{"error": true, "msg": message, "detail": detail})
}

// GetStream returns the shared MPEG-TS transport stream for a channel UUID.
// Every viewer receives data from a bounded Go fan-out queue, so a slow or
// disconnected client cannot stall the one upstream GStreamer producer.
// @Router /stream/{stream_id} [get]
func GetStream(c *fiber.Ctx) error {
	streamID := c.Params("stream_id")
	if streamID == "" {
		return streamError(c, fiber.StatusBadRequest, "A stream id is required.", nil)
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), streamOperationTimeout)
	defer cancel()
	session, channels, err := acquireStreamSession(ctx, streamID)
	if err != nil {
		return streamError(c, fiber.StatusBadGateway, "The stream could not start.", err)
	}
	if !settings.APP_SETTINGS.Streaming.Proxy {
		return c.Redirect(channels[0].Url, http.StatusTemporaryRedirect)
	}

	subscription := session.Subscribe()
	c.Set(fiber.HeaderContentType, "video/MP2T")
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Context().Response.SetBodyStreamWriter(func(writer *bufio.Writer) {
		defer subscription.Close()
		buffered := 0
		lastFlush := time.Now()
		for {
			chunk, ok := subscription.Next()
			if !ok {
				_ = writer.Flush()
				return
			}
			if _, err := writer.Write(chunk); err != nil {
				return
			}
			buffered += len(chunk)
			if buffered >= 256*1024 || time.Since(lastFlush) >= 200*time.Millisecond {
				if err := writer.Flush(); err != nil {
					return
				}
				buffered = 0
				lastFlush = time.Now()
			}
		}
	})
	return nil
}

// GetHlsStream starts or reuses the same producer as GetStream and returns its
// validated live playlist. Readiness requires a non-empty segment, not merely
// the existence of a playlist file.
// @Router /stream/hls/{stream_id} [get]
func GetHlsStream(c *fiber.Ctx) error {
	streamID := c.Params("stream_id")
	if streamID == "" {
		return streamError(c, fiber.StatusBadRequest, "A stream id is required.", nil)
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), streamOperationTimeout)
	defer cancel()
	session, channels, err := acquireStreamSession(ctx, streamID)
	if err != nil {
		return streamError(c, fiber.StatusBadGateway, "The stream could not start.", err)
	}
	if !settings.APP_SETTINGS.Streaming.Proxy {
		return c.Redirect(channels[0].Url, http.StatusTemporaryRedirect)
	}
	if err := session.WaitHLS(ctx); err != nil {
		return streamError(c, fiber.StatusGatewayTimeout, "The HLS playlist did not become ready.", err)
	}
	return sendHLSFile(c, streamID, "playlist.m3u8")
}

// GetHlsAsset serves only files belonging to a live managed session. This both
// blocks path traversal/stale-file access and lets playlist/segment requests
// keep HLS-only sessions alive until the viewer stops polling.
func GetHlsAsset(c *fiber.Ctx) error {
	streamID := c.Params("stream_id")
	asset := c.Params("asset")
	if filepath.Base(asset) != asset || (!strings.HasSuffix(asset, ".ts") && asset != "playlist.m3u8") {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if !streaming.DefaultManager.Touch(streamID) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	return sendHLSFile(c, streamID, asset)
}

func sendHLSFile(c *fiber.Ctx, streamID, asset string) error {
	path := filepath.Join(settings.STREAM_FILEPATH, streamID, asset)
	if asset == "playlist.m3u8" {
		var snapshot *streaming.HLSPlaylistSnapshot
		var err error
		for attempt := 0; attempt < 4; attempt++ {
			snapshot, err = streaming.ReadHLSPlaylist(path)
			if err == nil {
				c.Set(fiber.HeaderContentType, "application/vnd.apple.mpegurl")
				c.Set(fiber.HeaderCacheControl, "no-cache, no-store, must-revalidate")
				c.Set("Pragma", "no-cache")
				c.Set("Expires", "0")
				return c.Send(snapshot.Content)
			}
			time.Sleep(15 * time.Millisecond)
		}
		log.Debug().Err(err).Str("stream_id", streamID).Msg("HLS playlist rewrite was not ready to serve")
		c.Set("Retry-After", "1")
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return c.SendStatus(fiber.StatusNotFound)
	}
	c.Set(fiber.HeaderContentType, "video/MP2T")
	c.Set(fiber.HeaderCacheControl, "public, max-age=60, immutable")
	return c.SendFile(path)
}

// V2StreamingStatus exposes enough state to diagnose startup, failover, stalls
// and slow-client eviction without relying on server logs.
func V2StreamingStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"summary":  streaming.DefaultManager.Summary(),
		"sessions": streaming.DefaultManager.Snapshots(),
	})
}

// GetHlsChannels gets HLS channels by legacy template group.
// @Router /channels/hls/{group_id} [get]
func GetHlsChannels(c *fiber.Ctx) error {
	ctx := context.Background()
	groupID, err := strconv.ParseInt(c.Params("group_id"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "msg": err.Error()})
	}

	templateChannels, err := database.Db.GetTmplChannelsByGroup(groupID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "msg": "No channels found", "channels": nil})
	}

	channels := []models.LiveChannel{}
	for _, templateChannel := range templateChannels {
		items, itemErr := database.Db.GetTmplChannelItemsByCh(templateChannel.ID)
		if itemErr != nil || items == nil || len(*items) == 0 {
			continue
		}
		channel := models.LiveChannel{ID: templateChannel.ID, Name: templateChannel.Name}
		if logo, logoErr := database.Db.GetLogo(ctx, templateChannel.LogoId); logoErr == nil {
			channel.Logo = utils.GetLogoUrl(logo.Name)
		}
		channel.Stream = fmt.Sprintf("http://%s:%d/stream/hls/%s", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port, templateChannel.Uuid)
		if templateChannel.TvgID != nil {
			if programme, programmeErr := database.Db.GetCurrentProgramme(ctx, *templateChannel.TvgID, time.Now()); programmeErr == nil {
				channel.Programme = programme.Title.Value
				channel.Start = programme.Start.String()
				channel.End = programme.Stop.String()
				if next, nextErr := database.Db.GetCurrentProgramme(ctx, *templateChannel.TvgID, programme.Stop.Time); nextErr == nil {
					channel.Next = next.Title.Value
				} else {
					channel.Next = "No programme information"
				}
			} else {
				channel.Programme = "No programme information"
			}
		}
		channels = append(channels, channel)
	}

	return c.JSON(fiber.Map{"error": false, "msg": nil, "channels": channels})
}
