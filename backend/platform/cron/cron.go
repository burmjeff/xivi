package cron

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/app/queries"
	"xivi/backend/pkg/streaming"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/go-co-op/gocron"
	"github.com/rs/zerolog/log"
)

// Mutex to prevent concurrent database operations
var (
	updateMutex      sync.Mutex
	isUpdating       bool
	maintenanceMutex sync.Mutex
)

// TODO DISPLAY RUN COUNT AND NEXT UPDATE TIME IN GUI
// TODO DYNAMICALLY UPDATE CRON ON SETTINGS CHANGE
func RunCronJobs() {
	localTime, err := time.LoadLocation(settings.APP_SETTINGS.Application.TZ)
	if err != nil {
		localTime = time.UTC
	}

	s := gocron.NewScheduler(localTime)

	// Schedule the update process that runs all updates
	updateJob, _ := s.Cron(settings.APP_SETTINGS.UpdateCron).Do(RunUpdates)
	log.Log().Msgf("Full update scheduled at: %s", updateJob.ScheduledAtTime())

	maintenanceInterval := settings.APP_SETTINGS.Maintenance.CleanupIntervalHours
	if _, err := s.Every(maintenanceInterval).Hours().Do(RunMaintenance); err != nil {
		log.Error().Err(err).Msg("Failed to schedule storage maintenance")
	} else {
		log.Info().Int("interval_hours", maintenanceInterval).Msg("Storage maintenance scheduled")
	}

	s.StartAsync()
	log.Info().Msg("Cron scheduler started successfully")
}

func UpdatePlaylists() {
	// Prevent concurrent updates
	updateMutex.Lock()
	if isUpdating {
		log.Warn().Msg("Skipping playlist update as another update is already in progress")
		updateMutex.Unlock()
		return
	}
	isUpdating = true
	updateMutex.Unlock()

	// Ensure isUpdating is reset when we're done
	defer func() {
		updateMutex.Lock()
		isUpdating = false
		updateMutex.Unlock()

		// Recover from any panics
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("Recovered from panic in UpdatePlaylists")
		}
	}()

	// Create a context with timeout for operations that might need it
	_, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// get playlists - use context where supported
	playlists, err := database.Db.GetPlaylists()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get playlists")
		return
	}

	if playlists == nil || len(*playlists) == 0 {
		log.Warn().Msg("No playlists found to update")
		return
	}

	log.Info().Int("count", len(*playlists)).Msg("Starting playlist update process")

	for _, playlist := range *playlists {
		log.Info().Str("playlist_name", playlist.Name).Int64("playlist_id", playlist.ID).Msg("Starting playlist update")

		// Capture start time for each playlist individually
		startTime := time.Now()

		m3uParser := utils.M3uParser{}
		if err := m3uParser.ParseM3u(playlist); err != nil {
			log.Error().Err(err).Int64("playlist_id", playlist.ID).Msg("Playlist refresh failed; retaining the previous snapshot")
			continue
		}
		CleanPlaylist(playlist, startTime)
		if _, err := database.Db.SyncSourceGroupsForPlaylist(context.Background(), playlist.ID); err != nil {
			log.Error().Err(err).Int64("playlist_id", playlist.ID).Msg("One or more connected groups could not be synced")
		}

		log.Info().Str("playlist_name", playlist.Name).Int64("playlist_id", playlist.ID).Msg("Finished playlist update")
	}

	// get templates.
	if templates, err := database.Db.GetTemplates(); err != nil {
		log.Debug().Err(err)
	} else {
		m3uTools := utils.NewM3uTools()
		for _, template := range *templates {
			go m3uTools.CreateM3u(template)
		}
	}
}

func UpdateEpgs() {
	// Prevent concurrent updates
	updateMutex.Lock()
	if isUpdating {
		log.Warn().Msg("Skipping EPG update as another update is already in progress")
		updateMutex.Unlock()
		return
	}
	isUpdating = true
	updateMutex.Unlock()

	// Ensure isUpdating is reset when we're done
	defer func() {
		updateMutex.Lock()
		isUpdating = false
		updateMutex.Unlock()

		// Recover from any panics
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("Recovered from panic in UpdateEpgs")
		}
	}()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// get epgs.
	epgs, err := database.Db.GetEpgs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get EPGs")
		return
	}

	if epgs == nil || len(*epgs) == 0 {
		log.Warn().Msg("No EPGs found to update")
		return
	}

	log.Info().Int("count", len(*epgs)).Msg("Starting EPG update process")

	for _, epg := range *epgs {
		log.Log().Msgf("Updating EPG: %s", epg.Name)
		if err := utils.ParseEpg(&epg); err != nil {
			log.Error().Err(err).Str("epg", epg.Name).Msg("EPG update failed")
			continue
		}
		log.Log().Msgf("Finished Updating EPG: %s", epg.Name)
	}

	log.Info().Msg("All EPGs updated, cleaning up old programmes")
	CleanupOldEpgProgrammes()
	log.Info().Msg("EPG update process completed successfully")

	// No signal needed for manual updates
}

func CleanPlaylist(playlist models.Playlist, startTime time.Time) {
	log.Info().Int64("playlist_id", playlist.ID).Msg("Starting playlist cleanup")

	// Create a context with timeout for database operations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := database.Db.CleanPlaylistChannels(ctx, playlist.ID); err != nil {
		log.Error().Err(err).Int64("playlist_id", playlist.ID).Msg("Failed to clean playlist channels")
	}

	// Get all channels for this playlist
	channels, err := database.Db.GetPlChannels(playlist.ID)
	if err != nil {
		log.Error().Err(err).Int64("playlist_id", playlist.ID).Msg("Failed to get playlist channels for cleanup")
		return
	}

	if channels == nil || len(*channels) == 0 {
		log.Info().Int64("playlist_id", playlist.ID).Msg("No channels found for cleanup")
		return
	}
	groups, err := database.Db.GetPlGroups(playlist.ID)
	if err != nil {
		log.Error().Err(err).Int64("playlist_id", playlist.ID).Msg("Failed to get playlist groups for cleanup")
		return
	}
	disabledGroups := map[int64]bool{}
	for _, group := range *groups {
		disabledGroups[group.ID] = !group.Enabled
	}

	// Count channels that need to be removed
	removedCount := 0
	for _, channel := range *channels {
		if disabledGroups[channel.GroupId] {
			continue
		}
		// A channel is considered stale if UpdatedAt is before the start time
		if channel.UpdatedAt.Before(startTime) {
			if err := database.Db.DeletePlChannel(channel.ID); err != nil {
				log.Error().Err(err).Int64("channel_id", channel.ID).Str("title", channel.Title).Msg("Failed to delete stale channel")
			} else {
				log.Debug().Int64("channel_id", channel.ID).Str("title", channel.Title).Msg("Removed stale channel")
				removedCount++
			}
		}
	}

	log.Info().Int64("playlist_id", playlist.ID).Int("removed_count", removedCount).Msg("Playlist cleanup completed")
}

func VacuumDB() {
	// Prevent running vacuum during updates
	updateMutex.Lock()
	if isUpdating {
		log.Warn().Msg("Skipping database vacuum as an update is in progress")
		updateMutex.Unlock()
		return
	}
	// We don't set isUpdating here because vacuum can run alongside other operations
	updateMutex.Unlock()

	// Call INCREMENTAL VACUUM with a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	log.Info().Msg("Starting database vacuum operation")
	err := database.Db.CleanupQueries.VacuumDB(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Database vacuum failed")
		return
	}
	log.Info().Msg("Database vacuum completed successfully")

	// No signal needed for manual updates
}

func CleanupOldEpgProgrammes() {
	// Clean up old EPG programmes
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Info().Msg("Starting cleanup of old EPG programmes")
	err := database.Db.CleanupQueries.CleanOldEpgProgrammes(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to clean up old EPG programmes")
		return
	}
	log.Info().Msg("EPG programme cleanup completed successfully")
}

type temporaryFileCleanup struct {
	FilesRemoved   int
	BytesReclaimed int64
}

// RunMaintenance applies both time and count ceilings. It is safe to call at
// startup and from the scheduler; overlapping passes are skipped.
func RunMaintenance() {
	if !maintenanceMutex.TryLock() {
		log.Debug().Msg("Skipping storage maintenance because a pass is already running")
		return
	}
	defer maintenanceMutex.Unlock()
	if database.Db == nil {
		return
	}

	configured := settings.APP_SETTINGS.Maintenance
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	report, databaseErr := database.Db.PruneRetentionData(ctx, queries.RetentionPolicy{
		OperationJobsBefore:          now.AddDate(0, 0, -configured.OperationJobRetentionDays),
		StreamHistoryBefore:          now.AddDate(0, 0, -configured.StreamDiagnosticsDays),
		EPGProgrammesBefore:          now.Add(-24 * time.Hour),
		MaximumOperationJobs:         configured.MaximumOperationJobs,
		MaximumStreamSessions:        configured.MaximumStreamSessions,
		MaximumEventsPerSession:      1000,
		MaximumConnectionsPerSession: 1000,
		BatchSize:                    500,
		PruneOrphanVectors:           true,
	})
	if databaseErr != nil {
		log.Error().Err(databaseErr).Msg("Storage maintenance completed with database errors")
	}

	streamFiles, streamErr := streaming.DefaultManager.PruneOrphanedStreamFiles()
	if streamErr != nil {
		log.Error().Err(streamErr).Msg("Failed to remove one or more orphaned stream directories")
	}
	temporaryFiles, temporaryErr := pruneTemporaryFiles(now.Add(-24 * time.Hour))
	if temporaryErr != nil {
		log.Error().Err(temporaryErr).Msg("Failed to remove one or more stale temporary files")
	}

	if databaseErr == nil {
		if err := database.Db.CleanupQueries.VacuumDB(ctx); err != nil {
			log.Error().Err(err).Msg("Bounded database compaction failed after maintenance")
		}
	}

	log.Info().
		Int64("jobs", report.OperationJobs).
		Int64("stream_sessions", report.StreamSessions).
		Int64("stream_events", report.StreamEvents).
		Int64("stream_connections", report.StreamConnections).
		Int64("epg_programmes", report.EPGProgrammes).
		Int64("orphan_vectors", report.OrphanVectors).
		Int("stream_directories", streamFiles.DirectoriesRemoved).
		Int("temporary_files", temporaryFiles.FilesRemoved).
		Int64("file_bytes_reclaimed", streamFiles.BytesReclaimed+temporaryFiles.BytesReclaimed).
		Msg("Storage maintenance completed")
}

func pruneTemporaryFiles(before time.Time) (temporaryFileCleanup, error) {
	report := temporaryFileCleanup{}
	var cleanupErrors []error
	for _, root := range []string{settings.M3U_FILEPATH, settings.EPG_FILEPATH, settings.LOGO_FILEPATH} {
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			cleanupErrors = append(cleanupErrors, err)
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".tmp" {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				cleanupErrors = append(cleanupErrors, err)
				continue
			}
			if !info.ModTime().Before(before) {
				continue
			}
			if err := os.Remove(filepath.Join(root, entry.Name())); err != nil && !os.IsNotExist(err) {
				cleanupErrors = append(cleanupErrors, err)
				continue
			}
			report.FilesRemoved++
			report.BytesReclaimed += info.Size()
		}
	}
	return report, errors.Join(cleanupErrors...)
}

// RunUpdates runs all updates in sequence: playlist -> EPG -> vacuum
// This is used by the cron scheduler to ensure updates happen in the correct order
func RunUpdates() {
	log.Log().Msg("Starting scheduled update process")
	database.ReinitializePreparedStatements()

	// Step 1: Update playlists
	log.Info().Msg("Step 1/3: Starting playlist update")
	UpdatePlaylists()

	// Step 2: Update EPGs
	log.Info().Msg("Step 2/3: Starting EPG update")
	UpdateEpgs()

	// Step 3: Vacuum database
	log.Info().Msg("Step 3/3: Starting database vacuum")
	VacuumDB()

	log.Info().Msg("Update process completed successfully")
}
