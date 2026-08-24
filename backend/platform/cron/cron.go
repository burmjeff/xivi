package cron

import (
	"context"
	"sync"
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/go-co-op/gocron"
	"github.com/rs/zerolog/log"
)

// Mutex to prevent concurrent database operations
var (
	updateMutex sync.Mutex
	isUpdating  bool
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

	// Count channels that need to be removed
	removedCount := 0
	for _, channel := range *channels {
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
