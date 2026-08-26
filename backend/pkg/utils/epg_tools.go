package utils

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

const (
	XMLBufferSize               = 4 * 1024 * 1024
	xmlTVExportHistory          = 4 * time.Hour
	xmlTVExportFuture           = 72 * time.Hour
	xmlTVPlaceholderDuration    = 4 * time.Hour
	xmlTVPlaceholderDescription = "Schedule information is unavailable."
)

type xmlTVExportChannel struct {
	Channel  models.EpgChannel
	SourceID string
}

// CreateEpgXML generates an EPG XML file for a lineup.
func CreateEpgXML(lineup models.Template) error {
	return CreateEpgXMLWithProgress(lineup, nil)
}

// CreateEpgXMLWithProgress builds a bounded XMLTV window. Real guide rows are
// never changed or supplemented in the database; optional placeholders exist
// only in the published file so Studio coverage continues to report real data.
func CreateEpgXMLWithProgress(lineup models.Template, reporter ProgressReporter) error {
	start := time.Now()
	ctx := context.Background()
	log.Info().Str("lineup", lineup.Name).Msg("Creating EPG XML")
	reportProgress(reporter, 5, "Loading playable lineup channels for XMLTV…")

	channels, err := database.Db.GetPlayableTmplChannels(lineup.ID)
	if err != nil {
		log.Error().Err(err).Int64("lineup_id", lineup.ID).Msg("Failed to get lineup channels")
		return fmt.Errorf("could not load lineup channels for XMLTV: %w", err)
	}
	if len(channels) == 0 {
		return fmt.Errorf("could not build the XMLTV export: the lineup has no playable channels")
	}

	exportChannels := make([]xmlTVExportChannel, 0, len(channels))
	seenChannelIDs := make(map[string]struct{}, len(channels))
	for index, channel := range channels {
		exportID := xmlTVChannelID(lineup, channel)
		if _, exists := seenChannelIDs[exportID]; exists {
			reportProgress(reporter, 10+(index+1)*20/len(channels), fmt.Sprintf("Preparing XMLTV channels… %d/%d", index+1, len(channels)))
			continue
		}
		seenChannelIDs[exportID] = struct{}{}

		epgChannel := models.EpgChannel{ChannelId: exportID, DisplayName: channel.Name, Icon: models.Icon{}}
		if logo, logoErr := database.Db.GetLogo(ctx, channel.LogoId); logoErr == nil {
			epgChannel.Icon.Src = fmt.Sprintf("http://%s:%d/%s",
				settings.APP_SETTINGS.Server.Host,
				settings.APP_SETTINGS.Server.Port,
				GetLogoUrl(logo.Name))
		} else {
			log.Debug().Err(logoErr).Str("channel", channel.Name).Msg("Leaving XMLTV channel icon empty")
		}

		sourceID := xmlTVSourceChannelID(channel)
		exportChannels = append(exportChannels, xmlTVExportChannel{Channel: epgChannel, SourceID: sourceID})
		reportProgress(reporter, 10+(index+1)*20/len(channels), fmt.Sprintf("Preparing XMLTV channels… %d/%d", index+1, len(channels)))
	}

	windowStart, windowEnd := xmlTVExportWindow(time.Now())
	mappedIDs := make([]string, 0, len(exportChannels))
	for _, channel := range exportChannels {
		if channel.SourceID != "" {
			mappedIDs = append(mappedIDs, channel.SourceID)
		}
	}
	reportProgress(reporter, 32, fmt.Sprintf("Loading guide window for %d XMLTV channels…", len(exportChannels)))
	realProgrammes, err := database.Db.GetProgrammesByTVGIDsWindow(ctx, mappedIDs, windowStart, windowEnd)
	if err != nil {
		return fmt.Errorf("could not load the XMLTV guide window: %w", err)
	}
	programmesByChannel := make(map[string][]models.EpgProgramme, len(mappedIDs))
	for _, programme := range realProgrammes {
		programmesByChannel[programme.Channel] = append(programmesByChannel[programme.Channel], programme)
	}

	epg := models.EpgItem{
		GeneratorInfo:  settings.APP_SETTINGS.Application.AppName,
		SourceInfoName: fmt.Sprintf("%s - %s", settings.APP_SETTINGS.Application.AppName, settings.APP_SETTINGS.Application.AppVersion),
		Channels:       make([]models.EpgChannel, 0, len(exportChannels)),
		Programmes:     make([]models.EpgProgramme, 0, len(realProgrammes)),
	}
	for index, channel := range exportChannels {
		epg.Channels = append(epg.Channels, channel.Channel)
		real := programmesByChannel[channel.SourceID]
		epg.Programmes = append(epg.Programmes, programmesForXMLTV(
			channel.Channel.ChannelId,
			channel.Channel.DisplayName,
			real,
			windowStart,
			windowEnd,
			lineup.FillMissingGuideSlots,
		)...)
		reportProgress(reporter, 35+(index+1)*40/len(exportChannels), fmt.Sprintf("Building guide coverage… %d/%d channels", index+1, len(exportChannels)))
	}

	if err := os.MkdirAll(settings.EPG_FILEPATH, 0755); err != nil {
		return fmt.Errorf("could not create the XMLTV output directory: %w", err)
	}
	finalFilePath := filepath.Join(settings.EPG_FILEPATH, lineup.Name+".xml")
	file, err := os.CreateTemp(settings.EPG_FILEPATH, ".xivi-*.xml.tmp")
	if err != nil {
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

	bufWriter := bufio.NewWriterSize(file, XMLBufferSize)
	if _, err = bufWriter.WriteString(xml.Header); err != nil {
		return fmt.Errorf("could not write the XMLTV header: %w", err)
	}
	encoder := xml.NewEncoder(bufWriter)
	encoder.Indent("", "  ")
	reportProgress(reporter, 82, fmt.Sprintf("Encoding %d programmes to XMLTV…", len(epg.Programmes)))
	if err := encoder.Encode(epg); err != nil {
		return fmt.Errorf("could not encode the XMLTV export: %w", err)
	}
	reportProgress(reporter, 92, "Finalizing XMLTV output…")
	if err := bufWriter.Flush(); err != nil {
		return fmt.Errorf("could not finish writing the XMLTV export: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("could not close the XMLTV export: %w", err)
	}
	if err := os.Rename(tempFilePath, finalFilePath); err != nil {
		return fmt.Errorf("could not replace the XMLTV export: %w", err)
	}

	reportProgress(reporter, 98, "XMLTV output is ready.")
	log.Info().
		Str("lineup", lineup.Name).
		Int("channels", len(epg.Channels)).
		Int("programmes", len(epg.Programmes)).
		Dur("duration", time.Since(start)).
		Msg("EPG XML created successfully")
	return nil
}

func xmlTVExportWindow(now time.Time) (time.Time, time.Time) {
	location, err := time.LoadLocation(settings.APP_SETTINGS.Application.TZ)
	if err != nil {
		location = time.UTC
	}
	local := now.In(location)
	currentHour := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 0, 0, location)
	return currentHour.Add(-xmlTVExportHistory), currentHour.Add(xmlTVExportFuture)
}

func programmesForXMLTV(channelID, channelName string, real []models.EpgProgramme, from, to time.Time, fillMissing bool) []models.EpgProgramme {
	programmes := append([]models.EpgProgramme(nil), real...)
	sort.SliceStable(programmes, func(i, j int) bool {
		if programmes[i].Start == nil {
			return false
		}
		if programmes[j].Start == nil {
			return true
		}
		return programmes[i].Start.Before(programmes[j].Start.Time)
	})
	valid := programmes[:0]
	for _, programme := range programmes {
		if programme.Start == nil || programme.Stop == nil || !programme.Stop.After(programme.Start.Time) {
			continue
		}
		programme.Channel = channelID
		if strings.TrimSpace(programme.Title.Value) == "" {
			programme.Title.Value = "No Title"
		}
		valid = append(valid, programme)
	}
	if !fillMissing {
		return valid
	}

	result := make([]models.EpgProgramme, 0, len(valid)+4)
	cursor := from
	for _, programme := range valid {
		if programme.Start.After(cursor) {
			result = appendPlaceholderProgrammes(result, channelID, channelName, cursor, minTime(programme.Start.Time, to))
		}
		result = append(result, programme)
		if programme.Stop.After(cursor) {
			cursor = programme.Stop.Time
		}
		if !cursor.Before(to) {
			return result
		}
	}
	return appendPlaceholderProgrammes(result, channelID, channelName, cursor, to)
}

func appendPlaceholderProgrammes(programmes []models.EpgProgramme, channelID, channelName string, start, end time.Time) []models.EpgProgramme {
	if !start.Before(end) {
		return programmes
	}
	for start.Before(end) {
		stop := start.Add(xmlTVPlaceholderDuration)
		if stop.After(end) {
			stop = end
		}
		programmes = append(programmes, models.EpgProgramme{
			Start:   &models.Time{Time: start},
			Stop:    &models.Time{Time: stop},
			Channel: channelID,
			Title:   models.Title{Value: channelName},
			Desc:    xmlTVPlaceholderDescription,
		})
		start = stop
	}
	return programmes
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

// RebuildEpgOutputs rolls every lineup's export window forward even when an
// upstream guide refresh fails or no guide sources are configured.
func RebuildEpgOutputs(reporter ProgressReporter) error {
	return generateEPGFiles(context.Background(), reporter)
}

// RemoveEpg removes an EPG file.
func RemoveEpg(epg *models.Epg) {
	filePath := filepath.Join(settings.EPG_FILEPATH, epg.Name+".xml")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return
	}
	if err := os.Remove(filePath); err != nil {
		log.Error().Err(err).Str("file", filePath).Msg("Failed to remove EPG file")
	}
}
