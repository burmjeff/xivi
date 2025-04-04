package utils

import (
	"context"
	"errors"
	"math"
	"sort"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

// Matching request queue with larger buffer
var (
	matchQueue     = make(chan matchRequest, 100)
	matchInit      sync.Once
	matchSemaphore = make(chan struct{}, 3)
)

type matchRequest struct {
	playlistCh *models.PlaylistChannel
	templateCh *models.TemplateChannel
	resultCh   chan<- error
}

// Initialize the matching worker pool
func initMatchWorkers() {
	workerCount := 3

	for i := 0; i < workerCount; i++ {
		go matchWorker()
	}
}

// Worker that processes match requests
func matchWorker() {
	for req := range matchQueue {
		matchSemaphore <- struct{}{}

		var err error

		if req.playlistCh != nil {
			// Handle playlist channel matching
			if req.playlistCh.Enabled {
				if settings.APP_SETTINGS.Playlist.Tvgid_match {
					if err = matchPlaylistTvgid(*req.playlistCh); err == nil {
						if req.resultCh != nil {
							req.resultCh <- nil
						}
						<-matchSemaphore
						continue
					}
				}

				if settings.APP_SETTINGS.Playlist.Name_match {
					err = matchPlaylistChannelName(*req.playlistCh)
				}
			}
		} else if req.templateCh != nil {
			// Handle template channel matching
			if settings.APP_SETTINGS.Playlist.Tvgid_match {
				if _, err = matchTemplateTvgid(req.templateCh); err == nil {
					if req.resultCh != nil {
						req.resultCh <- nil
					}
					<-matchSemaphore
					continue
				}
			}

			if settings.APP_SETTINGS.Playlist.Name_match {
				err = matchTemplateChannelName(req.templateCh)
			}
		}

		if req.resultCh != nil {
			req.resultCh <- err
		}

		<-matchSemaphore
	}
}

func MatchPlaylistChannel(playlistCh models.PlaylistChannel) {
	// Initialize workers if not already done
	matchInit.Do(initMatchWorkers)

	// Submit request to the matching queue
	matchQueue <- matchRequest{
		playlistCh: &playlistCh,
		templateCh: nil,
		resultCh:   nil, // No need for result in this case
	}
}

func MatchTemplateChannel(templateCh *models.TemplateChannel) {
	// Initialize workers if not already done
	matchInit.Do(initMatchWorkers)

	// Submit request to the matching queue
	matchQueue <- matchRequest{
		playlistCh: nil,
		templateCh: templateCh,
		resultCh:   nil, // No need for result in this case
	}
}

// Search if tvgid matches for playlist channel and add to template if match
func matchPlaylistTvgid(playlistCh models.PlaylistChannel) error {
	if playlistCh.TvgID == nil {
		return errors.New("tvgid is nil")
	}

	var err error
	maxRetries := 3
	retryDelay := 200 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		err = executeMatchPlaylistTvgid(playlistCh)
		if err == nil {
			return nil
		}

		if i < maxRetries-1 {
			log.Warn().Msgf("Retry %d: matchPlaylistTvgid failed: %v", i+1, err)
			time.Sleep(retryDelay)
			retryDelay *= 2
		}
	}

	return err
}

func executeMatchPlaylistTvgid(playlistCh models.PlaylistChannel) error {
	if playlistCh.TvgID == nil {
		return errors.New("tvgid is nil")
	}

	// Use transaction for batch operations
	tx, err := database.Db.VectorQueries.Beginx()
	if err != nil {
		log.Error().Msgf("Failed to begin transaction: %v", err)
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	playlistChUrl, err := database.Db.GetChannelUrl(playlistCh.ID)
	if err != nil {
		log.Debug().Msgf("matchPlaylistTvgid GetChannelUrl: %v", err)
		// Continue even if URL retrieval fails
	}

	channels, err := database.Db.GetTmplChannelsBytvgid(playlistCh.TvgID)
	if err != nil {
		log.Debug().Msgf("matchPlaylistTvgid GetTmplChannelsBytvgid: %v", err)
		return err
	}

	// Check if channels is nil or empty
	if channels == nil || len(*channels) == 0 {
		log.Debug().Msgf("matchPlaylistTvgid: No matching template channels found for tvgid %s", *playlistCh.TvgID)
		return nil // Return nil to indicate processing completed without error
	}

	// Prepare statement for batch inserts
	stmt, err := tx.Prepare("INSERT INTO templatechannelitem (channel_id, playlist_channel_id) VALUES (?, ?)")
	if err != nil {
		log.Error().Msgf("Failed to prepare statement: %v", err)
		return err
	}

	insertCount := 0
	for _, channel := range *channels {
		// Handle the case where playlistChUrl might be nil
		if playlistChUrl == nil || !itemExists(channel.ID, playlistChUrl.Url) {
			_, err = stmt.Exec(channel.ID, playlistCh.ID)
			if err != nil {
				log.Debug().Msgf("matchPlaylistTvgid insert: %v", err)
				continue // Continue with other inserts even if one fails
			}
			insertCount++
		}
	}

	// Only commit if we have successful inserts
	if insertCount > 0 {
		if err = tx.Commit(); err != nil {
			log.Error().Msgf("Failed to commit transaction: %v", err)
			return err
		}
		log.Debug().Msgf("matchPlaylistTvgid: Successfully inserted %d matches for channel ID %d", insertCount, playlistCh.ID)
	} else {
		tx.Rollback() // No need to commit an empty transaction
		log.Debug().Msgf("matchPlaylistTvgid: No new matches to insert for channel ID %d", playlistCh.ID)
	}

	return nil
}

func matchPlaylistChannelName(playlistCh models.PlaylistChannel) error {
	var err error
	maxRetries := 3
	retryDelay := 200 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		err = executeMatchPlaylistChannelName(playlistCh)
		if err == nil {
			return nil
		}

		if i < maxRetries-1 {
			log.Warn().Msgf("Retry %d: matchPlaylistChannelName failed: %v", i+1, err)
			time.Sleep(retryDelay)
			retryDelay *= 2
		}
	}

	return err
}

func executeMatchPlaylistChannelName(playlistCh models.PlaylistChannel) error {
	var wg sync.WaitGroup
	var err error
	var chMatch int64
	var score float64
	chunkSize := 30
	var playlistChUrl *models.ChannelUrl

	// Get channel URL, but handle the case where it might not exist
	playlistChUrl, err = database.Db.GetChannelUrl(playlistCh.ID)
	if err != nil {
		log.Debug().Msgf("matchPlaylistChannelName: %v", err)
		// Continue without URL - we'll handle nil check later
	}

	channelVector, err := database.Db.GetChannelVectorByName(playlistCh.Title)
	if err != nil {
		log.Debug().Msgf("matchPlaylistChannelName: %v", err)
		vectorId := UpdatePlaylistVector(playlistCh)
		if channelVector, err = database.Db.GetChannelVector(vectorId); err != nil {
			log.Debug().Msgf("matchPlaylistChannelName: %v", err)
			return err
		}
	}

	templateVectors, err := database.Db.GetTemplateChannelVectors()
	if err != nil {
		log.Debug().Msgf("matchPlaylistChannelName: %v", err)
		return err
	}

	// Use a more efficient chunking approach
	numChunks := int(math.Ceil(float64(len(templateVectors)) / float64(chunkSize)))
	results := make(chan struct {
		id    int64
		score float64
	}, numChunks)

	// Process chunks in parallel
	for i := 0; i < len(templateVectors); i += chunkSize {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()

			end := start + chunkSize
			if end > len(templateVectors) {
				end = len(templateVectors)
			}

			var bestId int64
			var bestScore float64

			for j := start; j < end; j++ {
				vector, err := database.Db.GetChannelVector(templateVectors[j].VectorId)
				if err == nil {
					if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
						if cosine >= settings.APP_SETTINGS.Playlist.Name_score && cosine > bestScore {
							bestScore = cosine
							bestId = templateVectors[j].ChannelId
						}
					}
				}
			}

			if bestId != 0 {
				results <- struct {
					id    int64
					score float64
				}{bestId, bestScore}
			}
		}(i)
	}

	// Collect results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Find the best match across all chunks
	for result := range results {
		if result.score > score {
			score = result.score
			chMatch = result.id
		}
	}

	// Only proceed if we found a match and either we don't have a URL or the item doesn't exist
	if chMatch != 0 && (playlistChUrl == nil || !itemExists(chMatch, playlistChUrl.Url)) {
		tx, err := database.Db.VectorQueries.Beginx()
		if err != nil {
			log.Error().Msgf("Failed to begin transaction: %v", err)
			return err
		}

		stmt, err := tx.Prepare("INSERT INTO templatechannelitem (channel_id, playlist_channel_id) VALUES (?, ?)")
		if err != nil {
			log.Error().Msgf("Failed to prepare statement: %v", err)
			tx.Rollback()
			return err
		}

		_, err = stmt.Exec(chMatch, playlistCh.ID)
		if err != nil {
			log.Debug().Msgf("matchPlaylistChannelName: %v", err)
			tx.Rollback()
			return err
		}

		if err = tx.Commit(); err != nil {
			log.Error().Msgf("Failed to commit transaction: %v", err)
			return err
		}
	}

	return nil
}

// Search if tvgid matches for template channel and add to template if match
func matchTemplateTvgid(templateCh *models.TemplateChannel) ([]models.PlaylistChannel, error) {
	if templateCh.TvgID != nil {
		channels, err := database.Db.GetPlChannelsByTvgID(*templateCh.TvgID)
		if err != nil {
			log.Debug().Msgf("matchTemplateTvgid GetPlChannelsByTvgID: %v", err)
			return nil, err
		}

		// Check if channels is empty
		if len(channels) == 0 {
			log.Debug().Msgf("matchTemplateTvgid: No playlist channels found for tvgid %s", *templateCh.TvgID)
			return channels, nil // Return empty slice but no error
		}

		// Use transaction for batch operations
		tx, err := database.Db.VectorQueries.Beginx()
		if err != nil {
			log.Error().Msgf("Failed to begin transaction: %v", err)
			return nil, err
		}

		stmt, err := tx.Prepare("INSERT INTO templatechannelitem (channel_id, playlist_channel_id) VALUES (?, ?)")
		if err != nil {
			log.Error().Msgf("Failed to prepare statement: %v", err)
			tx.Rollback()
			return nil, err
		}

		insertCount := 0
		for _, channel := range channels {
			playlistChUrl, err := database.Db.GetChannelUrl(channel.ID)
			if err != nil {
				log.Debug().Msgf("matchTemplateTvgid GetChannelUrl: %v", err)
				continue
			} else if playlistChUrl != nil && itemExists(channel.ID, playlistChUrl.Url) {
				continue
			}

			_, err = stmt.Exec(templateCh.ID, channel.ID)
			if err != nil {
				log.Debug().Msgf("matchTemplateTvgid insert: %v", err)
				continue
			}
			insertCount++
		}

		// Only commit if we have successful inserts
		if insertCount > 0 {
			if err = tx.Commit(); err != nil {
				log.Error().Msgf("Failed to commit transaction: %v", err)
				tx.Rollback()
				return nil, err
			}
			log.Debug().Msgf("matchTemplateTvgid: Successfully inserted %d matches for template channel ID %d", insertCount, templateCh.ID)
		} else {
			tx.Rollback()
			log.Debug().Msgf("matchTemplateTvgid: No new matches to insert for template channel ID %d", templateCh.ID)
		}

		return channels, nil
	} else {
		return nil, errors.New("matchTemplateTvgid: Tvgid is NULL")
	}
}

func matchTemplateChannelName(templateCh *models.TemplateChannel) error {
	ctx := context.Background()
	vectorMatch := struct {
		match models.VectorMatch
		Mu    sync.Mutex
	}{}
	var wg sync.WaitGroup
	var err error
	chunkSize := 50 // Increased from 10 to 50

	channelVector, err := database.Db.GetChannelVectorByName(templateCh.Name)
	if err != nil {
		log.Debug().Msgf("matchTemplateChannelName: %v", err)
		vectorId := UpdateTemplateVector(templateCh)
		if channelVector, err = database.Db.GetChannelVector(vectorId); err != nil {
			log.Debug().Msgf("matchTemplateChannelName: %v", err)
			return err
		}
	}

	playlists, err := database.Db.GetPlaylists()
	if err != nil {
		log.Debug().Msgf("matchTemplateChannelName: %v", err)
		return err
	}

	// Begin a transaction for batch inserts
	tx, err := database.Db.VectorQueries.Beginx()
	if err != nil {
		log.Error().Msgf("Failed to begin transaction: %v", err)
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO templatechannelitem (channel_id, playlist_channel_id) VALUES (?, ?)")
	if err != nil {
		log.Error().Msgf("Failed to prepare statement: %v", err)
		tx.Rollback()
		return err
	}

	matchFound := false

	for _, playlist := range *playlists {
		if exists, err := database.Db.TmplChannelExists(templateCh.ID, playlist.ID); err != nil || exists {
			continue
		}

		playlistVectors, err := database.Db.GetPlChVectorsByPlaylist(playlist.ID)
		if err != nil {
			log.Debug().Msgf("matchTemplateChannelName: %v", err)
			continue
		}

		// Reset vector match for each playlist
		vectorMatch.match = models.VectorMatch{Id: 0, Score: 0}

		// Use a more efficient chunking approach
		numChunks := int(math.Ceil(float64(len(playlistVectors)) / float64(chunkSize)))
		results := make(chan struct {
			id    int64
			score float64
		}, numChunks)

		// Process chunks in parallel
		for i := 0; i < len(playlistVectors); i += chunkSize {
			wg.Add(1)
			go func(start int, ctx context.Context) {
				defer wg.Done()

				end := start + chunkSize
				if end > len(playlistVectors) {
					end = len(playlistVectors)
				}

				var bestId int64
				var bestScore float64

				for j := start; j < end; j++ {
					vector, err := database.Db.GetChannelVector(playlistVectors[j].VectorId)
					if err == nil {
						if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
							if cosine >= settings.APP_SETTINGS.Playlist.Name_score && cosine > bestScore {
								bestScore = cosine
								bestId = playlistVectors[j].ChannelId
							}
						}
					}
				}

				if bestId != 0 {
					results <- struct {
						id    int64
						score float64
					}{bestId, bestScore}
				}
			}(i, ctx)
		}

		// Collect results
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		// Find the best match across all chunks
		for {
			select {
			case result, ok := <-results:
				if !ok {
					continue
				}
				vectorMatch.Mu.Lock()
				if result.score > vectorMatch.match.Score {
					vectorMatch.match = models.VectorMatch{Id: result.id, Score: result.score}
				}
				vectorMatch.Mu.Unlock()
			case <-done:
				goto finishPlaylist
			}
		}

	finishPlaylist:
		if vectorMatch.match.Id != 0 {
			_, err = stmt.Exec(templateCh.ID, vectorMatch.match.Id)
			if err != nil {
				log.Debug().Msgf("matchTemplateChannelName: %v", err)
				continue
			}
			matchFound = true
		}
	}

	if matchFound {
		if err = tx.Commit(); err != nil {
			log.Error().Msgf("Failed to commit transaction: %v", err)
			tx.Rollback()
			return err
		}
	} else {
		tx.Rollback()
	}

	return nil
}

func itemExists(tmplId int64, plUrl string) bool {
	// If URL is empty, we can't check for existence
	if plUrl == "" {
		return false
	}

	items, err := database.Db.GetTmplChannelItemsByCh(tmplId)
	if err != nil {
		log.Debug().Msgf("itemExists: GetTmplChannelItemsByCh: %v", err)
		return false
	}

	if items == nil || len(*items) == 0 {
		return false
	}

	for _, item := range *items {
		itemUrl, err := database.Db.GetChannelUrl(item.PlaylistChannelId)
		if err != nil {
			log.Debug().Msgf("itemExists: GetChannelUrl: %v", err)
			continue
		}

		if itemUrl != nil && itemUrl.Url == plUrl {
			return true
		}
	}
	return false
}

func TopChannelMatches(templateCh *models.TemplateChannel) ([]models.VectorMatch, error) {
	var wg sync.WaitGroup
	var err error
	chunkSize := 50 // Increased from 10 to 50

	channelVector, err := database.Db.GetChannelVectorByName(templateCh.Name)
	if err != nil {
		log.Debug().Msgf("GetChannelVectorByName:, %v", err)
		vectorId := UpdateTemplateVector(templateCh)
		if channelVector, err = database.Db.GetChannelVector(vectorId); err != nil {
			log.Debug().Msgf("GetChannelVector:, %v", err)
			return nil, err
		}
	}

	playlistVectors, err := database.Db.GetPlaylistChannelVectors()
	if err != nil {
		log.Debug().Msgf("GetPlaylistChannelVectors:, %v", err)
		return nil, err
	}

	// Use a more efficient chunking approach
	numChunks := int(math.Ceil(float64(len(playlistVectors)) / float64(chunkSize)))
	results := make(chan []models.VectorMatch, numChunks)

	// Process chunks in parallel
	for i := 0; i < len(playlistVectors); i += chunkSize {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()

			end := start + chunkSize
			if end > len(playlistVectors) {
				end = len(playlistVectors)
			}

			localMatches := make([]models.VectorMatch, 0, 5)

			for j := start; j < end; j++ {
				if vector, err := database.Db.GetChannelVector(playlistVectors[j].VectorId); err == nil {
					if cosine, err := CosineMatch(channelVector.Vector, vector.Vector); err == nil {
						if len(localMatches) < 5 {
							localMatches = append(localMatches, models.VectorMatch{Id: playlistVectors[j].ChannelId, Score: cosine})

							// Sort after adding
							sort.SliceStable(localMatches, func(i, j int) bool {
								return localMatches[i].Score > localMatches[j].Score
							})
						} else if cosine > localMatches[len(localMatches)-1].Score {
							// Replace lowest score
							localMatches[len(localMatches)-1] = models.VectorMatch{Id: playlistVectors[j].ChannelId, Score: cosine}

							// Sort after replacing
							sort.SliceStable(localMatches, func(i, j int) bool {
								return localMatches[i].Score > localMatches[j].Score
							})
						}
					}
				}
			}

			results <- localMatches
		}(i)
	}

	// Collect and merge results
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
		close(results)
	}()

	// Collect all chunk results
	allMatches := make([]models.VectorMatch, 0, 5)
	for {
		select {
		case chunkMatches, ok := <-results:
			if !ok {
				continue
			}
			// Merge with main results
			allMatches = append(allMatches, chunkMatches...)

			// Keep only top 5
			if len(allMatches) > 5 {
				sort.SliceStable(allMatches, func(i, j int) bool {
					return allMatches[i].Score > allMatches[j].Score
				})
				allMatches = allMatches[:5]
			}
		case <-done:
			// Ensure sorted
			sort.SliceStable(allMatches, func(i, j int) bool {
				return allMatches[i].Score > allMatches[j].Score
			})
			return allMatches, nil
		}
	}
}
