package utils

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

// Constants for optimization
const (
	ChannelBatchSize   = 50  // Reduced from 100 to 50 to reduce transaction size
	ProgrammeBatchSize = 250 // Reduced from 500 to 250 to reduce transaction size
	MaxWorkers         = 2   // Reduced from 4 to 2 to reduce database contention
	MaxRetries         = 5   // Number of retries for database operations
)

// ParseEpg parses an EPG file and stores the data in the database
func ParseEpg(epg *models.Epg) {
	start := time.Now()
	ctx := context.Background()

	log.Info().Msg("EPG Parser started")

	// Create a custom HTTP client with optimized settings
	client := &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  false, // Enable automatic compression handling
		},
	}

	// Parse the XML data
	epgItem, err := fetchAndParseEPG(ctx, epg, client)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse EPG data")
		return
	}

	// Process channels in batches
	if len(epgItem.Channels) > 0 {
		log.Info().Int("count", len(epgItem.Channels)).Msg("Processing EPG channels")
		processChannels(ctx, epgItem.Channels)
	}

	// Process programmes in batches using improved workers with better retry logic
	if len(epgItem.Programmes) > 0 {
		log.Info().Int("count", len(epgItem.Programmes)).Msg("Processing EPG programmes")
		// Use the improved version with better retry logic
		processProgrammes(ctx, epgItem.Programmes)
	}

	// Update EPG timestamp
	epg.UpdatedAt = time.Now()
	err = database.Db.UpdateEpg(ctx, epg.ID, epg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update EPG timestamp")
	}

	log.Info().Dur("duration", time.Since(start)).Msg("EPG Parser Finished")

	// Generate EPG XML files for all templates in parallel
	generateEPGFiles(ctx)
}

// fetchAndParseEPG fetches and parses the EPG data from a URL or file
func fetchAndParseEPG(ctx context.Context, epg *models.Epg, client *http.Client) (models.EpgItem, error) {
	var reader io.Reader
	var cleanup func()

	if isValidURL(epg.URL) {
		log.Info().Msg("Fetching EPG from URL...")
		// Use the optimized HTTP client
		req, err := http.NewRequestWithContext(ctx, "GET", epg.URL, nil)
		if err != nil {
			return models.EpgItem{}, err
		}

		// Add headers for compression support
		req.Header.Set("Accept-Encoding", "gzip, deflate")

		resp, err := client.Do(req)
		if err != nil {
			return models.EpgItem{}, err
		}

		cleanup = func() { resp.Body.Close() }

		// Check for gzip encoding
		if resp.Header.Get("Content-Encoding") == "gzip" ||
			(resp.Header.Get("Content-Type") == "application/x-gzip") {
			gzReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				return models.EpgItem{}, err
			}
			reader = gzReader
			oldCleanup := cleanup
			cleanup = func() {
				gzReader.Close()
				oldCleanup()
			}
		} else {
			// Check for gzip magic bytes
			bufReader := bufio.NewReader(resp.Body)
			testBytes, err := bufReader.Peek(2)
			if err == nil && testBytes[0] == 31 && testBytes[1] == 139 {
				gzReader, err := gzip.NewReader(bufReader)
				if err != nil {
					return models.EpgItem{}, err
				}
				reader = gzReader
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
		log.Info().Msg("Reading EPG from file...")
		file, err := os.Open(epg.URL)
		if err != nil {
			return models.EpgItem{}, err
		}
		reader = file
		cleanup = func() { file.Close() }
	}

	defer cleanup()

	// Use optimized XML parsing
	return parseXML(reader)
}

// processChannels processes EPG channels in batches
func processChannels(ctx context.Context, channels []models.EpgChannel) {
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
				log.Warn().Str("displayName", channel.DisplayName).Msg("Skipping channel with empty channelId")
				continue
			}

			// Initialize Icon struct for new channels if needed
			if channel.Icon.Src == "" {
				channel.Icon = models.Icon{}
			}

			// Check if channel exists in our pre-fetched map
			existing, exists := existingChannels[channel.ChannelId]
			if !exists {
				// Double-check with a direct query in case our bulk fetch missed it
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
	}

	// Insert new channels in batches
	if len(newChannels) > 0 {
		log.Info().Int("count", len(newChannels)).Msg("Creating new EPG channels")

		// Use batch insert if available
		err := database.Db.BatchCreateEpgChannels(ctx, newChannels)
		if err != nil {
			log.Error().Err(err).Msg("Failed to batch create EPG channels")

			// Fall back to individual inserts with better error handling
			for _, channel := range newChannels {
				// Check if channel already exists (could have been created by another process)
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
						// If it's a unique constraint error, the channel was created by another process
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
}

// processProgrammes processes EPG programmes in batches using worker pool
func processProgrammes(ctx context.Context, programmes []models.EpgProgramme) {
	// Determine optimal number of workers based on CPU cores, but limit to 2 to reduce contention
	numWorkers := 2 // Reduced from MaxWorkers to 2

	// Create work distribution channels with smaller buffer to reduce memory usage
	jobs := make(chan models.EpgProgramme, 250) // Reduced from 500 to 250
	wg := sync.WaitGroup{}

	// Start worker pool
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processProgrammeWorker(ctx, jobs, workerID)
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
func processProgrammeWorker(ctx context.Context, jobs <-chan models.EpgProgramme, workerID int) {
	for programme := range jobs {
		// Skip invalid programmes
		if programme.Channel == "" || programme.Start == nil || programme.Stop == nil {
			log.Warn().Str("title", programme.Title.Value).Msg("Skipping programme with missing required fields")
			continue
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

		// Try to get the programme with retries
		for retry := 0; retry < EpgMaxRetries; retry++ {
			existing, err = database.Db.GetProgrammeByTime(ctx, programme.Channel, programme.Start.Time)
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
			for retry := 0; retry < EpgMaxRetries; retry++ {
				_, createErr = database.Db.CreateEpgProgramme(ctx, programme)
				if createErr == nil {
					break // Success
				}

				// Check if this is a duplicate error (constraint violation)
				if strings.Contains(createErr.Error(), "UNIQUE constraint failed") {
					// Try to get the existing programme again and update it
					for updateRetry := 0; updateRetry < EpgMaxRetries; updateRetry++ {
						existing, retryErr := database.Db.GetProgrammeByTime(ctx, programme.Channel, programme.Start.Time)
						if retryErr == nil {
							// Update the existing programme
							updateErr := database.Db.UpdateEpgProgramme(ctx, existing.ID, &programme)
							if updateErr == nil || !strings.Contains(updateErr.Error(), "database is locked") {
								break // Success or non-lock error
							}
						}

						// Retry with backoff
						delay := baseDelay * time.Duration(1<<uint(updateRetry))
						time.Sleep(delay)
					}
					break // Exit the create retry loop after handling duplicate
				}

				// If it's a database lock error, retry with backoff
				if strings.Contains(createErr.Error(), "database is locked") ||
					strings.Contains(createErr.Error(), "busy") {
					delay := baseDelay * time.Duration(1<<uint(retry))
					time.Sleep(delay)
					continue
				}

				// Other error, log and break
				log.Warn().Err(createErr).Str("channel", programme.Channel).Str("title", programme.Title.Value).Msg("Failed to create programme")
				break
			}
		} else {
			// Update existing programme with retry logic
			for retry := 0; retry < EpgMaxRetries; retry++ {
				updateErr := database.Db.UpdateEpgProgramme(ctx, existing.ID, &programme)
				if updateErr == nil || !strings.Contains(updateErr.Error(), "database is locked") {
					if updateErr != nil {
						log.Warn().Err(updateErr).Str("channel", programme.Channel).Str("title", programme.Title.Value).Msg("Failed to update programme")
					}
					break // Success or non-lock error
				}

				// Database lock error, retry with backoff
				delay := baseDelay * time.Duration(1<<uint(retry))
				time.Sleep(delay)
			}
		}
	}
}

// generateEPGFiles generates EPG XML files for all templates
func generateEPGFiles(ctx context.Context) {
	templates, err := database.Db.GetTemplates()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get templates for EPG generation")
		return
	}

	if templates == nil || len(*templates) == 0 {
		log.Info().Msg("No templates found for EPG generation")
		return
	}

	log.Info().Int("count", len(*templates)).Msg("Generating EPG files for templates")

	// Create a wait group to track completion
	wg := sync.WaitGroup{}

	// Process each template in a separate goroutine
	for _, template := range *templates {
		wg.Add(1)
		go func(t models.Template) {
			defer wg.Done()
			CreateEpgXML(t)
		}(template)
	}

	// Wait for all EPG files to be generated
	wg.Wait()
}

// parseXML parses XML data into an EpgItem
func parseXML(xmlData io.Reader) (models.EpgItem, error) {
	var epg models.EpgItem

	// Create a buffered reader for better performance
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
			}
		}
	}

	return epg, nil
}
