package utils

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"

	"github.com/anush008/fastembed-go"
	"github.com/rs/zerolog/log"
)

// Global embedding model instance
var (
	embeddingModel     *fastembed.FlagEmbedding
	embeddingModelOnce sync.Once
)

// Cache for embeddings to avoid recomputing the same vectors
var (
	embeddingCache        = make(map[string][]float64)
	embeddingCacheMux     sync.RWMutex
	vectorizationQueue    = make(chan vectorizationRequest, 500) // Reduced from 500 to 100
	batchSize             = 20                                   // Reduced from 50 to 20
	cachePath             = "./vector_cache"                     // Path for persistent cache
	cacheInitialized      = false
	cacheInitMux          sync.Mutex
	batchProcessorStarted = false
	batchProcessorMux     sync.Mutex
	dbOpSemaphore         = make(chan struct{}, 5) // Added semaphore to limit concurrent DB operations
)

// Initialize the embedding model
func getEmbeddingModel() (*fastembed.FlagEmbedding, error) {
	var initErr error
	embeddingModelOnce.Do(func() {
		// Initialize with default options (using AllMiniLML6V2 model)
		model, err := fastembed.NewFlagEmbedding(&fastembed.InitOptions{
			Model: fastembed.BGESmallENV15,
		})
		if err != nil {
			initErr = err
			return
		}
		embeddingModel = model
	})

	if initErr != nil {
		return nil, initErr
	}

	return embeddingModel, nil
}

// Initialize batch processor - safe to call multiple times
func startBatchProcessor() {
	batchProcessorMux.Lock()
	defer batchProcessorMux.Unlock()

	if !batchProcessorStarted {
		go batchProcessor()
		batchProcessorStarted = true
	}
}

// Initialize cache from disk - exposed so it can be called after DB is initialized
func InitializeCache() {
	// Start the batch processor first
	startBatchProcessor()

	cacheInitMux.Lock()
	defer cacheInitMux.Unlock()

	if cacheInitialized {
		return
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cachePath, 0755); err != nil {
		log.Error().Msgf("Failed to create vector cache directory: %v", err)
	}

	// Try to load cache from database only if database is already initialized
	if database.Db != nil {
		// Get top used vectors from database to pre-populate cache
		channelVectors, err := database.Db.GetFrequentlyUsedVectors(context.Background(), 200) // Get top 200 vectors
		if err != nil {
			log.Error().Msgf("Failed to load vectors from database: %v", err)
		} else {
			embeddingCacheMux.Lock()
			for _, vector := range channelVectors {
				embeddingCache[vector.Name] = vector.Vector
			}
			embeddingCacheMux.Unlock()
			log.Info().Msgf("Loaded %d vectors into cache", len(channelVectors))
		}
	} else {
		log.Warn().Msg("Database not initialized yet, skipping cache preload")
	}

	cacheInitialized = true
}

type vectorizationRequest struct {
	text     string
	resultCh chan<- vectorizationResult
}

type vectorizationResult struct {
	vector []float64
	err    error
}

// Batch processor goroutine
func batchProcessor() {
	for {
		// Collect batch of requests
		batch := make([]vectorizationRequest, 0, batchSize)
		request := <-vectorizationQueue
		batch = append(batch, request)

		// Try to collect more requests up to batchSize with shorter wait time
		batchTimer := time.NewTimer(20 * time.Millisecond) // Reduced from 50ms to 20ms
	collectLoop:
		for len(batch) < batchSize {
			select {
			case req := <-vectorizationQueue:
				batch = append(batch, req)
			case <-batchTimer.C:
				break collectLoop
			}
		}

		// Process the batch
		processBatch(batch)
	}
}

func processBatch(batch []vectorizationRequest) {
	// First check cache for any existing results
	textsToProcess := make([]string, 0, len(batch))
	cacheResults := make(map[int][]float64)
	requestIndices := make(map[string][]int) // Map text to request indices

	for i, req := range batch {
		embeddingCacheMux.RLock()
		vector, found := embeddingCache[req.text]
		embeddingCacheMux.RUnlock()

		if found {
			cacheResults[i] = vector
		} else {
			// Track indices of requests with the same text to avoid duplicate processing
			if indices, exists := requestIndices[req.text]; exists {
				requestIndices[req.text] = append(indices, i)
			} else {
				requestIndices[req.text] = []int{i}
				textsToProcess = append(textsToProcess, req.text)
			}
		}
	}

	// If all results were in cache, return immediately
	if len(textsToProcess) == 0 {
		for i, req := range batch {
			req.resultCh <- vectorizationResult{
				vector: cacheResults[i],
				err:    nil,
			}
		}
		return
	}

	// Process remaining texts
	vectors, err := VectorizeStringBatch(textsToProcess)

	// Handle any processing errors
	if err != nil {
		// Only send error for unprocessed requests
		for i, req := range batch {
			if _, found := cacheResults[i]; !found {
				req.resultCh <- vectorizationResult{
					vector: nil,
					err:    err,
				}
			} else {
				// Send cached result
				req.resultCh <- vectorizationResult{
					vector: cacheResults[i],
					err:    nil,
				}
			}
		}
		return
	}

	// Validate the vectors
	if len(vectors) > 0 {
		// Check if vectors are valid - AllMiniLML6V2 model uses 384-dimensional vectors
		for i, vector := range vectors {
			// Check if vector has valid dimensions
			if len(vector) == 0 || len(vector) != 384 {
				log.Error().Msgf("Invalid vector dimension for text '%s': got %d, expected 384",
					textsToProcess[i], len(vector))

				// Replace with fallback by processing this text individually
				if singleVector, singleErr := VectorizeStringSingle(textsToProcess[i]); singleErr == nil {
					vectors[i] = singleVector
					log.Info().Msgf("Successfully fallback to single vectorization for '%s'", textsToProcess[i])
				} else {
					// If even single vectorization fails, send error to all affected requests
					for _, idx := range requestIndices[textsToProcess[i]] {
						batch[idx].resultCh <- vectorizationResult{
							vector: nil,
							err:    errors.New("failed to create valid embedding vector"),
						}
					}
					continue
				}
			}

			// If batch contains multiple items, check for suspicious similarity
			if len(vectors) > 1 && i < len(vectors)-1 {
				// Compare with next vector
				if cosine, err := CosineMatch(vector, vectors[i+1]); err == nil && cosine > 0.99 {
					// Log high similarity between vectors for different texts
					if textsToProcess[i] != textsToProcess[i+1] {
						log.Warn().Msgf("Suspiciously high similarity (%.4f) between '%s' and '%s'",
							cosine, textsToProcess[i], textsToProcess[i+1])
					}
				}
			}
		}
	}

	// Match results back to requests
	embeddingCacheMux.Lock()
	for i, text := range textsToProcess {
		vector := vectors[i]
		// Cache the result
		embeddingCache[text] = vector

		// Save to database in a controlled manner
		go saveVectorToDB(text, vector)

		// Send result to all requests with this text
		for _, idx := range requestIndices[text] {
			batch[idx].resultCh <- vectorizationResult{
				vector: vector,
				err:    nil,
			}
		}
	}
	embeddingCacheMux.Unlock()

	// Send cached results for remaining requests
	for i, req := range batch {
		if vector, found := cacheResults[i]; found {
			// Send cached result
			req.resultCh <- vectorizationResult{
				vector: vector,
				err:    nil,
			}
		}
	}
}

func PlaylistVectorQueue(in <-chan models.PlaylistChannel) {
	bufChan := make(chan struct{}, 10) // Reduced from 20 to 10
	for playlistCh := range in {
		bufChan <- struct{}{}
		go func(playlistCh models.PlaylistChannel) {
			defer func() {
				<-bufChan
			}()

			UpdatePlaylistVector(playlistCh)
			// Add delay before matching to reduce contention
			time.Sleep(50 * time.Millisecond)
			// Decouple matching to run after all vectors are processed
			go MatchPlaylistChannel(playlistCh)
		}(playlistCh)
	}
}

func TemplateVectorQueue(in <-chan models.TemplateChannel) {
	bufChan := make(chan struct{}, 10) // Reduced from 20 to 10
	for templateCh := range in {
		bufChan <- struct{}{}
		go func(templateCh models.TemplateChannel) {
			defer func() {
				<-bufChan
			}()

			UpdateTemplateVector(&templateCh)
			// Add delay before matching to reduce contention
			time.Sleep(50 * time.Millisecond)
			// Decouple matching to run after all vectors are processed
			go MatchTemplateChannel(&templateCh)
		}(templateCh)
	}
}

// returns vector id
func UpdatePlaylistVector(playlistCh models.PlaylistChannel) int64 {
	// Add guard to prevent processing channels without IDs
	if playlistCh.ID == 0 {
		log.Warn().Msg("Attempted to vectorize channel with ID 0, skipping")
		return 0
	}

	if vectorId, err := getChannelVector(playlistCh.Title); err != nil {
		log.Warn().Msgf("VECTORIZE_PLAYLIST_STRING: %v", err)
		return 0
	} else {
		// Use the new retry-enabled association function
		if err := associateVectorWithChannel(playlistCh.ID, vectorId); err != nil {
			log.Error().Msgf("Failed to associate vector with channel: %v", err)
			return 0
		}
		return vectorId
	}
}

// returns vector id
func UpdateTemplateVector(templateCh *models.TemplateChannel) int64 {
	// Add guard to prevent processing channels without IDs
	if templateCh.ID == 0 {
		log.Warn().Msg("Attempted to vectorize template channel with ID 0, skipping")
		return 0
	}

	if vectorId, err := getChannelVector(templateCh.Name); err != nil {
		log.Warn().Msgf("VECTORIZE_TEMPLATE_STRING: %v", err)
		return 0
	} else {
		// Check if association exists and create/update it with retry logic
		// We'll create a similar helper function for template channel association
		if err := associateTemplateVectorWithChannel(templateCh.ID, vectorId); err != nil {
			log.Error().Msgf("Failed to associate vector with template channel: %v", err)
			return 0
		}
		return vectorId
	}
}

func getChannelVector(name string) (int64, error) {

	// First check cache for the channel name
	embeddingCacheMux.RLock()
	vector, found := embeddingCache[name]
	embeddingCacheMux.RUnlock()

	// Check if vector already exists in database
	channelVector, err := database.Db.GetChannelVectorByName(name)
	if err == nil {
		// Vector found in the database, update the cache if needed
		if !found {
			embeddingCacheMux.Lock()
			embeddingCache[name] = channelVector.Vector
			embeddingCacheMux.Unlock()
		}
		return channelVector.ID, nil
	}

	// Need to create new vector
	if !found {
		vector, err = VectorizeString(name)
		if err != nil {
			return 0, err
		}

		// Add to cache
		embeddingCacheMux.Lock()
		embeddingCache[name] = vector
		embeddingCacheMux.Unlock()
	}

	// Create in database
	channelVector = &models.ChannelVector{Name: name, Vector: vector}
	channelVector.ID, err = database.Db.CreateChannelVector(channelVector)
	if err != nil {
		return 0, err
	}
	log.Info().Msgf("VECTOR_TOOLS: ADDED VECTOR FOR %s", name)

	return channelVector.ID, nil
}

// Direct implementation for single string vectorization (used internally)
func VectorizeStringSingle(text string) ([]float64, error) {
	model, err := getEmbeddingModel()
	if err != nil {
		return nil, err
	}

	// Add passage prefix for better embedding quality if not already prefixed
	if !strings.HasPrefix(text, "passage:") && !strings.HasPrefix(text, "query:") {
		text = "passage: " + text
	}

	// Use QueryEmbed for single text processing
	embedding32, err := model.QueryEmbed(text)
	if err != nil {
		return nil, err
	}

	// Convert float32 to float64
	embedding := make([]float64, len(embedding32))
	for i, v := range embedding32 {
		embedding[i] = float64(v)
	}

	return embedding, nil
}

// Use the queueing system for vectorization
func VectorizeString(text string) ([]float64, error) {
	// Ensure batch processor is started
	startBatchProcessor()

	resultCh := make(chan vectorizationResult, 1)
	vectorizationQueue <- vectorizationRequest{
		text:     text,
		resultCh: resultCh,
	}

	// Wait for the result
	result := <-resultCh
	return result.vector, result.err
}

// Batch implementation for vectorizing multiple strings at once
func VectorizeStringBatch(texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	model, err := getEmbeddingModel()
	if err != nil {
		return nil, err
	}

	// Add passage prefix for better embedding quality if not already prefixed
	prefixedTexts := make([]string, len(texts))
	for i, text := range texts {
		if !strings.HasPrefix(text, "passage:") && !strings.HasPrefix(text, "query:") {
			prefixedTexts[i] = "passage: " + text
		} else {
			prefixedTexts[i] = text
		}
	}

	// Use Embed for batch processing
	embeddings32, err := model.Embed(prefixedTexts, batchSize)
	if err != nil {
		log.Error().Msgf("Failed to process batch embedding: %v", err)
		// Fall back to processing individually
		log.Warn().Msg("Falling back to individual vector processing")
		results := make([][]float64, len(texts))
		for i, text := range texts {
			if vector, err := VectorizeStringSingle(text); err == nil {
				results[i] = vector
			} else {
				log.Error().Msgf("Failed to vectorize text '%s': %v", text, err)
				results[i] = make([]float64, 0) // Empty vector for failed item
			}
		}
		return results, nil
	}

	// Convert float32 to float64
	embeddings := make([][]float64, len(embeddings32))
	for i, embedding32 := range embeddings32 {
		embeddings[i] = make([]float64, len(embedding32))
		for j, v := range embedding32 {
			embeddings[i][j] = float64(v)
		}
	}

	return embeddings, nil
}

func CosineMatch(a, b []float64) (cosine float64, err error) {
	dotProduct := 0.0
	aSquared := 0.0
	bSquared := 0.0

	for i := 0; i < len(a) && i < len(b); i++ {
		dotProduct += a[i] * b[i]
		aSquared += a[i] * a[i]
		bSquared += b[i] * b[i]
	}

	// Calculate cosine without rounding to preserve full precision
	similarity := math.Round(dotProduct/(math.Sqrt(aSquared*bSquared))*10000) / 10000

	// Add debug output for nearly identical vectors (but not exactly identical)
	if similarity > 0.999 && similarity < 1.0 {
		// Calculate vector difference statistics to help diagnose the issue
		var sumDiffSquared float64
		var maxDiff float64

		for i := 0; i < len(a) && i < len(b); i++ {
			diff := math.Abs(a[i] - b[i])
			sumDiffSquared += diff * diff
			if diff > maxDiff {
				maxDiff = diff
			}
		}

		// Output detailed debug info when vectors are suspiciously similar
		log.Debug().Msgf("Suspicious vector similarity: %.6f, max element diff: %.6f, euclidean distance: %.6f",
			similarity, maxDiff, math.Sqrt(sumDiffSquared))
	}

	return similarity, nil
}

// Add DB operation semaphore to limit concurrent DB operations
func acquireDBSemaphore() {
	dbOpSemaphore <- struct{}{}
}

func releaseDBSemaphore() {
	<-dbOpSemaphore
}

// Save vector to database with retry logic and concurrency limiting
func saveVectorToDB(text string, vector []float64) {
	// Limit concurrent DB operations
	acquireDBSemaphore()
	defer releaseDBSemaphore()

	// Check if vector exists in DB first
	if _, err := database.Db.GetChannelVectorByName(text); err == nil {
		// Vector already exists, no need to create
		return
	}

	// Create new vector in database with retry logic
	channelVector := &models.ChannelVector{Name: text, Vector: vector}

	if _, err := database.Db.CreateChannelVector(channelVector); err != nil {
		log.Warn().Msgf("Failed to save vector %s to database: %v", text, err)
	}
}

// associateVectorWithChannel attempts to associate a vector with a channel with retry logic
func associateVectorWithChannel(channelId int64, vectorId int64) error {

	if channelId == 0 {
		return errors.New("invalid channel ID (0)")
	}

	if vectorId == 0 {
		return errors.New("invalid vector ID (0)")
	}

	maxRetries := 10
	baseDelay := 500 * time.Millisecond

	for retry := 0; retry < maxRetries; retry++ {
		// Use a transaction for the operation
		tx, err := database.Db.VectorQueries.Beginx()
		if err != nil {
			if retry < maxRetries-1 {
				delay := baseDelay * time.Duration(1<<uint(retry))
				time.Sleep(delay)
				continue
			}
			return err
		}

		// Check if an association already exists
		var existingId int64
		row := tx.QueryRow("SELECT id FROM playlistchannelvectors WHERE channel_id = ? LIMIT 1", channelId)
		err = row.Scan(&existingId)

		var stmt *sql.Stmt
		if err == sql.ErrNoRows {
			// Create new association
			stmt, err = tx.Prepare("INSERT INTO playlistchannelvectors VALUES (null, ?, ?)")
			if err != nil {
				tx.Rollback()
				if retry < maxRetries-1 {
					delay := baseDelay * time.Duration(1<<uint(retry))
					time.Sleep(delay)
					continue
				}
				return err
			}

			_, err = stmt.Exec(channelId, vectorId)
		} else if err == nil {
			// Update existing association
			stmt, err = tx.Prepare("UPDATE playlistchannelvectors SET vector_id = ? WHERE channel_id = ?")
			if err != nil {
				tx.Rollback()
				if retry < maxRetries-1 {
					delay := baseDelay * time.Duration(1<<uint(retry))
					time.Sleep(delay)
					continue
				}
				return err
			}

			_, err = stmt.Exec(vectorId, channelId)
		} else {
			// Database error
			tx.Rollback()
			if retry < maxRetries-1 && (strings.Contains(err.Error(), "database is locked") ||
				strings.Contains(err.Error(), "busy")) {
				delay := baseDelay * time.Duration(1<<uint(retry))
				time.Sleep(delay)
				continue
			}
			return err
		}

		// Check for execution errors
		if err != nil {
			tx.Rollback()
			if retry < maxRetries-1 && (strings.Contains(err.Error(), "database is locked") ||
				strings.Contains(err.Error(), "busy") ||
				strings.Contains(err.Error(), "FOREIGN KEY constraint failed")) {
				delay := baseDelay * time.Duration(1<<uint(retry))
				time.Sleep(delay)
				continue
			}
			return err
		}

		// Commit the transaction
		if err := tx.Commit(); err != nil {
			if retry < maxRetries-1 && (strings.Contains(err.Error(), "database is locked") ||
				strings.Contains(err.Error(), "busy")) {
				delay := baseDelay * time.Duration(1<<uint(retry))
				time.Sleep(delay)
				continue
			}
			return err
		}

		// Success
		return nil
	}

	return errors.New("failed to associate vector with channel after maximum retries")
}

// associateTemplateVectorWithChannel attempts to associate a vector with a template channel with retry logic
func associateTemplateVectorWithChannel(channelId int64, vectorId int64) error {

	if channelId == 0 {
		return errors.New("invalid template channel ID (0)")
	}

	if vectorId == 0 {
		return errors.New("invalid vector ID (0)")
	}

	maxRetries := 10
	baseDelay := 500 * time.Millisecond

	for retry := 0; retry < maxRetries; retry++ {
		// Use a transaction for the operation
		tx, err := database.Db.VectorQueries.Beginx()
		if err != nil {
			if retry < maxRetries-1 {
				delay := baseDelay * time.Duration(1<<uint(retry))
				time.Sleep(delay)
				continue
			}
			return err
		}

		// Check if an association already exists
		var existingId int64
		row := tx.QueryRow("SELECT id FROM templatechannelvectors WHERE channel_id = ? LIMIT 1", channelId)
		err = row.Scan(&existingId)

		var stmt *sql.Stmt
		if err == sql.ErrNoRows {
			// Create new association
			stmt, err = tx.Prepare("INSERT INTO templatechannelvectors VALUES (null, ?, ?)")
			if err != nil {
				tx.Rollback()
				if retry < maxRetries-1 {
					delay := baseDelay * time.Duration(1<<uint(retry))
					time.Sleep(delay)
					continue
				}
				return err
			}

			_, err = stmt.Exec(channelId, vectorId)
		} else if err == nil {
			// Update existing association
			stmt, err = tx.Prepare("UPDATE templatechannelvectors SET vector_id = ? WHERE channel_id = ?")
			if err != nil {
				tx.Rollback()
				if retry < maxRetries-1 {
					delay := baseDelay * time.Duration(1<<uint(retry))
					time.Sleep(delay)
					continue
				}
				return err
			}

			_, err = stmt.Exec(vectorId, channelId)
		} else {
			// Database error
			tx.Rollback()
			if retry < maxRetries-1 && (strings.Contains(err.Error(), "database is locked") ||
				strings.Contains(err.Error(), "busy")) {
				delay := baseDelay * time.Duration(1<<uint(retry))
				time.Sleep(delay)
				continue
			}
			return err
		}

		// Check for execution errors
		if err != nil {
			tx.Rollback()
			if retry < maxRetries-1 && (strings.Contains(err.Error(), "database is locked") ||
				strings.Contains(err.Error(), "busy") ||
				strings.Contains(err.Error(), "FOREIGN KEY constraint failed")) {
				delay := baseDelay * time.Duration(1<<uint(retry))
				time.Sleep(delay)
				continue
			}
			return err
		}

		// Commit the transaction
		if err := tx.Commit(); err != nil {
			if retry < maxRetries-1 && (strings.Contains(err.Error(), "database is locked") ||
				strings.Contains(err.Error(), "busy")) {
				delay := baseDelay * time.Duration(1<<uint(retry))
				time.Sleep(delay)
				continue
			}
			return err
		}

		// Success
		return nil
	}

	return errors.New("failed to associate vector with template channel after maximum retries")
}
