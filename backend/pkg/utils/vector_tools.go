package utils

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

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
	embeddingCache          = make(map[string][]float64)
	embeddingCacheMux       sync.RWMutex
	vectorizationQueue      = make(chan vectorizationRequest, 200)
	cacheInitialized        = false
	cacheInitMux            sync.Mutex
	batchProcessorStarted   = false
	batchProcessorMux       sync.Mutex
	dbOpSemaphore           = make(chan struct{}, 3)
	dbOperationQueue        = make(chan dbOperation, 100)
	dbQueueProcessorStarted = false
	dbQueueProcessorMux     sync.Mutex
)

// Database operation types for queuing
type dbOperation struct {
	operationType string
	channelId     int64
	vectorId      int64
	resultCh      chan<- error
}

// Start database operation queue processor
func startDBQueueProcessor() {
	dbQueueProcessorMux.Lock()
	defer dbQueueProcessorMux.Unlock()

	if !dbQueueProcessorStarted {
		go dbOperationProcessor()
		dbQueueProcessorStarted = true
		log.Info().Msg("Database operation queue processor started")
	}
}

func dbOperationProcessor() {
	for op := range dbOperationQueue {
		var err error

		switch op.operationType {
		case "associate_playlist_vector":
			err = associateVectorWithChannelDirect(op.channelId, op.vectorId)
		case "associate_template_vector":
			err = associateTemplateVectorWithChannelDirect(op.channelId, op.vectorId)
		default:
			err = errors.New("unknown operation type")
		}

		// Send result back
		select {
		case op.resultCh <- err:
		case <-time.After(5 * time.Second):
			log.Warn().Str("operation", op.operationType).Msg("Timeout sending result back")
		}
	}
}

// Initialize the embedding model
func getEmbeddingModel() (*fastembed.FlagEmbedding, error) {
	var initErr error
	embeddingModelOnce.Do(func() {
		// Keep the model under the persistent writable serve volume so matching
		// works when the production root filesystem is read-only.
		model, err := fastembed.NewFlagEmbedding(&fastembed.InitOptions{
			Model:    fastembed.BGESmallENV15,
			CacheDir: filepath.Join(settings.SERVE_PATH, "model_cache"),
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

// Initialize batch processor
func startBatchProcessor() {
	batchProcessorMux.Lock()
	defer batchProcessorMux.Unlock()

	if !batchProcessorStarted {
		go batchProcessor()
		batchProcessorStarted = true
	}
}

// Initialize cache from disk
func InitializeCache() {
	// Start the batch processor first
	startBatchProcessor()

	// Start the database operation queue processor
	startDBQueueProcessor()

	cacheInitMux.Lock()
	defer cacheInitMux.Unlock()

	if cacheInitialized {
		return
	}

	// Vectors are persisted in SQLite; the process-local map is only a hot cache.
	// Keeping it database-backed avoids an obsolete writable directory outside
	// the mounted config volume when the production root filesystem is read-only.
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
		// Get batch size from settings
		batchSize := settings.Current().Vector.BatchSize
		if batchSize <= 0 {
			batchSize = 25
		}

		// Collect batch of requests
		batch := make([]vectorizationRequest, 0, batchSize)
		request := <-vectorizationQueue
		batch = append(batch, request)

		batchTimer := time.NewTimer(20 * time.Millisecond)
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
	bufChan := make(chan struct{}, 5)
	for playlistCh := range in {
		bufChan <- struct{}{}
		go func(playlistCh models.PlaylistChannel) {
			defer func() {
				<-bufChan
			}()

			// Add a small delay to ensure channel is committed to database
			time.Sleep(50 * time.Millisecond)

			UpdatePlaylistVector(playlistCh)
			// Add delay before matching to reduce contention
			time.Sleep(50 * time.Millisecond)
			// Decouple matching to run after all vectors are processed
			go MatchPlaylistChannel(playlistCh)
		}(playlistCh)
	}
}

func TemplateVectorQueue(in <-chan models.TemplateChannel) {
	bufChan := make(chan struct{}, 5)
	for templateCh := range in {
		bufChan <- struct{}{}
		go func(templateCh models.TemplateChannel) {
			defer func() {
				<-bufChan
			}()

			// Add a small delay to ensure channel is committed to database
			time.Sleep(50 * time.Millisecond)

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
		// Queued association function
		if err := associateVectorWithChannel(playlistCh.ID, vectorId); err != nil {
			log.Error().Int64("channelId", playlistCh.ID).Int64("vectorId", vectorId).Str("title", playlistCh.Title).Msgf("Failed to associate vector with channel: %v", err)
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
		// Queued association function
		if err := associateTemplateVectorWithChannel(templateCh.ID, vectorId); err != nil {
			log.Error().Int64("channelId", templateCh.ID).Int64("vectorId", vectorId).Str("name", templateCh.Name).Msgf("Failed to associate vector with template channel: %v", err)
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

// Direct implementation for single string vectorization
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

// Batch implementation for vectorizing multiple strings at once with parallel processing
func VectorizeStringBatch(texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	model, err := getEmbeddingModel()
	if err != nil {
		return nil, err
	}

	// Get batch configuration from settings
	batchSize := settings.Current().Vector.BatchSize
	if batchSize <= 0 {
		batchSize = 25
	}

	parallelBatches := settings.Current().Vector.ParallelBatches
	if parallelBatches <= 0 {
		parallelBatches = 4
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

	// Prepare for parallel processing
	totalTexts := len(prefixedTexts)
	results := make([][]float32, totalTexts)
	errChan := make(chan error, parallelBatches)
	var wg sync.WaitGroup

	// Create a semaphore to limit concurrent batches
	semaphore := make(chan struct{}, parallelBatches)

	// Process in batches
	for startIdx := 0; startIdx < totalTexts; startIdx += batchSize {
		endIdx := startIdx + batchSize
		if endIdx > totalTexts {
			endIdx = totalTexts
		}

		// Acquire semaphore slot
		semaphore <- struct{}{}

		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore slot when done

			batch := prefixedTexts[start:end]
			log.Debug().Msgf("Processing batch %d to %d (size: %d)", start, end, len(batch))

			// Process this batch
			batchEmbeddings, batchErr := model.Embed(batch, batchSize)
			if batchErr != nil {
				log.Error().Msgf("Failed to process batch %d-%d: %v", start, end, batchErr)
				errChan <- batchErr
				return
			}

			// Store results in the correct positions
			for i, embedding := range batchEmbeddings {
				results[start+i] = embedding
			}
		}(startIdx, endIdx)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check if any errors occurred
	for err := range errChan {
		if err != nil {
			log.Error().Msgf("Batch processing error: %v", err)
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
	}

	// Convert float32 to float64
	embeddings := make([][]float64, len(results))
	for i, embedding32 := range results {
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

// associateVectorWithChannel queues the association operation to avoid database locking
func associateVectorWithChannel(channelId int64, vectorId int64) error {
	if channelId == 0 {
		return errors.New("invalid channel ID (0)")
	}

	if vectorId == 0 {
		return errors.New("invalid vector ID (0)")
	}

	// Create result channel
	resultCh := make(chan error, 1)

	// Queue the operation
	select {
	case dbOperationQueue <- dbOperation{
		operationType: "associate_playlist_vector",
		channelId:     channelId,
		vectorId:      vectorId,
		resultCh:      resultCh,
	}:
		// Wait for result with timeout
		select {
		case err := <-resultCh:
			return err
		case <-time.After(30 * time.Second):
			return errors.New("timeout waiting for database operation")
		}
	case <-time.After(5 * time.Second):
		return errors.New("timeout queuing database operation")
	}
}

// associateVectorWithChannelDirect performs the actual database operation
func associateVectorWithChannelDirect(channelId int64, vectorId int64) error {
	maxRetries := 3
	baseDelay := 200 * time.Millisecond

	for retry := 0; retry < maxRetries; retry++ {
		if err := validateChannelAndVector(channelId, vectorId, "playlist"); err != nil {
			log.Debug().Int64("channelId", channelId).Int64("vectorId", vectorId).Err(err).Msg("Validation failed for playlist channel association")
			return err
		}

		newAssoc := &models.PlaylistChannelVector{
			ChannelId: channelId,
			VectorId:  vectorId,
		}

		_, err := database.Db.CreatePlaylistChannelVector(newAssoc)
		if err == nil {
			return nil // Success
		}

		log.Debug().Int64("channelId", channelId).Int64("vectorId", vectorId).Err(err).Msg("Failed to create playlist channel vector association")

		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			// Foreign key constraint means either channel or vector doesn't exist
			// Re-validate to see which one is missing
			if validateErr := validateChannelAndVector(channelId, vectorId, "playlist"); validateErr != nil {
				log.Warn().Int64("channelId", channelId).Int64("vectorId", vectorId).Err(validateErr).Msg("Foreign key constraint failed - record no longer exists")
				return validateErr
			}
			if retry < maxRetries-1 {
				delay := baseDelay * time.Duration(retry+1)
				log.Debug().Int64("channelId", channelId).Int64("vectorId", vectorId).Dur("delay", delay).Msg("Retrying after foreign key constraint failure")
				time.Sleep(delay)
				continue
			}
		}

		// Retry on locking errors
		if retry < maxRetries-1 && (strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "busy")) {
			delay := baseDelay * time.Duration(1<<uint(retry))
			time.Sleep(delay)
			continue
		}

		return err
	}

	return errors.New("failed to associate vector with channel after maximum retries")
}

// validateChannelAndVector checks if both channel and vector exist before creating association
func validateChannelAndVector(channelId int64, vectorId int64, channelType string) error {
	// Check if vector exists
	_, err := database.Db.GetChannelVector(vectorId)
	if err != nil {
		if err == sql.ErrNoRows || strings.Contains(err.Error(), "record not found") {
			return errors.New("vector not found")
		}
		return err
	}

	// Check if channel exists based on type
	if channelType == "playlist" {
		_, err = database.Db.GetPlChannel(channelId)
		if err != nil {
			if err == sql.ErrNoRows || strings.Contains(err.Error(), "record not found") {
				return errors.New("playlist channel not found")
			}
			return err
		}
	} else if channelType == "template" {
		_, err = database.Db.GetTmplChannel(channelId)
		if err != nil {
			if err == sql.ErrNoRows || strings.Contains(err.Error(), "record not found") {
				return errors.New("template channel not found")
			}
			return err
		}
	}

	return nil
}

// associateTemplateVectorWithChannel queues the association operation to avoid database locking
func associateTemplateVectorWithChannel(channelId int64, vectorId int64) error {
	if channelId == 0 {
		return errors.New("invalid template channel ID (0)")
	}

	if vectorId == 0 {
		return errors.New("invalid vector ID (0)")
	}

	// Create result channel
	resultCh := make(chan error, 1)

	// Queue the operation
	select {
	case dbOperationQueue <- dbOperation{
		operationType: "associate_template_vector",
		channelId:     channelId,
		vectorId:      vectorId,
		resultCh:      resultCh,
	}:
		// Wait for result with timeout
		select {
		case err := <-resultCh:
			return err
		case <-time.After(30 * time.Second):
			return errors.New("timeout waiting for database operation")
		}
	case <-time.After(5 * time.Second):
		return errors.New("timeout queuing database operation")
	}
}

// associateTemplateVectorWithChannelDirect performs the actual database operation
func associateTemplateVectorWithChannelDirect(channelId int64, vectorId int64) error {
	maxRetries := 3
	baseDelay := 200 * time.Millisecond

	for retry := 0; retry < maxRetries; retry++ {
		// First, validate that both the channel and vector exist
		if err := validateChannelAndVector(channelId, vectorId, "template"); err != nil {
			log.Debug().Int64("channelId", channelId).Int64("vectorId", vectorId).Err(err).Msg("Validation failed for template channel association")
			return err
		}

		newAssoc := &models.TemplateChannelVector{
			ChannelId: channelId,
			VectorId:  vectorId,
		}

		_, err := database.Db.CreateTemplateChannelVector(newAssoc)
		if err == nil {
			return nil // Success
		}

		log.Debug().Int64("channelId", channelId).Int64("vectorId", vectorId).Err(err).Msg("Failed to create template channel vector association")

		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			// Foreign key constraint means either channel or vector doesn't exist
			// Re-validate to see which one is missing
			if validateErr := validateChannelAndVector(channelId, vectorId, "template"); validateErr != nil {
				log.Warn().Int64("channelId", channelId).Int64("vectorId", vectorId).Err(validateErr).Msg("Foreign key constraint failed - record no longer exists")
				return validateErr
			}
			if retry < maxRetries-1 {
				delay := baseDelay * time.Duration(retry+1)
				log.Debug().Int64("channelId", channelId).Int64("vectorId", vectorId).Dur("delay", delay).Msg("Retrying after foreign key constraint failure")
				time.Sleep(delay)
				continue
			}
		}

		// Retry on locking errors
		if retry < maxRetries-1 && (strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "busy")) {
			delay := baseDelay * time.Duration(1<<uint(retry))
			time.Sleep(delay)
			continue
		}

		return err
	}

	return errors.New("failed to associate vector with template channel after maximum retries")
}
