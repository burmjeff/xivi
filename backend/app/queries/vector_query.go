package queries

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// SQL query constants
const (
	selectFrequentlyUsedVectorsQuery = `
		WITH vector_refs AS (
			SELECT vector_id, COUNT(*) as ref_count
			FROM (
				SELECT vector_id FROM playlistchannelvectors
				UNION ALL
				SELECT vector_id FROM templatechannelvectors
			)
			GROUP BY vector_id
		)
		SELECT cv.* FROM channelvectors cv
		JOIN vector_refs ON cv.id = vector_refs.vector_id
		ORDER BY vector_refs.ref_count DESC
		LIMIT ?
	`

	selectTemplateChannelVectorQuery     = `SELECT * FROM templatechannelvectors WHERE channel_id = ?`
	selectAllTemplateChannelVectorsQuery = `SELECT * FROM templatechannelvectors`
	insertTemplateChannelVectorQuery     = `INSERT INTO templatechannelvectors VALUES (null, :channel_id, :vector_id)`
	updateTemplateChannelVectorQuery     = `UPDATE templatechannelvectors SET vector_id = ? WHERE channel_id = ?`

	selectPlaylistChannelVectorsQuery = `
		SELECT pcv.* FROM playlistchannelvectors pcv
		JOIN playlistchannel pc ON pc.id = pcv.channel_id
		JOIN playlistgroup pg ON pg.id = pc.group_id
		WHERE pg.enabled = true
	`

	selectPlaylistChannelVectorsByPlaylistQuery = `
		SELECT pcv.* FROM playlistchannelvectors pcv
		JOIN playlistchannel pc ON pc.id = pcv.channel_id
		JOIN playlistgroup pg ON pg.id = pc.group_id
		JOIN playlist pl ON pl.id = pg.playlist_id
		WHERE pg.enabled = true AND pl.id = ?
	`

	selectPlaylistChannelVectorQuery = `SELECT * FROM playlistchannelvectors WHERE channel_id = ?`
	insertPlaylistChannelVectorQuery = `INSERT INTO playlistchannelvectors VALUES (null, :channel_id, :vector_id)`
	updatePlaylistChannelVectorQuery = `UPDATE playlistchannelvectors SET vector_id = :vector_id WHERE channel_id = :channel_id`

	selectChannelVectorQuery       = `SELECT * FROM channelvectors WHERE id = ?`
	selectChannelVectorByNameQuery = `SELECT * FROM channelvectors WHERE name = ?`
	insertChannelVectorQuery       = `INSERT INTO channelvectors VALUES (null, :name, :vector)`
)

type VectorQueries struct {
	BaseQueries
}

// Maximum cache sizes to prevent memory leaks
const (
	maxVectorNameCacheSize     = 5000
	maxVectorIdCacheSize       = 5000
	maxPlaylistVectorCacheSize = 5000
)

// startCacheCleanupRoutine starts a background routine to clean up caches periodically
func startCacheCleanupRoutine() {
	go func() {
		for {
			// Run every 5 minutes
			time.Sleep(5 * time.Minute)

			// Clean up vector name cache
			vectorNameCacheMux.Lock()
			if len(vectorNameCache) > maxVectorNameCacheSize {
				log.Info().Int("size", len(vectorNameCache)).Int("max", maxVectorNameCacheSize).Msg("Cleaning up vector name cache")
				// Remove random entries until we're under the limit
				for k := range vectorNameCache {
					delete(vectorNameCache, k)
					if len(vectorNameCache) <= maxVectorNameCacheSize*0.8 {
						break
					}
				}
			}
			vectorNameCacheMux.Unlock()

			// Clean up vector ID cache
			vectorIdCacheMux.Lock()
			if len(vectorIdCache) > maxVectorIdCacheSize {
				log.Info().Int("size", len(vectorIdCache)).Int("max", maxVectorIdCacheSize).Msg("Cleaning up vector ID cache")
				// Remove random entries until we're under the limit
				for k := range vectorIdCache {
					delete(vectorIdCache, k)
					if len(vectorIdCache) <= maxVectorIdCacheSize*0.8 {
						break
					}
				}
			}
			vectorIdCacheMux.Unlock()

			// Clean up playlist vector cache
			playlistVectorCacheMux.Lock()
			if len(playlistVectorCache) > maxPlaylistVectorCacheSize {
				log.Info().Int("size", len(playlistVectorCache)).Int("max", maxPlaylistVectorCacheSize).Msg("Cleaning up playlist vector cache")
				// Remove random entries until we're under the limit
				for k := range playlistVectorCache {
					delete(playlistVectorCache, k)
					if len(playlistVectorCache) <= maxPlaylistVectorCacheSize*0.8 {
						break
					}
				}
			}
			playlistVectorCacheMux.Unlock()
		}
	}()
}

// NewVectorQueries creates a new VectorQueries instance
func NewVectorQueries(db *sqlx.DB) *VectorQueries {
	// Start the cache cleanup routine
	startCacheCleanupRoutine()

	// Initialize prepared statements
	initPreparedStatements(db)

	// Create a new VectorQueries instance
	queries := &VectorQueries{
		BaseQueries: NewBaseQueries(db),
	}

	return queries
}

// Initialize prepared statements for better performance
func initPreparedStatements(db *sqlx.DB) {
	preparedStmtsMux.Lock()
	defer preparedStmtsMux.Unlock()

	// Always reinitialize statements to ensure they're valid
	// Close existing statements if they exist
	if insertVectorStmt != nil {
		insertVectorStmt.Close()
		insertVectorStmt = nil
	}
	if selectVectorByNameStmt != nil {
		selectVectorByNameStmt.Close()
		selectVectorByNameStmt = nil
	}
	if selectVectorByIdStmt != nil {
		selectVectorByIdStmt.Close()
		selectVectorByIdStmt = nil
	}
	if insertPlaylistVectorStmt != nil {
		insertPlaylistVectorStmt.Close()
		insertPlaylistVectorStmt = nil
	}
	if updatePlaylistVectorStmt != nil {
		updatePlaylistVectorStmt.Close()
		updatePlaylistVectorStmt = nil
	}
	if selectPlaylistVectorStmt != nil {
		selectPlaylistVectorStmt.Close()
		selectPlaylistVectorStmt = nil
	}

	// Create prepared statements with error handling
	var err error

	// Prepare insert statement
	insertVectorStmt, err = db.PrepareNamed(insertChannelVectorQuery)
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare insertVectorStmt")
	} else {
		log.Info().Msg("Successfully prepared insertVectorStmt")
	}

	// Prepare select by name statement
	selectVectorByNameStmt, err = db.Preparex(selectChannelVectorByNameQuery)
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare selectVectorByNameStmt")
	} else {
		log.Info().Msg("Successfully prepared selectVectorByNameStmt")
	}

	// Prepare select by id statement
	selectVectorByIdStmt, err = db.Preparex(selectChannelVectorQuery)
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare selectVectorByIdStmt")
	} else {
		log.Info().Msg("Successfully prepared selectVectorByIdStmt")
	}

	// Prepare insert playlist vector statement
	insertPlaylistVectorStmt, err = db.PrepareNamed(insertPlaylistChannelVectorQuery)
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare insertPlaylistVectorStmt")
	} else {
		log.Info().Msg("Successfully prepared insertPlaylistVectorStmt")
	}

	// Prepare update playlist vector statement
	updatePlaylistVectorStmt, err = db.PrepareNamed(updatePlaylistChannelVectorQuery)
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare updatePlaylistVectorStmt")
	} else {
		log.Info().Msg("Successfully prepared updatePlaylistVectorStmt")
	}

	// Prepare select playlist vector statement
	selectPlaylistVectorStmt, err = db.Preparex(selectPlaylistChannelVectorQuery)
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare selectPlaylistVectorStmt")
	} else {
		log.Info().Msg("Successfully prepared selectPlaylistVectorStmt")
	}
}

// Cache for frequently used vectors
var (
	frequentVectorsCache     []models.ChannelVector
	frequentVectorsCacheMux  sync.RWMutex
	frequentVectorsCacheTime time.Time
	frequentVectorsCacheTTL  = 5 * time.Minute // Cache TTL
)

// GetFrequentlyUsedVectors gets the most frequently referenced vectors in the database
func (q *VectorQueries) GetFrequentlyUsedVectors(ctx context.Context, limit int) ([]models.ChannelVector, error) {
	// For very large limits, check if we can use the cache
	if limit > 200 {
		frequentVectorsCacheMux.RLock()
		cacheValid := len(frequentVectorsCache) > 0 && time.Since(frequentVectorsCacheTime) < frequentVectorsCacheTTL
		if cacheValid && len(frequentVectorsCache) >= limit {
			// Use cached vectors
			result := make([]models.ChannelVector, limit)
			copy(result, frequentVectorsCache[:limit])
			frequentVectorsCacheMux.RUnlock()
			log.Debug().Int("limit", limit).Msg("Using cached frequently used vectors")
			return result, nil
		}
		frequentVectorsCacheMux.RUnlock()
	}

	vectors := []models.ChannelVector{}

	// Use a simpler query for large limits to improve performance
	var queryToUse string
	if limit > 500 {
		// For large limits, use a direct query that doesn't join tables
		queryToUse = `SELECT * FROM channelvectors ORDER BY id DESC LIMIT ?`
		log.Info().Int("limit", limit).Msg("Using simplified query for large vector cache")
	} else {
		// For smaller limits, use the original query that gets the most frequently used vectors
		queryToUse = selectFrequentlyUsedVectorsQuery
	}

	// Use direct query for better performance
	start := time.Now()
	err := q.SelectContext(ctx, &vectors, queryToUse, limit)
	duration := time.Since(start)

	if duration > 100*time.Millisecond {
		log.Debug().
			Int("limit", limit).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow GetFrequentlyUsedVectors query")
	}

	if err != nil {
		log.Error().Err(err).Int("limit", limit).Msg("Error getting frequently used vectors")
		return nil, err
	}

	// Update cache if this was a large query
	if limit > 200 {
		frequentVectorsCacheMux.Lock()
		frequentVectorsCache = vectors
		frequentVectorsCacheTime = time.Now()
		frequentVectorsCacheMux.Unlock()
		log.Info().Int("count", len(vectors)).Msg("Updated frequently used vectors cache")
	}

	// Also update the name cache with these vectors
	vectorNameCacheMux.Lock()
	for _, v := range vectors {
		vectorNameCache[v.Name] = &v
	}
	vectorNameCacheMux.Unlock()

	return vectors, nil
}

func (q *VectorQueries) GetTemplateChannelVector(channel_id int64) (*models.TemplateChannelVector, error) {
	vectorChannel := &models.TemplateChannelVector{}

	stmt, err := q.GetPreparedStmt(selectTemplateChannelVectorQuery)
	if err != nil {
		return nil, err
	}

	err = stmt.Get(vectorChannel, channel_id)
	if err != nil {
		// If prepared statement fails with parameter mismatch, fall back to direct query
		if strings.Contains(err.Error(), "expected 2 arguments") ||
			strings.Contains(err.Error(), "sql: expected") {
			log.Warn().Err(err).Int64("channel_id", channel_id).Msg("Prepared statement failed, falling back to direct query")
			// Fall back to direct query
			err = q.Get(vectorChannel, selectTemplateChannelVectorQuery, channel_id)
		}
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrNotFound
			}
			return nil, err
		}
	}

	return vectorChannel, nil
}

func (q *VectorQueries) GetTemplateChannelVectors() ([]models.TemplateChannelVector, error) {
	vectorChannels := []models.TemplateChannelVector{}

	stmt, err := q.GetPreparedStmt(selectAllTemplateChannelVectorsQuery)
	if err != nil {
		return nil, err
	}

	err = stmt.Select(&vectorChannels)
	if err != nil {
		// If prepared statement fails with parameter mismatch, fall back to direct query
		if strings.Contains(err.Error(), "expected 2 arguments") ||
			strings.Contains(err.Error(), "sql: expected") {
			log.Warn().Err(err).Msg("Prepared statement failed, falling back to direct query")
			// Fall back to direct query
			err = q.Select(&vectorChannels, selectAllTemplateChannelVectorsQuery)
		}
		if err != nil {
			return nil, err
		}
	}

	return vectorChannels, nil
}

func (q *VectorQueries) CreateTemplateChannelVector(channelVector *models.TemplateChannelVector) (int64, error) {
	stmt, err := q.GetPreparedStmt(insertTemplateChannelVectorQuery)
	if err != nil {
		return 0, err
	}

	res, err := stmt.Exec(channelVector.ChannelId, channelVector.VectorId)
	if err != nil {
		// If prepared statement fails with parameter mismatch, fall back to direct query
		if strings.Contains(err.Error(), "expected 2 arguments") ||
			strings.Contains(err.Error(), "sql: expected") {
			log.Warn().Err(err).Int64("channel_id", channelVector.ChannelId).Msg("Prepared statement failed, falling back to direct query")
			// Fall back to direct query
			res, err = q.Exec(insertTemplateChannelVectorQuery, channelVector.ChannelId, channelVector.VectorId)
		}
		if err != nil {
			return 0, err
		}
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Msgf("Error retrieving the ID: %v", err)
		return 0, err
	}

	return id, nil
}

func (q *VectorQueries) UpdateTemplateChannelVector(channelVector *models.TemplateChannelVector) error {
	stmt, err := q.GetPreparedStmt(updateTemplateChannelVectorQuery)
	if err != nil {
		return err
	}

	_, err = stmt.Exec(channelVector.VectorId, channelVector.ChannelId)
	if err != nil {
		// If prepared statement fails with parameter mismatch, fall back to direct query
		if strings.Contains(err.Error(), "expected 2 arguments") ||
			strings.Contains(err.Error(), "sql: expected") {
			log.Warn().Err(err).Int64("channel_id", channelVector.ChannelId).Msg("Prepared statement failed, falling back to direct query")
			// Fall back to direct query
			_, err = q.Exec(updateTemplateChannelVectorQuery, channelVector.VectorId, channelVector.ChannelId)
		}
	}
	return err
}

func (q *VectorQueries) GetPlaylistChannelVectors() ([]models.PlaylistChannelVector, error) {
	vectorChannels := []models.PlaylistChannelVector{}

	stmt, err := q.GetPreparedStmt(selectPlaylistChannelVectorsQuery)
	if err != nil {
		return nil, err
	}

	err = stmt.Select(&vectorChannels)
	if err != nil {
		// If prepared statement fails with parameter mismatch, fall back to direct query
		if strings.Contains(err.Error(), "expected 2 arguments") ||
			strings.Contains(err.Error(), "sql: expected") {
			log.Warn().Err(err).Msg("Prepared statement failed, falling back to direct query")
			// Fall back to direct query
			err = q.Select(&vectorChannels, selectPlaylistChannelVectorsQuery)
		}
		if err != nil {
			return nil, err
		}
	}

	return vectorChannels, nil
}

func (q *VectorQueries) GetPlChVectorsByPlaylist(playlistId int64) ([]models.PlaylistChannelVector, error) {
	vectorChannels := []models.PlaylistChannelVector{}

	stmt, err := q.GetPreparedStmt(selectPlaylistChannelVectorsByPlaylistQuery)
	if err != nil {
		return nil, err
	}

	err = stmt.Select(&vectorChannels, playlistId)
	if err != nil {
		// If prepared statement fails with parameter mismatch, fall back to direct query
		if strings.Contains(err.Error(), "expected 2 arguments") ||
			strings.Contains(err.Error(), "sql: expected") {
			log.Warn().Err(err).Int64("playlist_id", playlistId).Msg("Prepared statement failed, falling back to direct query")
			// Fall back to direct query
			err = q.Select(&vectorChannels, selectPlaylistChannelVectorsByPlaylistQuery, playlistId)
		}
		if err != nil {
			return nil, err
		}
	}

	return vectorChannels, nil
}

// Cache for playlist channel vectors
var (
	playlistVectorCache    = make(map[int64]*models.PlaylistChannelVector)
	playlistVectorCacheMux sync.RWMutex
)

func (q *VectorQueries) GetPlaylistChannelVector(channel_id int64) (*models.PlaylistChannelVector, error) {
	// Check cache first
	playlistVectorCacheMux.RLock()
	if cachedVector, found := playlistVectorCache[channel_id]; found {
		playlistVectorCacheMux.RUnlock()
		return cachedVector, nil
	}
	playlistVectorCacheMux.RUnlock()

	vectorChannel := &models.PlaylistChannelVector{}

	// Use prepared statement for better performance
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Increased from 1s to 15s to prevent timeouts
	defer cancel()

	start := time.Now()
	var err error

	// Check if prepared statement is available
	preparedStmtsMux.RLock()
	if selectPlaylistVectorStmt != nil {
		// Use prepared statement
		err = selectPlaylistVectorStmt.GetContext(ctx, vectorChannel, channel_id)

		// If it fails, fall back to direct query
		if err != nil && (strings.Contains(err.Error(), "expected 2 arguments") ||
			strings.Contains(err.Error(), "sql: expected")) {
			log.Warn().Err(err).Int64("channel_id", channel_id).Msg("Prepared statement failed, falling back to direct query")
			// Fall back to direct query with positional parameters
			err = q.GetContext(ctx, vectorChannel, selectPlaylistChannelVectorQuery, channel_id)
		}
	} else {
		// Fall back to direct query with positional parameters
		err = q.GetContext(ctx, vectorChannel, selectPlaylistChannelVectorQuery, channel_id)
	}
	preparedStmtsMux.RUnlock()

	duration := time.Since(start)

	if duration > 500*time.Millisecond { // Increased from 50ms to 500ms to match new timeout values
		log.Debug().
			Int64("channel_id", channel_id).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow GetPlaylistChannelVector query")
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Add to cache
	playlistVectorCacheMux.Lock()
	playlistVectorCache[channel_id] = vectorChannel
	playlistVectorCacheMux.Unlock()

	return vectorChannel, nil
}

func (q *VectorQueries) CreatePlaylistChannelVector(channelVector *models.PlaylistChannelVector) (int64, error) {
	// Use prepared statement for better performance
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Increased from 1s to 15s to prevent timeouts
	defer cancel()

	start := time.Now()
	var res sql.Result
	var err error

	// Check if prepared statement is available
	preparedStmtsMux.RLock()
	if insertPlaylistVectorStmt != nil {
		// Use prepared statement
		res, err = insertPlaylistVectorStmt.ExecContext(ctx, map[string]interface{}{
			"channel_id": channelVector.ChannelId,
			"vector_id":  channelVector.VectorId,
		})

		// If it fails, fall back to direct query
		if err != nil && (strings.Contains(err.Error(), "expected 2 arguments") ||
			strings.Contains(err.Error(), "sql: expected")) {
			log.Warn().Err(err).Int64("channel_id", channelVector.ChannelId).Msg("Prepared statement failed, falling back to direct query")
			// Fall back to direct query with positional parameters
			res, err = q.ExecContext(ctx, "INSERT INTO playlistchannelvectors VALUES (null, ?, ?)", channelVector.ChannelId, channelVector.VectorId)
		}
	} else {
		// Fall back to direct query with positional parameters
		res, err = q.ExecContext(ctx, "INSERT INTO playlistchannelvectors VALUES (null, ?, ?)", channelVector.ChannelId, channelVector.VectorId)
	}
	preparedStmtsMux.RUnlock()

	duration := time.Since(start)

	if duration > 500*time.Millisecond { // Increased from 50ms to 500ms to match new timeout values
		log.Debug().
			Int64("channel_id", channelVector.ChannelId).
			Int64("vector_id", channelVector.VectorId).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow CreatePlaylistChannelVector query")
	}

	if err != nil {
		// Check for UNIQUE constraint error
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			// Get the existing vector association
			existingVector, getErr := q.GetPlaylistChannelVector(channelVector.ChannelId)
			if getErr == nil {
				// If the vector ID is the same, just return the ID
				if existingVector.VectorId == channelVector.VectorId {
					return existingVector.ID, nil
				}
				// If the vector ID is different, update it
				existingVector.VectorId = channelVector.VectorId
				updateErr := q.UpdatePlaylistChannelVector(existingVector)
				if updateErr != nil {
					log.Error().Err(updateErr).Int64("channel_id", channelVector.ChannelId).Msg("Failed to update vector association")
					return 0, updateErr
				}
				return existingVector.ID, nil
			}
		}
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Err(err).Int64("channel_id", channelVector.ChannelId).Msg("Error retrieving the ID")
		return 0, err
	}

	// Update cache
	playlistVectorCacheMux.Lock()
	playlistVectorCache[channelVector.ChannelId] = &models.PlaylistChannelVector{
		ID:        id,
		ChannelId: channelVector.ChannelId,
		VectorId:  channelVector.VectorId,
	}
	playlistVectorCacheMux.Unlock()

	return id, nil
}

func (q *VectorQueries) UpdatePlaylistChannelVector(channelVector *models.PlaylistChannelVector) error {
	// Use prepared statement for better performance
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Increased from 1s to 15s to prevent timeouts
	defer cancel()

	start := time.Now()
	var err error

	// Check if prepared statement is available
	preparedStmtsMux.RLock()
	if updatePlaylistVectorStmt != nil {
		// Use prepared statement
		_, err = updatePlaylistVectorStmt.ExecContext(ctx, map[string]interface{}{
			"channel_id": channelVector.ChannelId,
			"vector_id":  channelVector.VectorId,
		})

		// If it fails, fall back to direct query
		if err != nil && (strings.Contains(err.Error(), "expected 2 arguments") ||
			strings.Contains(err.Error(), "sql: expected")) {
			log.Warn().Err(err).Int64("channel_id", channelVector.ChannelId).Msg("Prepared statement failed, falling back to direct query")
			// Fall back to direct query with positional parameters
			_, err = q.ExecContext(ctx, "UPDATE playlistchannelvectors SET vector_id = ? WHERE channel_id = ?", channelVector.VectorId, channelVector.ChannelId)
		}
	} else {
		// Fall back to direct query with positional parameters
		_, err = q.ExecContext(ctx, "UPDATE playlistchannelvectors SET vector_id = ? WHERE channel_id = ?", channelVector.VectorId, channelVector.ChannelId)
	}
	preparedStmtsMux.RUnlock()

	duration := time.Since(start)

	if duration > 500*time.Millisecond { // Increased from 50ms to 500ms to match new timeout values
		log.Debug().
			Int64("channel_id", channelVector.ChannelId).
			Int64("vector_id", channelVector.VectorId).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow UpdatePlaylistChannelVector query")
	}

	if err != nil {
		log.Error().Err(err).Int64("channel_id", channelVector.ChannelId).Msg("Failed to update playlist channel vector")
		return err
	}

	// Update cache
	playlistVectorCacheMux.Lock()
	if cached, exists := playlistVectorCache[channelVector.ChannelId]; exists {
		cached.VectorId = channelVector.VectorId
	} else {
		playlistVectorCache[channelVector.ChannelId] = channelVector
	}
	playlistVectorCacheMux.Unlock()

	return nil
}

// Cache for channel vectors by ID
var (
	vectorIdCache    = make(map[int64]*models.ChannelVector)
	vectorIdCacheMux sync.RWMutex
)

func (q *VectorQueries) GetChannelVector(id int64) (*models.ChannelVector, error) {
	// Check cache first
	vectorIdCacheMux.RLock()
	if cachedVector, found := vectorIdCache[id]; found {
		vectorIdCacheMux.RUnlock()
		return cachedVector, nil
	}
	vectorIdCacheMux.RUnlock()

	vectorChannel := &models.ChannelVector{}

	// Use prepared statement for better performance
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Increased from 1s to 15s to prevent timeouts
	defer cancel()

	start := time.Now()
	var err error

	// Check if prepared statement is available
	preparedStmtsMux.RLock()
	if selectVectorByIdStmt != nil {
		// Use prepared statement
		err = selectVectorByIdStmt.GetContext(ctx, vectorChannel, id)
	} else {
		// Fall back to direct query
		err = q.GetContext(ctx, vectorChannel, selectChannelVectorQuery, id)
	}
	preparedStmtsMux.RUnlock()

	duration := time.Since(start)

	if duration > 500*time.Millisecond { // Increased from 50ms to 500ms to match new timeout values
		log.Debug().
			Int64("id", id).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow GetChannelVector query")
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Add to cache
	vectorIdCacheMux.Lock()
	vectorIdCache[id] = vectorChannel
	vectorIdCacheMux.Unlock()

	// Also add to name cache
	vectorNameCacheMux.Lock()
	vectorNameCache[vectorChannel.Name] = vectorChannel
	vectorNameCacheMux.Unlock()

	return vectorChannel, nil
}

// In-memory cache for frequently accessed vectors by name
var (
	vectorNameCache     = make(map[string]*models.ChannelVector)
	vectorNameCacheMux  sync.RWMutex
	vectorNameCacheHits int64
	vectorNameCacheMiss int64
)

func (q *VectorQueries) GetChannelVectorByName(name string) (*models.ChannelVector, error) {
	// First check the in-memory cache (fast path)
	vectorNameCacheMux.RLock()
	if cachedVector, found := vectorNameCache[name]; found {
		vectorNameCacheMux.RUnlock()
		atomic.AddInt64(&vectorNameCacheHits, 1)

		// Log cache hit rate periodically
		total := atomic.LoadInt64(&vectorNameCacheHits) + atomic.LoadInt64(&vectorNameCacheMiss)
		if total%100 == 0 {
			hits := atomic.LoadInt64(&vectorNameCacheHits)
			rate := float64(hits) / float64(total) * 100
			log.Debug().Float64("hit_rate_percent", rate).Int64("hits", hits).Int64("total", total).Msg("Vector name cache stats")
		}

		return cachedVector, nil
	}
	vectorNameCacheMux.RUnlock()
	atomic.AddInt64(&vectorNameCacheMiss, 1)

	// Cache miss, query the database
	vectorChannel := &models.ChannelVector{}

	// Create a context with timeout to prevent long-running queries
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Increased from 1s to 15s to prevent timeouts
	defer cancel()

	// Use prepared statement for better performance
	start := time.Now()
	var err error

	// Check if prepared statement is available
	preparedStmtsMux.RLock()
	if selectVectorByNameStmt != nil {
		// Use prepared statement
		err = selectVectorByNameStmt.GetContext(ctx, vectorChannel, name)
	} else {
		// Fall back to direct query
		err = q.GetContext(ctx, vectorChannel, "SELECT * FROM channelvectors WHERE name = ? LIMIT 1", name)
	}
	preparedStmtsMux.RUnlock()

	duration := time.Since(start)

	if duration > 500*time.Millisecond { // Increased from 50ms to 500ms to match new timeout values
		log.Debug().
			Str("name", name).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow GetChannelVectorByName query")
	}

	if err != nil {
		if err == sql.ErrNoRows || strings.Contains(err.Error(), "no rows") {
			return nil, ErrNotFound
		}
		log.Error().Err(err).Str("name", name).Msg("Error getting channel vector by name")
		return nil, err
	}

	// Add to cache for future requests
	vectorNameCacheMux.Lock()
	vectorNameCache[name] = vectorChannel
	vectorNameCacheMux.Unlock()

	return vectorChannel, nil
}

// Cache for recently created vectors to avoid duplicate inserts
var (
	recentlyCreatedVectors    = make(map[string]int64) // name -> id
	recentlyCreatedVectorsMux sync.RWMutex
	maxRecentVectors          = 5000 // Maximum number of entries to keep (increased from 1000 to 5000)

	// Prepared statements for vector operations
	insertVectorStmt         *sqlx.NamedStmt
	selectVectorByNameStmt   *sqlx.Stmt
	selectVectorByIdStmt     *sqlx.Stmt
	insertPlaylistVectorStmt *sqlx.NamedStmt
	updatePlaylistVectorStmt *sqlx.NamedStmt
	selectPlaylistVectorStmt *sqlx.Stmt
	preparedStmtsMux         sync.RWMutex
)

func (q *VectorQueries) CreateChannelVector(channelVector *models.ChannelVector) (int64, error) {
	// Skip empty name
	if channelVector.Name == "" {
		return 0, fmt.Errorf("cannot create vector with empty name")
	}

	// First check if we've recently created this vector
	recentlyCreatedVectorsMux.RLock()
	if id, found := recentlyCreatedVectors[channelVector.Name]; found {
		recentlyCreatedVectorsMux.RUnlock()
		log.Debug().Str("name", channelVector.Name).Int64("id", id).Msg("Using recently created vector ID")
		return id, nil
	}
	recentlyCreatedVectorsMux.RUnlock()

	// Check if vector already exists in the database
	existingVector, getErr := q.GetChannelVectorByName(channelVector.Name)
	if getErr == nil {
		// Vector already exists, return its ID
		log.Debug().Str("name", channelVector.Name).Int64("id", existingVector.ID).Msg("Vector already exists, returning ID")

		// Add to recently created cache
		recentlyCreatedVectorsMux.Lock()
		recentlyCreatedVectors[channelVector.Name] = existingVector.ID
		// Prune cache if it gets too large
		if len(recentlyCreatedVectors) > maxRecentVectors {
			// Remove a random entry (first one we find)
			for k := range recentlyCreatedVectors {
				delete(recentlyCreatedVectors, k)
				break
			}
		}
		recentlyCreatedVectorsMux.Unlock()

		return existingVector.ID, nil
	}

	// Create a context with timeout to prevent long-running queries
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Increased from 1s to 15s to prevent timeouts
	defer cancel()

	// Use prepared statement for better performance
	start := time.Now()
	var res sql.Result
	var err error

	// Check if prepared statement is available
	preparedStmtsMux.RLock()
	if insertVectorStmt != nil {
		// Use prepared statement
		res, err = insertVectorStmt.ExecContext(ctx, map[string]interface{}{
			"name":   channelVector.Name,
			"vector": channelVector.Vector,
		})

		// If it fails, fall back to direct query
		if err != nil && (strings.Contains(err.Error(), "expected 2 arguments") ||
			strings.Contains(err.Error(), "sql: expected")) {
			log.Warn().Err(err).Str("name", channelVector.Name).Msg("Prepared statement failed, falling back to direct query")
			// Fall back to direct query with positional parameters
			res, err = q.ExecContext(ctx, "INSERT INTO channelvectors VALUES (null, ?, ?)", channelVector.Name, channelVector.Vector)
		}
	} else {
		// Fall back to direct query with positional parameters
		res, err = q.ExecContext(ctx, "INSERT INTO channelvectors VALUES (null, ?, ?)", channelVector.Name, channelVector.Vector)
	}
	preparedStmtsMux.RUnlock()

	duration := time.Since(start)

	if duration > 500*time.Millisecond { // Increased from 200ms to 500ms to match new timeout values
		log.Debug().
			Str("name", channelVector.Name).
			Float64("duration_ms", float64(duration.Milliseconds())).
			Msg("slow CreateChannelVector query")
	}

	if err != nil {
		// Check for UNIQUE constraint error and handle it specially
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			// Try to get the existing vector again
			existingVector, getErr := q.GetChannelVectorByName(channelVector.Name)
			if getErr == nil {
				// Return the existing vector ID
				log.Debug().Str("name", channelVector.Name).Int64("id", existingVector.ID).Msg("Returning existing vector ID after UNIQUE constraint")

				// Add to recently created cache
				recentlyCreatedVectorsMux.Lock()
				recentlyCreatedVectors[channelVector.Name] = existingVector.ID
				recentlyCreatedVectorsMux.Unlock()

				return existingVector.ID, nil
			}
		}

		log.Error().Err(err).Str("name", channelVector.Name).Msg("Error creating channel vector")
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Warn().Err(err).Str("name", channelVector.Name).Msg("Error retrieving the ID")
		return 0, err
	}

	// Add to recently created cache
	recentlyCreatedVectorsMux.Lock()
	recentlyCreatedVectors[channelVector.Name] = id
	recentlyCreatedVectorsMux.Unlock()

	// Also add to the name cache to avoid future database lookups
	vectorNameCacheMux.Lock()
	vectorNameCache[channelVector.Name] = &models.ChannelVector{ID: id, Name: channelVector.Name, Vector: channelVector.Vector}
	vectorNameCacheMux.Unlock()

	return id, nil
}
