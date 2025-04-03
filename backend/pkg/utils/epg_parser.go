package utils

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

// Constants for optimization
const (
	ChannelBatchSize   = 100 // Number of channels to process in a batch
	ProgrammeBatchSize = 500 // Number of programmes to process in a batch
	MaxWorkers         = 4   // Maximum number of worker goroutines
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

	// Process programmes in batches using workers
	if len(epgItem.Programmes) > 0 {
		log.Info().Int("count", len(epgItem.Programmes)).Msg("Processing EPG programmes")
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

	// First pass: collect all channel IDs to fetch in bulk
	channelIDs := make([]string, 0, len(channels))
	for _, channel := range channels {
		if channel.ChannelId != "" {
			channelIDs = append(channelIDs, channel.ChannelId)
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
	// Determine optimal number of workers based on CPU cores
	numWorkers := runtime.NumCPU()
	if numWorkers > MaxWorkers {
		numWorkers = MaxWorkers
	}

	// Create work distribution channels
	jobs := make(chan models.EpgProgramme, ProgrammeBatchSize)
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

	// Batch insert remaining programmes if supported
	// Note: This would require collecting programmes in the worker and returning them
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

		// Look up existing programme
		existing, err := database.Db.GetProgrammeByTime(ctx, programme.Channel, programme.Start.Time)
		if err != nil {
			// Create new programme
			_, createErr := database.Db.CreateEpgProgramme(ctx, programme)
			if createErr != nil {
				// Check if this is a duplicate error (constraint violation)
				if strings.Contains(createErr.Error(), "UNIQUE constraint failed") {
					// Try to get the existing programme again and update it
					existing, retryErr := database.Db.GetProgrammeByTime(ctx, programme.Channel, programme.Start.Time)
					if retryErr == nil {
						// Update the existing programme
						if updateErr := database.Db.UpdateEpgProgramme(ctx, existing.ID, &programme); updateErr != nil {
							log.Warn().Err(updateErr).Str("channel", programme.Channel).Str("title", programme.Title.Value).Msg("Failed to update programme after duplicate detection")
						}
					} else {
						log.Warn().Err(retryErr).Str("channel", programme.Channel).Str("title", programme.Title.Value).Msg("Failed to retrieve programme after duplicate detection")
					}
				} else {
					log.Warn().Err(createErr).Str("channel", programme.Channel).Str("title", programme.Title.Value).Msg("Failed to create programme")
				}
			}
		} else {
			// Update existing programme
			if err := database.Db.UpdateEpgProgramme(ctx, existing.ID, &programme); err != nil {
				log.Warn().Err(err).Str("channel", programme.Channel).Str("title", programme.Title.Value).Msg("Failed to update programme")
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
