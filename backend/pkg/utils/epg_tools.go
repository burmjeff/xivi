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

// CreateEpgXML generates an EPG XML file for a template.
func CreateEpgXML(template models.Template) error {
	return CreateEpgXMLWithProgress(template, nil)
}

// CreateEpgXMLWithProgress generates an EPG XML file while reporting the
// measurable stages of channel, programme, and output generation.
func CreateEpgXMLWithProgress(template models.Template, reporter ProgressReporter) error {
	start := time.Now()
	ctx := context.Background()
	log.Info().Str("template", template.Name).Msg("Creating EPG XML")
	reportProgress(reporter, 5, "Loading lineup channels for XMLTV…")

	// Initialize EPG item with metadata
	epg := models.EpgItem{
		GeneratorInfo:  settings.APP_SETTINGS.Application.AppName,
		SourceInfoName: fmt.Sprintf("%s - %s", settings.APP_SETTINGS.Application.AppName, settings.APP_SETTINGS.Application.AppVersion),
	}

	// Get template channels
	channels, err := database.Db.GetTmplChannels(template.ID)
	if err != nil {
		log.Error().Err(err).Int64("template_id", template.ID).Msg("Failed to get template channels")
		return fmt.Errorf("could not load lineup channels for XMLTV: %w", err)
	}

	if len(channels) == 0 {
		log.Warn().Int64("template_id", template.ID).Msg("No channels found for template")
		return fmt.Errorf("could not build the XMLTV export: the lineup has no channels")
	}
	reportProgress(reporter, 10, fmt.Sprintf("Preparing %d XMLTV channels…", len(channels)))

	// Ensure directory exists
	if err := os.MkdirAll(settings.EPG_FILEPATH, 0755); err != nil {
		log.Error().Err(err).Str("path", settings.EPG_FILEPATH).Msg("Failed to create EPG directory")
		return fmt.Errorf("could not create the XMLTV output directory: %w", err)
	}

	// Create a unique temporary file first so concurrent publishes cannot collide.
	finalFilePath := fmt.Sprintf("%s/%s.xml", settings.EPG_FILEPATH, template.Name)

	file, err := os.CreateTemp(settings.EPG_FILEPATH, ".xivi-*.xml.tmp")
	if err != nil {
		log.Error().Err(err).Str("path", settings.EPG_FILEPATH).Msg("Failed to create temporary EPG file")
		return fmt.Errorf("could not create the XMLTV export: %w", err)
	}
	tempFilePath := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(tempFilePath)
	}()
	if err := file.Chmod(0644); err != nil {
		return fmt.Errorf("could not set XMLTV file permissions: %w", err)
	}

	// Use buffered writer for better performance
	bufWriter := bufio.NewWriterSize(file, XMLBufferSize)
	encoder := xml.NewEncoder(bufWriter)
	encoder.Indent("", "  ")

	// Write XML header
	if _, err = bufWriter.WriteString(xml.Header); err != nil {
		log.Error().Err(err).Msg("Failed to write XML header")
		return fmt.Errorf("could not write the XMLTV header: %w", err)
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

				// Set icon URL - get the logo from database to match M3U tvg-logo URL format
				logo, err := database.Db.GetLogo(ctx, channel.LogoId)
				if err != nil {
					log.Warn().Str("channel", channel.Name).Msg("Failed to get logo for XMLTV channel icon")
					// Use empty icon URL if logo not found
					epgChannel.Icon.Src = ""
				} else {
					epgChannel.Icon.Src = fmt.Sprintf("http://%s:%d/%s",
						settings.APP_SETTINGS.Server.Host,
						settings.APP_SETTINGS.Server.Port,
						GetLogoUrl(logo.Name))
				}

				channelChan <- *epgChannel
			}
		}
		close(channelChan)
	}()

	// Collect channels
	collectedChannels := 0
	for channel := range channelChan {
		epg.Channels = append(epg.Channels, channel)
		collectedChannels++
		reportProgress(reporter, 10+collectedChannels*20/len(channels), fmt.Sprintf("Preparing XMLTV channels… %d/%d", collectedChannels, len(channels)))
	}

	// Wait for channel collection to complete
	wg.Wait()

	// Process programmes for each channel
	programmeChan := make(chan []models.EpgProgramme, len(epg.Channels))
	programmeErrors := make(chan error, len(epg.Channels))
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
			if err != nil {
				programmeErrors <- fmt.Errorf("could not load programmes for channel %q: %w", ch.DisplayName, err)
				return
			}
			if epgProgrammes == nil || len(*epgProgrammes) == 0 {
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
		close(programmeErrors)
	}()

	// Collect programmes
	collectedProgrammes := 0
	for programmes := range programmeChan {
		epg.Programmes = append(epg.Programmes, programmes...)
		collectedProgrammes++
		reportProgress(reporter, 30+collectedProgrammes*45/len(epg.Channels), fmt.Sprintf("Gathering schedules… %d/%d channels", collectedProgrammes, len(epg.Channels)))
	}
	for programmeErr := range programmeErrors {
		log.Error().Err(programmeErr).Int64("template_id", template.ID).Msg("Failed to build XMLTV programmes")
		return programmeErr
	}

	// Encode EPG to XML
	reportProgress(reporter, 82, fmt.Sprintf("Encoding %d programmes to XMLTV…", len(epg.Programmes)))
	if err := encoder.Encode(epg); err != nil {
		log.Error().Err(err).Msg("Failed to encode EPG to XML")
		return fmt.Errorf("could not encode the XMLTV export: %w", err)
	}

	// Flush buffer to file
	reportProgress(reporter, 92, "Finalizing XMLTV output…")
	if err := bufWriter.Flush(); err != nil {
		log.Error().Err(err).Msg("Failed to flush XML buffer")
		return fmt.Errorf("could not finish writing the XMLTV export: %w", err)
	}

	// Close file before renaming
	if err := file.Close(); err != nil {
		return fmt.Errorf("could not close the XMLTV export: %w", err)
	}

	// Rename temporary file to final file
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		log.Error().Err(err).Str("from", tempFilePath).Str("to", finalFilePath).Msg("Failed to rename EPG file")
		return fmt.Errorf("could not replace the XMLTV export: %w", err)
	}

	reportProgress(reporter, 98, "XMLTV output is ready.")
	log.Info().Str("template", template.Name).Dur("duration", time.Since(start)).Msg("EPG XML created successfully")
	return nil
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
