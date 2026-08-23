package utils

import (
	"context"
	"crypto/md5"
	"database/sql"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/rs/zerolog/log"
)

// ChannelURLPair represents a channel and its associated URL with a correlation ID
type ChannelURLPair struct {
	Channel       models.PlaylistChannel
	URL           models.ChannelUrl
	CorrelationID string // Used to ensure channels and URLs stay paired correctly
}

// M3uParser - A parser for m3u files.
type M3uParser struct {
	playlistID      int64
	lines           []string
	content         string
	regexes         map[string]*regexp.Regexp
	matchedPlaylist int64
	// New fields for batch processing with improved channel-URL association
	channelPairs []ChannelURLPair
	batchMutex   sync.Mutex
	batchSize    int
}

// Add a group creation semaphore to limit concurrent group creations
var (
	groupCreationSemaphore = make(chan struct{}, 2)
)

// Create group with limiting concurrency
func createGroupWithLimit(playlistId int64, groupName string) (int64, error) {
	groupCreationSemaphore <- struct{}{}        // Acquire semaphore
	defer func() { <-groupCreationSemaphore }() // Release semaphore

	playlistGroup := models.PlaylistGroup{
		PlaylistId: playlistId,
		Name:       groupName,
	}

	// Create a new group with the playlist ID and group name
	return database.Db.CreatePlGroup(playlistGroup)
}

// ParseM3u - Parses the content of local file/URL.
func (m *M3uParser) ParseM3u(playlist models.Playlist) {
	m.playlistID = playlist.ID
	log.Info().Msg("Parser started")

	// Initialize batch processing with smaller batch size to reduce contention
	m.batchSize = 25 // Further reduced from 50 to 25 to reduce transaction size and prevent deadlocks
	m.channelPairs = make([]ChannelURLPair, 0, m.batchSize)

	//Check if matching playlist exists
	m.matchedPlaylist = MatchDomain(playlist.ID)

	m.regexes = make(map[string]*regexp.Regexp)
	m.regexes["file"] = CompileRegex(`(?m)^[a-zA-Z]:\\((?:.*?\\)*).*.[\d\w]{3,5}$|^(/[^/]*)+/?.[\d\w]{3,5}$`)
	m.regexes["xuiID"] = CompileRegex(`xui-id="(.*?)"`)
	m.regexes["tvgID"] = CompileRegex(`tvg-id="(.*?)"`)
	m.regexes["tvgName"] = CompileRegex(`tvg-name="(.*?)"`)
	m.regexes["tvgLogo"] = CompileRegex(`tvg-logo="(.*?)"`)
	m.regexes["group"] = CompileRegex(`group-title="(.*?)"`)
	m.regexes["title"] = CompileRegex(`(?:",|" ,)(.*?)$`)

	if isValidURL(playlist.URL) {
		log.Info().Msg("Started parsing m3u URL...")
		resp, err := http.Get(playlist.URL)
		if err != nil {
			log.Error().Msgf("Unable to get M3U FILE: %v", err)
			return
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error().Msgf("Unable to get M3U FILE: %v", err)
			return
		}
		m.content = string(body)
	} else {
		log.Info().Msg("Started parsing m3u file...")
		body, err := os.ReadFile(playlist.URL)
		if err != nil {
			log.Error().Msgf("Unable to get M3U FILE: %v", err)
			return
		}
		m.content = string(body)
	}

	if m.content != "" {
		for _, line := range strings.Split(m.content, "\n") {
			if strings.TrimSpace(line) != "" {
				m.lines = append(m.lines, strings.TrimSpace(line))
			}
		}
	}

	log.Info().Msgf("Loaded %d lines from m3u content", len(m.lines))

	if len(m.lines) > 0 {
		m.parseLines()
	} else {
		log.Info().Msg("No content to parse!!!")
	}

	// Check if we have any remaining channels to process
	m.batchMutex.Lock()
	remainingChannels := len(m.channelPairs)
	m.batchMutex.Unlock()

	log.Info().Msgf("Parsing complete, flushing any remaining channel pairs: %d", remainingChannels)

	// Ensure any remaining batched items are processed
	m.flushBatches()

	// Log summary of processing
	log.Info().Msgf("M3U parse completed for playlist ID %d", playlist.ID)

	// Update playlist
	playlist.UpdatedAt = time.Now()
	if err := database.Db.UpdatePlaylist(playlist.ID, &playlist); err != nil {
		log.Error().Msgf("Failed to update playlist: %v", err)
	} else {
		log.Info().Msgf("Successfully updated playlist: %s (ID: %d)", playlist.Name, playlist.ID)
	}

	log.Info().Msg("Parser finished")
}

func (m *M3uParser) parseLines() {
	chunkSize := 100 // Increased from 10 to 100 for better throughput
	var wg sync.WaitGroup

	re := CompileRegex("#EXTINF")

	// Check if we have any EXTINF lines in the content
	extinfoCount := 0
	for _, line := range m.lines {
		if re.Match([]byte(line)) {
			extinfoCount++
		}
	}

	log.Info().Msgf("Found %d EXTINF lines in m3u content", extinfoCount)

	if extinfoCount == 0 {
		log.Warn().Msg("No channel information found in m3u content - check formatting")
		return
	}

	var chunks [][]string
	if len(m.lines) > 100 {
		chunkSize = int(math.RoundToEven(float64(len(m.lines) / 10)))
	}

	log.Info().Msgf("Parsing m3u with %d lines, chunking into size %d", len(m.lines), chunkSize)

	for i := 0; i < len(m.lines); i += chunkSize {
		end := i + chunkSize

		if end >= len(m.lines) {
			end = len(m.lines)
		} else if re.Match([]byte(m.lines[end])) {
			end += 1
		}

		chunks = append(chunks, m.lines[i:end])
	}

	log.Info().Msgf("Created %d chunks for processing", len(chunks))

	// Limit maximum concurrent chunk processing to 5
	concurrencyLimit := make(chan struct{}, 3)
	for chunkIndex, chunk := range chunks {
		wg.Add(1)
		concurrencyLimit <- struct{}{} // Acquire semaphore
		go func(chunk []string, chunkIndex int) {
			defer func() {
				<-concurrencyLimit // Release semaphore
				wg.Done()
				log.Info().Msgf("Completed processing chunk %d", chunkIndex)
			}()

			log.Info().Msgf("Starting to process chunk %d with %d lines", chunkIndex, len(chunk))
			channelsFound := 0

			for i := 0; i < len(chunk); i += 1 {
				if re.Match([]byte(chunk[i])) {
					// Sample part of the line to help debugging
					lineSample := chunk[i]
					if len(lineSample) > 50 {
						lineSample = lineSample[:50] + "..."
					}
					log.Debug().Msgf("Found EXTINF line: %s", lineSample)

					if i+1 < len(chunk) && isValidURL(chunk[i+1]) {
						m.parseLine(chunk[i], chunk[i+1])
						channelsFound++
					} else if i+1 < len(chunk) {
						// Log the invalid URL for debugging
						if i+1 < len(chunk) {
							invalidURL := chunk[i+1]
							if len(invalidURL) > 50 {
								invalidURL = invalidURL[:50] + "..."
							}
							log.Warn().Msgf("Invalid URL following EXTINF: %s", invalidURL)
						} else {
							log.Warn().Msg("EXTINF line has no following URL (at end of chunk)")
						}
					}
				}
			}

			log.Info().Msgf("Chunk %d found %d channels", chunkIndex, channelsFound)
		}(chunk, chunkIndex)
	}

	log.Info().Msg("Waiting for all chunks to complete processing")
	wg.Wait()
	log.Info().Msg("All chunks processed")
}

// flushBatches commits all pending database operations
func (m *M3uParser) flushBatches() {
	m.batchMutex.Lock()
	defer m.batchMutex.Unlock()

	if len(m.channelPairs) == 0 {
		log.Debug().Msg("No items in batch to flush")
		return
	}

	log.Info().Msgf("Starting flush of %d channel-URL pairs", len(m.channelPairs))

	// Define retry parameters with increased values for better handling of database locks
	maxRetries := 10                    // Increased from 5 to 10
	baseDelay := 500 * time.Millisecond // Increased from 50ms to 500ms

	// Prepare channels for matching after their rows have committed.
	pendingMatches := make([]models.PlaylistChannel, 0, len(m.channelPairs))

	for retry := 0; retry < maxRetries; retry++ {
		// Only log retries after the first attempt
		if retry > 0 {
			log.Warn().Msgf("Retrying batch flush (attempt %d/%d) after delay of %v",
				retry+1, maxRetries, baseDelay*time.Duration(1<<uint(retry-1)))
		}

		// Create a context with timeout for the transaction
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Begin transaction for batch insert/update with context
		tx, err := database.Db.PlaylistQueries.BeginTxx(ctx, nil)
		if err != nil {
			log.Error().Msgf("Failed to begin transaction: %v", err)

			// If we can't even begin the transaction, use exponential backoff
			if retry < maxRetries-1 && (strings.Contains(err.Error(), "database is locked") ||
				strings.Contains(err.Error(), "busy")) {
				delay := baseDelay * time.Duration(1<<uint(retry))
				jitter := time.Duration(rand.Intn(100)) * time.Millisecond // Increased jitter
				log.Warn().Msgf("Database locked, retrying in %v", delay+jitter)
				time.Sleep(delay + jitter)
				continue
			}
			return
		}

		// Process all channel pairs, storing created IDs
		channelIDMap := make(map[string]int64) // Map to track correlation IDs to channel IDs

		log.Info().Msgf("Processing %d channel pairs in batch", len(m.channelPairs))

		success := true
		for _, pair := range m.channelPairs {
			channel := pair.Channel
			var channelID int64

			if channel.ID == 0 {
				// Insert new channel
				log.Debug().Msgf("Inserting new channel: %s (GroupId: %d)", channel.Title, channel.GroupId)
				stmt, err := tx.Prepare("INSERT INTO playlistchannel (tvg_id, tvg_name, tvg_logo, title, group_id, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
				if err != nil {
					log.Error().Msgf("Failed to prepare statement: %v", err)
					tx.Rollback()
					success = false
					break
				}

				res, err := stmt.Exec(channel.TvgID, channel.TvgName, channel.Logo, channel.Title,
					channel.GroupId, channel.Enabled, channel.CreatedAt, channel.UpdatedAt)
				if err != nil {
					log.Error().Msgf("Failed to execute statement: %v", err)
					tx.Rollback()
					success = false
					break
				}

				channelID, err = res.LastInsertId()
				if err != nil {
					log.Error().Msgf("Failed to get last insert ID: %v", err)
					tx.Rollback()
					success = false
					break
				}

				log.Debug().Msgf("Successfully inserted channel: %s with ID %d (correlation: %s)", channel.Title, channelID, pair.CorrelationID)
				// Store the channel ID for URL processing
				channelIDMap[pair.CorrelationID] = channelID
			} else {
				// Update existing channel
				log.Debug().Msgf("Updating existing channel: %s (ID: %d)", channel.Title, channel.ID)
				stmt, err := tx.Prepare("UPDATE playlistchannel SET tvg_id = ?, tvg_name = ?, tvg_logo = ?, title = ?, enabled = ?, updated_at = ? WHERE id = ?")
				if err != nil {
					log.Error().Msgf("Failed to prepare update statement: %v", err)
					tx.Rollback()
					success = false
					break
				}

				_, err = stmt.Exec(channel.TvgID, channel.TvgName, channel.Logo, channel.Title,
					channel.Enabled, channel.UpdatedAt, channel.ID)
				if err != nil {
					log.Error().Msgf("Failed to execute update statement: %v", err)
					tx.Rollback()
					success = false
					break
				}

				log.Debug().Msgf("Successfully updated channel: %s (ID: %d, correlation: %s)", channel.Title, channel.ID, pair.CorrelationID)
				channelIDMap[pair.CorrelationID] = channel.ID
			}
		}

		// If we failed processing channels, retry or return
		if !success {
			log.Warn().Msg("Channel processing failed, retrying or returning")
			if retry < maxRetries-1 {
				delay := baseDelay * time.Duration(1<<uint(retry))
				jitter := time.Duration(rand.Intn(50)) * time.Millisecond
				time.Sleep(delay + jitter)
				continue
			}
			return
		}

		// Process all URLs from channel pairs after all channels are processed
		log.Info().Msgf("Processing URLs for %d channel pairs", len(m.channelPairs))
		for _, pair := range m.channelPairs {
			var stmt *sql.Stmt
			var err error
			url := pair.URL

			// Get the correct channel ID from our map using correlation ID
			if channelID, exists := channelIDMap[pair.CorrelationID]; exists {
				url.ChannelId = channelID
				log.Debug().Msgf("Mapped URL to channel ID: %d (correlation: %s)", channelID, pair.CorrelationID)
			}

			// Skip URLs with invalid channel IDs (would violate foreign key constraints)
			if url.ChannelId == 0 {
				log.Warn().Msg("Skipping URL insert with no valid channel ID")
				continue
			}

			// Check if URL exists
			var urlID int64
			row := tx.QueryRow("SELECT id FROM channelurl WHERE channel_id = ? LIMIT 1", url.ChannelId)
			err = row.Scan(&urlID)

			if err == sql.ErrNoRows || err != nil {
				// Insert new URL
				log.Debug().Msgf("Inserting new URL for channel ID: %d", url.ChannelId)
				stmt, err = tx.Prepare("INSERT INTO channelurl (url, channel_id, orderr, created_at, updated_at) VALUES (?, ?, ?, ?, ?)")
				if err != nil {
					log.Error().Msgf("Failed to prepare URL statement: %v", err)
					tx.Rollback()
					success = false
					break
				}

				_, err = stmt.Exec(url.Url, url.ChannelId, url.Order, url.CreatedAt, url.UpdatedAt)
			} else {
				// Update existing URL
				log.Debug().Msgf("Updating existing URL ID: %d for channel ID: %d", urlID, url.ChannelId)
				stmt, err = tx.Prepare("UPDATE channelurl SET url = ?, orderr = ?, updated_at = ? WHERE id = ?")
				if err != nil {
					log.Error().Msgf("Failed to prepare URL update statement: %v", err)
					tx.Rollback()
					success = false
					break
				}

				_, err = stmt.Exec(url.Url, url.Order, url.UpdatedAt, urlID)
			}

			if err != nil {
				log.Error().Msgf("Failed to execute URL statement: %v", err)
				tx.Rollback()
				success = false
				break
			}
		}

		// If we failed processing URLs, retry or return
		if !success {
			log.Warn().Msg("URL processing failed, retrying or returning")
			if retry < maxRetries-1 {
				delay := baseDelay * time.Duration(1<<uint(retry))
				jitter := time.Duration(rand.Intn(50)) * time.Millisecond
				time.Sleep(delay + jitter)
				continue
			}
			return
		}

		// Commit the transaction
		log.Info().Msg("All batch items processed, committing transaction")
		if err := tx.Commit(); err != nil {
			log.Error().Msgf("Failed to commit transaction: %v", err)
			// Attempt to rollback, but don't fail if rollback fails
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Error().Msgf("Failed to rollback transaction: %v", rbErr)
			}

			if retry < maxRetries-1 && (strings.Contains(err.Error(), "database is locked") ||
				strings.Contains(err.Error(), "busy") ||
				strings.Contains(err.Error(), "cannot commit") ||
				strings.Contains(err.Error(), "SQL statements in progress")) {
				delay := baseDelay * time.Duration(1<<uint(retry))
				jitter := time.Duration(rand.Intn(100)) * time.Millisecond // Increased jitter
				log.Warn().Msgf("Commit failed due to lock, retrying in %v", delay+jitter)
				time.Sleep(delay + jitter)
				continue
			}
			return
		}

		// Success - log and prepare channels for matching.
		log.Info().Msgf("Successfully committed batch of %d channel pairs (attempt %d)", len(m.channelPairs), retry+1)

		// Collect channels with their new IDs for matching.
		for _, pair := range m.channelPairs {
			if id, exists := channelIDMap[pair.CorrelationID]; exists {
				// Create a copy of the channel with the new ID
				channelCopy := pair.Channel
				channelCopy.ID = id
				pendingMatches = append(pendingMatches, channelCopy)
				log.Debug().Msgf("Added channel for matching: %s (ID: %d, correlation: %s)",
					channelCopy.Title, channelCopy.ID, pair.CorrelationID)
			}
		}

		// Clear the batches
		m.channelPairs = make([]ChannelURLPair, 0, m.batchSize)

		// Queue matching separately so the parser can release its batch lock.
		if len(pendingMatches) > 0 {
			log.Info().Msgf("Starting matching for %d channels", len(pendingMatches))
			channelsToMatch := make([]models.PlaylistChannel, len(pendingMatches))
			copy(channelsToMatch, pendingMatches)
			go m.processMatches(channelsToMatch)
		} else {
			log.Warn().Msg("No channels to match after batch processing")
		}

		return
	}

	// If we got here, all retries failed
	log.Error().Msgf("Failed to flush batch after %d retries", maxRetries)
}

func (m *M3uParser) parseLine(line string, streamLink string) {
	validate := NewValidator()
	playlistGroup := models.PlaylistGroup{}
	playlistGroup.PlaylistId = m.playlistID
	playlistChannel := models.PlaylistChannel{}
	channelURL := models.ChannelUrl{}

	if line != "" && streamLink != "" {

		tvgID := GetByRegex(m.regexes["tvgID"], line)
		tvgName := GetByRegex(m.regexes["tvgName"], line)
		tvgLogo := GetByRegex(m.regexes["tvgLogo"], line)
		groupName := GetByRegex(m.regexes["group"], line)
		title := GetByRegex(m.regexes["title"], line)

		// Add diagnostic logging for channel identification
		log.Debug().
			Str("tvgID", tvgID).
			Str("tvgName", tvgName).
			Str("groupName", groupName).
			Str("title", title).
			Msg("Parsing channel line")

		if tvgID != "" {
			playlistChannel.TvgID = &tvgID
			//Add new channel vectors
		} else {
			tvgID = GetByRegex(m.regexes["xuiID"], line)
			if tvgID != "" {
				playlistChannel.TvgID = &tvgID
			}
		}
		if tvgName != "" {
			playlistChannel.TvgName = tvgName
		}
		if tvgLogo != "" {
			playlistChannel.Logo = &tvgLogo
		}

		if title != "" {
			playlistChannel.Title = title
			if tvgName == "" {
				playlistChannel.TvgName = title
			}
		} else if tvgName != "" {
			playlistChannel.Title = tvgName
		} else if tvgID != "" {
			playlistChannel.Title = tvgID
		}

		if groupName != "" {
			// Checking, if playlist with given ID is exists.
			if group, err := database.Db.GetPlGroupByName(m.playlistID, groupName); group == nil {
				log.Info().Msgf("Group not found. %v, Creating Group: %s", err, groupName)

				// Use the new concurrency-limited group creation function
				if groupId, err := createGroupWithLimit(m.playlistID, groupName); err != nil {
					log.Error().Msgf("FAILED TO CREATE PLAYLIST GROUP: %v", err)
					return
				} else {
					playlistChannel.GroupId = groupId
					log.Info().Msgf("Successfully created group '%s' with ID %d", groupName, groupId)
				}
			} else {
				playlistChannel.GroupId = group.ID
				if !group.Enabled {
					log.Debug().Msgf("Skipping disabled group '%s'", groupName)
					return
				}
				log.Debug().Msgf("Using existing group '%s' with ID %d", groupName, group.ID)
			}
		} else {
			log.Error().Msgf("M3U PARSER: No Group found for %s", title)
			return
		}

		// Validate playlist fields.
		if err := validate.Struct(playlistChannel); err != nil {
			//Some fields are not valid.
			log.Error().Msg(err.Error())
			return
		}

		// Define a function to handle database retries
		getExistingChannels := func() ([]models.PlaylistChannel, error) {
			// Apply exponential backoff for database queries
			maxRetries := 5
			baseDelay := 50 * time.Millisecond

			for retry := 0; retry < maxRetries; retry++ {
				// Try to find channels by different identifiers
				foundChannels, err := database.Db.GetM3UParseByTvgID(tvgID, playlistChannel.GroupId, m.playlistID)
				if err == nil && len(foundChannels) > 0 {
					log.Debug().Msgf("Found channel by tvgID: %s", tvgID)
					return foundChannels, nil
				}

				if err != nil && !strings.Contains(err.Error(), "database is locked") {
					log.Error().Msgf("M3U_PARSER, tvg_id not found: %v", err)
				} else if err != nil {
					// Log only on first retry for locked database
					if retry == 0 {
						log.Warn().Msgf("Database locked when checking tvg_id, will retry: %v", err)
					}
					// Apply backoff with jitter
					delay := baseDelay * time.Duration(1<<uint(retry))
					jitter := time.Duration(rand.Intn(50)) * time.Millisecond
					time.Sleep(delay + jitter)
					continue
				}

				foundChannels, err = database.Db.GetM3UParseByTvgName(tvgName, playlistChannel.GroupId, m.playlistID)
				if err == nil && len(foundChannels) > 0 {
					log.Debug().Msgf("Found channel by tvgName: %s", tvgName)
					return foundChannels, nil
				}

				if err != nil && !strings.Contains(err.Error(), "database is locked") {
					log.Error().Msgf("M3U_PARSER, tvg_name not found: %v", err)
				} else if err != nil {
					// Apply backoff with jitter and continue
					delay := baseDelay * time.Duration(1<<uint(retry))
					jitter := time.Duration(rand.Intn(50)) * time.Millisecond
					time.Sleep(delay + jitter)
					continue
				}

				foundChannels, err = database.Db.GetM3UParseByTitle(title, playlistChannel.GroupId, m.playlistID)
				if err == nil {
					log.Debug().Msgf("Found channel by title: %s", title)
					return foundChannels, nil
				} else if !strings.Contains(err.Error(), "database is locked") {
					log.Error().Msgf("M3U_PARSER, title not found: %v", err)
					return nil, err
				} else {
					// Apply backoff with jitter and continue
					delay := baseDelay * time.Duration(1<<uint(retry))
					jitter := time.Duration(rand.Intn(50)) * time.Millisecond
					time.Sleep(delay + jitter)
					continue
				}
			}

			// If we get here after all retries, return empty result
			log.Debug().Msgf("No existing channel found for %s/%s/%s", tvgID, tvgName, title)
			return []models.PlaylistChannel{}, nil
		}

		foundChannels, _ := getExistingChannels()

		// Set common URL properties
		channelURL.Url = streamLink
		channelURL.CreatedAt = time.Now()
		channelURL.UpdatedAt = time.Now()
		// Default to 0 for order - will be set properly by the batch process
		channelURL.Order = 0

		// Create a unique correlation ID for this channel-URL pair
		// Use a combination of title, tvgID, and URL to ensure uniqueness
		correlationBase := title
		if tvgID != "" {
			correlationBase += "-" + tvgID
		}
		correlationBase += "-" + streamLink
		correlationID := fmt.Sprintf("%x", md5.Sum([]byte(correlationBase)))

		if len(foundChannels) == 0 {
			// New channel - add to batch
			playlistChannel.CreatedAt = time.Now()
			playlistChannel.UpdatedAt = time.Now()
			playlistChannel.Enabled = true

			// Create a channel-URL pair with correlation ID
			channelPair := ChannelURLPair{
				Channel:       playlistChannel,
				URL:           channelURL,
				CorrelationID: correlationID,
			}

			// Add to batch for processing
			m.batchMutex.Lock()
			preCount := len(m.channelPairs)
			m.channelPairs = append(m.channelPairs, channelPair)
			log.Debug().Msgf("Added new channel to batch: %s (correlation: %s, batch size: %d)",
				playlistChannel.Title, correlationID, len(m.channelPairs))

			// If batch is full, process it
			if len(m.channelPairs) >= m.batchSize {
				log.Info().Msgf("Batch full (%d channel pairs) - flushing batch", len(m.channelPairs))
				// Release the lock before flushing to prevent deadlock
				m.batchMutex.Unlock()
				// Flush directly instead of in a goroutine
				m.flushBatches()
				log.Info().Msgf("Batch flush complete, processed %d channel pairs", preCount)
			} else {
				m.batchMutex.Unlock()
			}
		} else {
			for _, foundChannel := range foundChannels {
				playlistChannel.UpdatedAt = time.Now()
				playlistChannel.Enabled = foundChannel.Enabled
				playlistChannel.ID = foundChannel.ID

				// Create a channel-URL pair with correlation ID
				channelPair := ChannelURLPair{
					Channel:       playlistChannel,
					URL:           channelURL,
					CorrelationID: correlationID,
				}

				m.batchMutex.Lock()
				preCount := len(m.channelPairs)
				m.channelPairs = append(m.channelPairs, channelPair)
				log.Debug().Msgf("Updated existing channel in batch: %s (ID: %d, correlation: %s, batch size: %d)",
					playlistChannel.Title, playlistChannel.ID, correlationID, len(m.channelPairs))

				// If batch is full, process it
				if len(m.channelPairs) >= m.batchSize {
					log.Info().Msgf("Batch full (%d channel pairs) - flushing batch", len(m.channelPairs))
					// Release the lock before flushing to prevent deadlock
					m.batchMutex.Unlock()
					// Flush directly instead of in a goroutine
					m.flushBatches()
					log.Info().Msgf("Batch flush complete, processed %d channel pairs", preCount)
				} else {
					m.batchMutex.Unlock()
				}
			}
		}

		// Update tvgID if needed
		if playlistChannel.TvgID == nil && playlistChannel.ID != 0 {
			tvgid := strconv.Itoa(int(playlistChannel.ID))
			playlistChannel.TvgID = &tvgid

			// Create a channel-URL pair with correlation ID for tvgID update
			channelPair := ChannelURLPair{
				Channel:       playlistChannel,
				URL:           models.ChannelUrl{ChannelId: playlistChannel.ID},
				CorrelationID: fmt.Sprintf("%x-update", md5.Sum([]byte(fmt.Sprintf("%d", playlistChannel.ID)))),
			}

			// Add to update batch
			m.batchMutex.Lock()
			m.channelPairs = append(m.channelPairs, channelPair)
			log.Debug().Msgf("Added channel to batch for tvgID update: %s (ID: %d)", playlistChannel.Title, playlistChannel.ID)
			m.batchMutex.Unlock()
		}
	}
}

func isValidURL(toTest string) bool {
	_, err := url.ParseRequestURI(toTest)
	if err != nil {
		return false
	}

	u, err := url.Parse(toTest)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	return true
}

// processMatches queues channels after their database rows have committed.
func (m *M3uParser) processMatches(channels []models.PlaylistChannel) {
	if len(channels) == 0 {
		return
	}

	log.Info().Msgf("Queueing matches for %d channels", len(channels))

	for _, channel := range channels {
		// Skip channels with no ID
		if channel.ID == 0 {
			log.Warn().Msg("Skipping matching for channel with ID 0")
			continue
		}
		MatchPlaylistChannel(channel)
	}
	log.Info().Msg("Match queueing completed")
}
