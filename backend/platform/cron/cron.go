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
	playlistJob, _ := s.Cron(settings.APP_SETTINGS.UpdateCron).Do(UpdatePlaylists)
	epgJob, _ := s.Cron(settings.APP_SETTINGS.UpdateCron).Do(UpdateEpgs)
	vacuumJob, _ := s.Cron(settings.APP_SETTINGS.UpdateCron).Do(VacuumDB)

	log.Log().Msgf("Playlist update scheduled at: %s", playlistJob.ScheduledAtTime())
	log.Log().Msgf("EPG update scheduled at: %s", epgJob.ScheduledAtTime())
	log.Log().Msgf("Database vacuum scheduled at: %s", vacuumJob.ScheduledAtTime())

	s.StartAsync()
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
	}()

	// Store dynamic group relationships before updating playlists
	var dynamicGroups []models.TemplateGroup
	if groups, err := database.Db.GetAllTmplGroups(); err != nil {
		log.Err(err)
	} else {
		for _, group := range *groups {
			if group.Dynamic && group.DynamicGroup != nil {
				dynamicGroups = append(dynamicGroups, group)
			}
		}
	}

	// get playlists.
	playlists, err := database.Db.GetPlaylists()
	if err != nil {
		log.Err(err)
		return
	}
	startTime := time.Now()
	for _, playlist := range *playlists {
		log.Log().Msgf("Updating Playlist: %s", playlist.Name)
		m3uParser := utils.M3uParser{}
		m3uParser.ParseM3u(playlist)
		cleanPlaylist(playlist, startTime)
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
	}()

	// get epgs.
	epgs, err := database.Db.GetEpgs(context.Background())
	if err != nil {
		log.Err(err)
		return
	}

	for _, epg := range *epgs {
		log.Log().Msgf("Updating EPG: %s", epg.Name)
		utils.ParseEpg(&epg)
		log.Log().Msgf("Finished Updating EPG: %s", epg.Name)
	}
	CleanupOldEpgProgrammes()
}

func cleanPlaylist(playlist models.Playlist, startTime time.Time) {
	ctx := context.Background()
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

	// Call INCREMENTAL VACUUM
	ctx := context.Background()
	err := database.Db.CleanupQueries.VacuumDB(ctx)
	if err != nil {
		log.Debug().Msgf("Database vacuum: %v", err)
	}
	log.Debug().Msgf("Database vacuum completed")
}

func CleanupOldEpgProgrammes() {
	// Clean up old EPG programmes
	ctx := context.Background()
	err := database.Db.CleanupQueries.CleanOldEpgProgrammes(ctx)
	if err != nil {
		log.Error().Msgf("Failed to clean up old EPG programmes: %v", err)
	}
	log.Debug().Msg("EPG programme cleanup completed")
}
