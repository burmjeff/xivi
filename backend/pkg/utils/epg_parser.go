package utils

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/outbound"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

// Constants for optimization
const (
	ChannelBatchSize   = 50  // Number of channels to process in a batch
	ProgrammeBatchSize = 250 // Number of programmes to process in a batch
	MaxWorkers         = 2   // Maximum number of worker goroutines
	MaxRetries         = 5   // Number of retries for database operations
)

// ParseEpg parses an EPG file and stores the data in the database.
// Errors are returned so callers that track background work can report an
// accurate failed state instead of treating a logged error as success.
func ParseEpg(epg *models.Epg) error {
	return ParseEpgWithProgress(epg, nil)
}

// ParseEpgWithProgress parses an EPG source while reporting indeterminate
// acquisition/parsing work and determinate import phases.
func ParseEpgWithProgress(epg *models.Epg, reporter ProgressReporter) error {
	start := time.Now()
	ctx := context.Background()

	log.Info().Msg("EPG Parser started")
	reportProgress(reporter, 0, "Downloading and reading XMLTV schedule…")

	// Parse the XML data
	epgItem, err := fetchAndParseEPGWithProgress(ctx, epg, nil, reporter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse EPG data")
		return fmt.Errorf("could not load guide data: %w", err)
	}
	programmeCount := len(epgItem.Programmes)
	epgItem.Programmes = currentEPGProgrammes(epgItem.Programmes, time.Now().Add(-24*time.Hour))
	for index := range epgItem.Channels {
		epgItem.Channels[index].Icon.Src = ""
	}
	for index := range epgItem.Programmes {
		epgItem.Programmes[index].Icon.Src = ""
	}
	if skipped := programmeCount - len(epgItem.Programmes); skipped > 0 {
		log.Info().Int("expired_programmes_skipped", skipped).Msg("Skipped expired XMLTV programmes before database import")
	}
	reportProgress(reporter, 20, fmt.Sprintf("XMLTV loaded: %d channels and %d programmes.", len(epgItem.Channels), len(epgItem.Programmes)))

	// Process channels in batches
	if len(epgItem.Channels) > 0 {
		log.Info().Int("count", len(epgItem.Channels)).Msg("Processing EPG channels")
		processChannels(ctx, epgItem.Channels, reporter)
	}

	// Process programmes in batches using improved workers with better retry logic
	if len(epgItem.Programmes) > 0 {
		log.Info().Int("count", len(epgItem.Programmes)).Msg("Processing EPG programmes")
		// Use the improved version with better retry logic
		processProgrammes(ctx, epgItem.Programmes, reporter)
	}

	// Update EPG timestamp
	epg.UpdatedAt = time.Now()
	err = database.Db.UpdateEpg(ctx, epg.ID, epg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update EPG timestamp")
		return fmt.Errorf("could not finish the guide import: %w", err)
	}

	log.Info().Dur("duration", time.Since(start)).Msg("EPG Parser Finished")

	// Generate EPG XML files for all templates in parallel
	reportProgress(reporter, 86, "Rebuilding lineup guide outputs…")
	if err := generateEPGFiles(ctx, reporter); err != nil {
		return fmt.Errorf("guide data imported but lineup XMLTV outputs could not be rebuilt: %w", err)
	}
	reportProgress(reporter, 95, "Guide import saved.")
	return nil
}

func currentEPGProgrammes(programmes []models.EpgProgramme, cutoff time.Time) []models.EpgProgramme {
	retained := programmes[:0]
	for _, programme := range programmes {
		if programme.Stop != nil && programme.Stop.Time.Before(cutoff) {
			continue
		}
		retained = append(retained, programme)
	}
	return retained
}

// fetchAndParseEPG fetches and parses the EPG data from a URL or file
func fetchAndParseEPG(ctx context.Context, epg *models.Epg, client *http.Client) (models.EpgItem, error) {
	return fetchAndParseEPGWithProgress(ctx, epg, client, nil)
}

func fetchAndParseEPGWithProgress(ctx context.Context, epg *models.Epg, client *http.Client, reporter ProgressReporter) (models.EpgItem, error) {
	var reader io.Reader
	var cleanup func()

	if isValidURL(epg.URL) {
		log.Info().Msg("Fetching EPG from URL...")
		// Use the optimized HTTP client
		var resp *http.Response
		var err error
		if client != nil {
			request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, epg.URL, nil)
			if requestErr != nil {
				return models.EpgItem{}, requestErr
			}
			resp, err = client.Do(request)
			if err == nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
				status := resp.Status
				resp.Body.Close()
				return models.EpgItem{}, fmt.Errorf("guide source returned %s", status)
			}
		} else {
			resp, err = outbound.Open(ctx, epg.URL, outbound.Policy{
				AllowPrivate: true,
				Timeout:      5 * time.Minute,
				MaxRedirects: 3,
			}, http.Header{"User-Agent": []string{"Xivi XMLTV importer"}})
		}
		if err != nil {
			return models.EpgItem{}, err
		}
		const maxXMLTVBytes = int64(512 << 20)
		if resp.ContentLength > maxXMLTVBytes {
			resp.Body.Close()
			return models.EpgItem{}, fmt.Errorf("guide source exceeds the 512 MiB limit")
		}

		cleanup = func() { resp.Body.Close() }

		// Check for gzip encoding
		if resp.Header.Get("Content-Encoding") == "gzip" ||
			(resp.Header.Get("Content-Type") == "application/x-gzip") {
			gzReader, err := gzip.NewReader(io.LimitReader(resp.Body, maxXMLTVBytes+1))
			if err != nil {
				return models.EpgItem{}, err
			}
			reader = io.LimitReader(gzReader, maxXMLTVBytes+1)
			oldCleanup := cleanup
			cleanup = func() {
				gzReader.Close()
				oldCleanup()
			}
		} else {
			// Check for gzip magic bytes
			bufReader := bufio.NewReader(io.LimitReader(resp.Body, maxXMLTVBytes+1))
			testBytes, err := bufReader.Peek(2)
			if err == nil && testBytes[0] == 31 && testBytes[1] == 139 {
				gzReader, err := gzip.NewReader(bufReader)
				if err != nil {
					return models.EpgItem{}, err
				}
				reader = io.LimitReader(gzReader, maxXMLTVBytes+1)
				oldCleanup := cleanup
				cleanup = func() {
					gzReader.Close()
					oldCleanup()
				}
			} else {
				reader = bufReader
			}
		}
	} else {
		return models.EpgItem{}, fmt.Errorf("local guide paths are not accepted; use an HTTPS URL")
	}

	defer cleanup()

	// Use optimized XML parsing
	return parseXMLWithProgress(reader, reporter)
}

// processChannels processes EPG channels in batches
func processChannels(ctx context.Context, channels []models.EpgChannel, reporter ProgressReporter) {
	// First, collect all channel IDs for efficient lookup
	var newChannels []models.EpgChannel
	var updateChannels []models.EpgChannel

	// Create a map to track all channel IDs we're processing
	allChannelIds := make(map[string]bool)

	// First pass: collect all channel IDs in a map
	for _, channel := range channels {
		if channel.ChannelId != "" {
			allChannelIds[channel.ChannelId] = true
		}
	}

	// Fetch all existing channels in one query if possible
	// TODO: Implement GetEpgChannelsByChannelIds to fetch multiple channels at once
	// For now, we'll build a map of existing channels
	existingChannels := make(map[string]*models.EpgChannel)
	existingChannelsList, err := database.Db.GetEpgChannels(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch existing EPG channels")
	} else if existingChannelsList != nil {
		for i := range *existingChannelsList {
			channel := (*existingChannelsList)[i]
			existingChannels[channel.ChannelId] = &channel
		}
		log.Info().Int("count", len(existingChannels)).Msg("Fetched existing EPG channels")
	}

	// Process channels in batches
	for i := 0; i < len(channels); i += ChannelBatchSize {
		end := i + ChannelBatchSize
		if end > len(channels) {
			end = len(channels)
		}

		batch := channels[i:end]

		for _, channel := range batch {
			if channel.ChannelId == "" {
				log.Debug().Str("displayName", channel.DisplayName).Msg("Skipping channel with empty channelId")
				continue
			}

			// Initialize Icon struct for new channels if needed
			if channel.Icon.Src == "" {
				channel.Icon = models.Icon{}
			}

			// Check if channel exists in our pre-fetched map
			existing, exists := existingChannels[channel.ChannelId]
			if !exists {
				existing, err := database.Db.GetEpgChannelByChannelId(ctx, channel.ChannelId)
				if err != nil {
					// Channel doesn't exist, add to new channels list
					newChannels = append(newChannels, channel)
					log.Debug().Str("channelId", channel.ChannelId).Str("displayName", channel.DisplayName).Msg("New channel identified")
				} else {
					// Channel exists but wasn't in our map, add it
					existingChannels[channel.ChannelId] = existing

					// Check if any fields have changed
					if existing.DisplayName != channel.DisplayName || existing.Icon.Src != channel.Icon.Src {
						// Update the existing channel with new data
						channel.ID = existing.ID // Preserve the ID for update
						updateChannels = append(updateChannels, channel)
						log.Debug().Str("channelId", channel.ChannelId).Str("displayName", channel.DisplayName).Msg("Channel update needed")
					}
				}
			} else {
				// Channel exists in our map, check if it needs updating
				if existing.DisplayName != channel.DisplayName || existing.Icon.Src != channel.Icon.Src {
					// Update the existing channel with new data
					channel.ID = existing.ID // Preserve the ID for update
					updateChannels = append(updateChannels, channel)
					log.Debug().Str("channelId", channel.ChannelId).Str("displayName", channel.DisplayName).Msg("Channel update needed")
				}
			}
		}
		progress := 20 + end*10/len(channels)
		reportProgress(reporter, progress, fmt.Sprintf("Preparing guide channels… %d/%d", end, len(channels)))
	}

	// Insert new channels in batches
	if len(newChannels) > 0 {
		log.Info().Int("count", len(newChannels)).Msg("Creating new EPG channels")

		// Use batch insert if available
		err := database.Db.BatchCreateEpgChannels(ctx, newChannels)
		if err != nil {
			log.Error().Err(err).Msg("Failed to batch create EPG channels")
			for _, channel := range newChannels {
				// Check if channel already exists
				existing, checkErr := database.Db.GetEpgChannelByChannelId(ctx, channel.ChannelId)
				if checkErr == nil {
					// Channel already exists, check if it needs updating
					if existing.DisplayName != channel.DisplayName || existing.Icon.Src != channel.Icon.Src {
						// Update the existing channel
						channel.ID = existing.ID
						if updateErr := database.Db.UpdateEpgChannel(ctx, channel.ID, &channel); updateErr != nil {
							log.Error().Err(updateErr).Str("channelId", channel.ChannelId).Msg("Failed to update existing EPG channel")
						}
					}
					log.Debug().Str("channelId", channel.ChannelId).Msg("Channel already exists, skipping creation")
				} else {
					// Try to create the channel
					_, createErr := database.Db.CreateEpgChannel(ctx, channel)
					if createErr != nil {
						if strings.Contains(createErr.Error(), "UNIQUE constraint failed") {
							log.Debug().Str("channelId", channel.ChannelId).Msg("Channel already exists (created by another process)")
						} else {
							log.Error().Err(createErr).Str("channelId", channel.ChannelId).Str("displayName", channel.DisplayName).Msg("Failed to create EPG channel")
						}
					}
				}
			}
		}
	}

	// Update existing channels that have changed
	if len(updateChannels) > 0 {
		log.Info().Int("count", len(updateChannels)).Msg("Updating existing EPG channels")

		// Update channels individually
		for _, channel := range updateChannels {
			if err := database.Db.UpdateEpgChannel(ctx, channel.ID, &channel); err != nil {
				log.Error().Err(err).Str("channelId", channel.ChannelId).Str("displayName", channel.DisplayName).Msg("Failed to update EPG channel")
			}
		}
	}
	reportProgress(reporter, 32, fmt.Sprintf("Guide channels ready: %d processed.", len(channels)))
}

// processProgrammes processes EPG programmes in batches using worker pool
func processProgrammes(ctx context.Context, programmes []models.EpgProgramme, reporter ProgressReporter) {
	numWorkers := 2
	var completed atomic.Int64
	var lastProgress atomic.Int64
	total := int64(len(programmes))
	completeProgramme := func() {
		done := completed.Add(1)
		progress := int64(32) + done*50/total
		for {
			previous := lastProgress.Load()
			if progress <= previous || !lastProgress.CompareAndSwap(previous, progress) {
				if progress <= previous {
					return
				}
				continue
			}
			reportProgress(reporter, int(progress), fmt.Sprintf("Importing programmes… %d/%d", done, total))
			return
		}
	}

	// Create work distribution channels
	jobs := make(chan models.EpgProgramme, 250) // Reduced from 500 to 250
	wg := sync.WaitGroup{}

	// Start worker pool
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processProgrammeWorker(ctx, jobs, workerID, completeProgramme)
		}(w)
	}

	// Send programmes to workers
	for _, programme := range programmes {
		if !programme.Start.IsZero() && programme.Channel != "" {
			jobs <- programme
		} else {
			log.Debug().Str("title", programme.Title.Value).Msg("Skipping invalid programme")
		}
	}

	// Signal workers to finish
	close(jobs)

	// Wait for all workers to complete
	wg.Wait()
}

// processProgrammeWorker processes programmes from the jobs channel
func processProgrammeWorker(ctx context.Context, jobs <-chan models.EpgProgramme, workerID int, complete func()) {
	for programme := range jobs {
		func() {
			defer complete()
			// Skip invalid programmes
			if programme.Channel == "" || programme.Start == nil || programme.Stop == nil {
				log.Warn().Str("title", programme.Title.Value).Msg("Skipping programme with missing required fields")
				return
			}

			// Ensure Title.Value is not empty
			if programme.Title.Value == "" {
				programme.Title.Value = "No Title"
				log.Warn().Str("channel", programme.Channel).Msg("Empty Title.Value in programme, using default")
			}

			// Log the programme being processed
			log.Debug().Str("channel", programme.Channel).Str("title", programme.Title.Value).Time("start", programme.Start.Time).Msg("Processing programme")

			// Look up existing programme with retry logic
			var existing *models.EpgProgramme
			var err error

			// Retry parameters
			baseDelay := 200 * time.Millisecond

			// Try to get the programme by exact time with retries
			for retry := 0; retry < MaxRetries; retry++ {
				existing, err = database.Db.GetProgrammeByExactTime(ctx, programme.Channel, programme.Start.Time, programme.Stop.Time)
				if err == nil || !strings.Contains(err.Error(), "database is locked") {
					break // Success or non-lock error
				}

				// Database lock error, retry with backoff
				delay := baseDelay * time.Duration(1<<uint(retry))
				time.Sleep(delay)
			}

			if err != nil {
				// Create new programme with retry logic
				var createErr error
				for retry := 0; retry < MaxRetries; retry++ {
					if existing == nil {
						_, createErr = database.Db.CreateEpgProgramme(ctx, programme)
						if createErr == nil || !strings.Contains(createErr.Error(), "database is locked") {
							if createErr != nil {
								log.Warn().Err(createErr).Str("channel", programme.Channel).Str("title", programme.Title.Value).Msg("Failed to create programme")
							}
							break // Success or non-lock error
						}
					} else {
						// Programme already exists, update it
						updateErr := database.Db.UpdateEpgProgramme(ctx, existing.ID, &programme)
						if updateErr == nil || !strings.Contains(updateErr.Error(), "database is locked") {
							if updateErr != nil {
								log.Warn().Err(updateErr).Str("channel", programme.Channel).Str("title", programme.Title.Value).Msg("Failed to update programme-1")
							}
							break // Success or non-lock error
						}
					}
					// Retry with backoff
					delay := baseDelay * time.Duration(1<<uint(retry))
					time.Sleep(delay)
				}
			} else {
				// Update existing programme with retry logic
				for retry := 0; retry < MaxRetries; retry++ {
					updateErr := database.Db.UpdateEpgProgramme(ctx, existing.ID, &programme)
					if updateErr == nil || !strings.Contains(updateErr.Error(), "database is locked") {
						if updateErr != nil {
							log.Warn().Err(updateErr).Str("channel", programme.Channel).Str("title", programme.Title.Value).Msg("Failed to update programme-2")
						}
						break // Success or non-lock error
					}

					// Database lock error, retry with backoff
					delay := baseDelay * time.Duration(1<<uint(retry))
					time.Sleep(delay)
				}
			}
		}()
	}
}

// generateEPGFiles generates EPG XML files for all templates
func generateEPGFiles(ctx context.Context, reporter ProgressReporter) error {
	templates, err := database.Db.GetTemplates()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get templates for EPG generation")
		return fmt.Errorf("could not load lineups for XMLTV generation: %w", err)
	}

	if templates == nil || len(*templates) == 0 {
		log.Info().Msg("No templates found for EPG generation")
		reportProgress(reporter, 94, "No lineup XMLTV outputs need rebuilding.")
		return nil
	}

	log.Info().Int("count", len(*templates)).Msg("Generating EPG files for templates")

	// Create a wait group to track completion
	wg := sync.WaitGroup{}
	var completed atomic.Int64
	total := int64(len(*templates))
	generationErrors := make(chan error, len(*templates))

	// Process each template in a separate goroutine
	for _, template := range *templates {
		wg.Add(1)
		go func(t models.Template) {
			defer wg.Done()
			if err := CreateEpgXML(t); err != nil {
				generationErrors <- fmt.Errorf("lineup %q: %w", t.Name, err)
			}
			done := completed.Add(1)
			reportProgress(reporter, 86+int(done)*8/int(total), fmt.Sprintf("Rebuilding lineup guide outputs… %d/%d", done, total))
		}(template)
	}

	// Wait for all EPG files to be generated
	wg.Wait()
	close(generationErrors)
	generationErrs := make([]error, 0, len(generationErrors))
	for generationErr := range generationErrors {
		generationErrs = append(generationErrs, generationErr)
	}
	return errors.Join(generationErrs...)
}

// parseXML parses XML data into an EpgItem
func parseXML(xmlData io.Reader) (models.EpgItem, error) {
	return parseXMLWithProgress(xmlData, nil)
}

func parseXMLWithProgress(xmlData io.Reader, reporter ProgressReporter) (models.EpgItem, error) {
	var epg models.EpgItem

	// Create a buffered reader
	bufReader := bufio.NewReaderSize(xmlData, 1024*1024) // 1MB buffer
	decoder := xml.NewDecoder(bufReader)

	for {
		// Read tokens from the XML data stream
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return models.EpgItem{}, err
		}

		// If the token is a start element, check the element name and decode the corresponding struct
		if se, ok := token.(xml.StartElement); ok {
			switch se.Name.Local {
			case "channel":
				var channel models.EpgChannel
				if err := decoder.DecodeElement(&channel, &se); err != nil {
					return models.EpgItem{}, err
				}
				epg.Channels = append(epg.Channels, channel)
			case "programme":
				var programme models.EpgProgramme
				if err := decoder.DecodeElement(&programme, &se); err != nil {
					return models.EpgItem{}, err
				}
				epg.Programmes = append(epg.Programmes, programme)
				if len(epg.Programmes)%5000 == 0 {
					reportProgress(reporter, 0, fmt.Sprintf("Reading XMLTV… %d programmes found.", len(epg.Programmes)))
				}
			}
		}
	}

	return epg, nil
}
