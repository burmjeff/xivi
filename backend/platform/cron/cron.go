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

	// Store dynamic group relationships before updating playlists
	var dynamicGroups []models.TemplateGroup
	if groups, err := database.Db.GetAllTmplGroups(); err != nil {
		log.Error().Err(err).Msg("Failed to get template groups")
	} else {
		for _, group := range *groups {
			if group.Dynamic && group.DynamicGroup != nil {
				dynamicGroups = append(dynamicGroups, group)
			}
		}
		log.Info().Int("count", len(dynamicGroups)).Msg("Found dynamic template groups")
	}

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
	startTime := time.Now()

	for _, playlist := range *playlists {
		log.Log().Msgf("Updating Playlist: %s", playlist.Name)
		m3uParser := utils.M3uParser{}
		m3uParser.ParseM3u(playlist)
		CleanPlaylist(playlist, startTime)
		log.Log().Msgf("Finished Updating Playlist: %s", playlist.Name)
	}

	// Update all dynamic groups
	if groups, err := database.Db.GetAllTmplGroups(); err != nil {
		log.Err(err)
	} else {
		// First, ensure all previously dynamic groups are still properly connected
		for _, storedGroup := range dynamicGroups {
			// Check if the group still exists and is still dynamic
			group, err := database.Db.GetTmplGroup(storedGroup.ID)
			if err != nil {
				log.Warn().Msgf("Template group %d no longer exists", storedGroup.ID)
				continue
			}

			// If the group is no longer dynamic, but was before, restore it
			if !group.Dynamic && storedGroup.DynamicGroup != nil {
				log.Info().Msgf("Restoring dynamic connection for template group %d", group.ID)
				group.Dynamic = true
				group.DynamicGroup = storedGroup.DynamicGroup
				if err := database.Db.UpdateTmplGroup(group); err != nil {
					log.Err(err).Msgf("Failed to restore dynamic connection for template group %d", group.ID)
				}
			}

			// Update the dynamic group
			if group.Dynamic && group.DynamicGroup != nil {
				utils.UpdateDynamicGroup(*group)
			}
		}

		// Then update any other dynamic groups
		for _, group := range *groups {
			if group.Dynamic && group.DynamicGroup != nil {
				// Skip groups we've already processed
				alreadyProcessed := false
				for _, storedGroup := range dynamicGroups {
					if group.ID == storedGroup.ID {
						alreadyProcessed = true
						break
					}
				}
				if !alreadyProcessed {
					utils.UpdateDynamicGroup(group)
				}
			}
		}
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

	// No signal needed for manual updates
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
		utils.ParseEpg(&epg)
		log.Log().Msgf("Finished Updating EPG: %s", epg.Name)
	}

	log.Info().Msg("All EPGs updated, cleaning up old programmes")
	CleanupOldEpgProgrammes()
	log.Info().Msg("EPG update process completed successfully")

	// No signal needed for manual updates
}

func CleanPlaylist(playlist models.Playlist, startTime time.Time) {
	// Create a context with timeout for database operations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := database.Db.CleanPlaylistGroups(ctx, playlist.ID); err != nil {
		log.Debug().Msgf("Playlist Clean: %v", err)
	}
	if err := database.Db.CleanPlaylistChannels(ctx, playlist.ID); err != nil {
		log.Debug().Msgf("Playlist Clean: %v", err)
	}
	if channels, err := database.Db.GetPlChannels(playlist.ID); err != nil {
		log.Err(err)
	} else {
		for _, channel := range *channels {
			if channel.UpdatedAt.Before(startTime) && channel.CreatedAt.Before(startTime) {
				database.Db.DeletePlChannel(channel.ID)
			}
		}
	}
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
