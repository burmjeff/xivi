package utils

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

// Constants for EPG XML generation
const (
	EpgChannelBatchSize   = 50              // Number of channels to process in a batch
	EpgProgrammeBatchSize = 200             // Number of programmes to process in a batch
	XMLBufferSize         = 4 * 1024 * 1024 // 4MB buffer for XML encoding
)

// CreateEpgXML generates an EPG XML file for a template
func CreateEpgXML(template models.Template) {
	start := time.Now()
	ctx := context.Background()
	log.Info().Str("template", template.Name).Msg("Creating EPG XML")

	// Initialize EPG item with metadata
	epg := models.EpgItem{
		GeneratorInfo:  settings.APP_SETTINGS.Application.AppName,
		SourceInfoName: fmt.Sprintf("%s - %s", settings.APP_SETTINGS.Application.AppName, settings.APP_SETTINGS.Application.AppVersion),
	}

	// Get template channels
	channels, err := database.Db.GetTmplChannels(template.ID)
	if err != nil {
		log.Error().Err(err).Int64("template_id", template.ID).Msg("Failed to get template channels")
		return
	}

	if len(channels) == 0 {
		log.Warn().Int64("template_id", template.ID).Msg("No channels found for template")
		return
	}

	// Ensure directory exists
	if err := os.MkdirAll(settings.EPG_FILEPATH, 0755); err != nil {
		log.Error().Err(err).Str("path", settings.EPG_FILEPATH).Msg("Failed to create EPG directory")
		return
	}

	// Create temporary file first to avoid corrupting existing file if there's an error
	tempFilePath := fmt.Sprintf("%s/%s.xml.tmp", settings.EPG_FILEPATH, template.Name)
	finalFilePath := fmt.Sprintf("%s/%s.xml", settings.EPG_FILEPATH, template.Name)

	file, err := os.Create(tempFilePath)
	if err != nil {
		log.Error().Err(err).Str("path", tempFilePath).Msg("Failed to create temporary EPG file")
		return
	}
	defer file.Close()

	// Use buffered writer for better performance
	bufWriter := bufio.NewWriterSize(file, XMLBufferSize)
	encoder := xml.NewEncoder(bufWriter)
	encoder.Indent("", "  ")

	// Write XML header
	if _, err = bufWriter.WriteString(xml.Header); err != nil {
		log.Error().Err(err).Msg("Failed to write XML header")
		return
	}

	// Process channels in parallel
	channelChan := make(chan models.EpgChannel, len(channels))
	var wg sync.WaitGroup

	// Start goroutine to collect channels
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, channel := range channels {
			if channel.TvgID != nil {
				epgChannel, err := database.Db.GetEpgChannelByChannelId(ctx, *channel.TvgID)
				if err != nil {
					log.Debug().Str("channel", channel.Name).Str("tvg_id", *channel.TvgID).Msg("Creating default EPG channel")
					epgChannel = &models.EpgChannel{
						ChannelId:   *channel.TvgID,
						DisplayName: channel.Name,
						Icon:        models.Icon{}, // Initialize Icon struct
					}
				}

				// Set icon URL
				epgChannel.Icon.Src = fmt.Sprintf("http://%s:%d/%s",
					settings.APP_SETTINGS.Server.Host,
					settings.APP_SETTINGS.Server.Port,
					GetLogoUrl(channel.Uuid))

				channelChan <- *epgChannel
			}
		}
		close(channelChan)
	}()

	// Collect channels
	for channel := range channelChan {
		epg.Channels = append(epg.Channels, channel)
	}

	// Wait for channel collection to complete
	wg.Wait()

	// Process programmes for each channel
	programmeChan := make(chan []models.EpgProgramme, len(epg.Channels))
	wg = sync.WaitGroup{}

	// Start goroutines to fetch programmes for each channel
	for _, channel := range epg.Channels {
		wg.Add(1)
		go func(ch models.EpgChannel) {
			defer wg.Done()

			// Log the channel ID for debugging
			log.Debug().Str("channelId", ch.ChannelId).Msg("Getting programmes for channel")

			// Get programmes for this channel
			epgProgrammes, err := database.Db.GetProgrammesBytvgid(ctx, ch.ChannelId)
			if err != nil || epgProgrammes == nil || len(*epgProgrammes) == 0 {
				log.Debug().Str("channel_id", ch.ChannelId).Msg("Generating default programmes")

				// Generate default programmes if none exist
				var defaultProgrammes []models.EpgProgramme
				timeNow := &models.Time{Time: time.Now().Truncate(time.Hour)}
				timeFrame := 4

				for i := 0; i <= 48; i += timeFrame {
					programme := models.EpgProgramme{
						Start:   &models.Time{Time: timeNow.Add(time.Hour * time.Duration(i))},
						Stop:    &models.Time{Time: timeNow.Add(time.Hour * time.Duration(i+timeFrame))},
						Channel: ch.ChannelId,
						Title: models.Title{
							Value: ch.DisplayName,
						},
						Desc: ch.DisplayName,
					}

					defaultProgrammes = append(defaultProgrammes, programme)
				}

				programmeChan <- defaultProgrammes
			} else {
				// Ensure Title.Value is not empty for each programme
				for i := range *epgProgrammes {
					if (*epgProgrammes)[i].Title.Value == "" {
						(*epgProgrammes)[i].Title.Value = "No Title"
						log.Warn().Str("channel", (*epgProgrammes)[i].Channel).Msg("Empty Title.Value in programme, using default")
					}
				}
				programmeChan <- *epgProgrammes
			}
		}(channel)
	}

	// Close programme channel when all goroutines are done
	go func() {
		wg.Wait()
		close(programmeChan)
	}()

	// Collect programmes
	for programmes := range programmeChan {
		epg.Programmes = append(epg.Programmes, programmes...)
	}

	// Encode EPG to XML
	if err := encoder.Encode(epg); err != nil {
		log.Error().Err(err).Msg("Failed to encode EPG to XML")
		return
	}

	// Flush buffer to file
	if err := bufWriter.Flush(); err != nil {
		log.Error().Err(err).Msg("Failed to flush XML buffer")
		return
	}

	// Close file before renaming
	file.Close()

	// Rename temporary file to final file
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		log.Error().Err(err).Str("from", tempFilePath).Str("to", finalFilePath).Msg("Failed to rename EPG file")
		return
	}

	log.Info().Str("template", template.Name).Dur("duration", time.Since(start)).Msg("EPG XML created successfully")
}

// RemoveEpg removes an EPG file
func RemoveEpg(epg *models.Epg) {
	filePath := filepath.Join(settings.EPG_FILEPATH, epg.Name+".xml")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// File doesn't exist, nothing to do
		return
	}

	if err := os.Remove(filePath); err != nil {
		log.Error().Err(err).Str("file", filePath).Msg("Failed to remove EPG file")
	}
}
