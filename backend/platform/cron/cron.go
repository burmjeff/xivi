package cron

import (
	"time"
	"xivi/backend/app/models"
	"xivi/backend/pkg/utils"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/go-co-op/gocron"
	"github.com/rs/zerolog/log"
)

// TODO DISPLAY RUN COUNT AND NEXT UPDATE TIME IN GUI
// TODO DYNAMICALLY UPDATE CRON ON SETTINGS CHANGE
func RunCronJobs() {
	localTime, err := time.LoadLocation(settings.APP_SETTINGS.Application.TZ)
	if err != nil {
		localTime = time.UTC
	}

	s := gocron.NewScheduler(localTime)
	playlistJob, err := s.Cron(settings.APP_SETTINGS.UpdateCron).Do(UpdatePlaylists)
	epgJob, err := s.Cron(settings.APP_SETTINGS.UpdateCron).Do(UpdateEpgs)

	log.Printf("Playlist update scheduled at: %s", playlistJob.ScheduledAtTime())
	log.Printf("Playlist update scheduled at: %s", epgJob.ScheduledAtTime())

	s.StartAsync()
}

func UpdatePlaylists() {

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
		log.Log().Msgf("Finished Updating Playlist: %s", playlist.Name)
	}

	cleanPlaylists(*playlists, startTime)

	if groups, err := database.Db.GetAllTmplGroups(); err != nil {
		log.Err(err)
	} else {
		for _, group := range *groups {
			if group.Dynamic {
				utils.UpdateDynamicGroup(group)
			}
		}
	}

	// get templates.
	if templates, err := database.Db.GetTemplates(); err != nil {
		log.Debug().Err(err)
	} else {
		m3uTools := utils.M3uTools{}
		for _, template := range *templates {
			go m3uTools.CreateM3u(template)
		}
	}
}

func UpdateEpgs() {
	// get epgs.
	epgs, err := database.Db.GetEpgs()
	if err != nil {
		log.Err(err)
		return
	}

	for _, epg := range *epgs {
		log.Log().Msgf("Updating EPG: %s", epg.Name)
		utils.ParseEpg(&epg)
		log.Log().Msgf("Finished Updating EPG: %s", epg.Name)
	}
}

func cleanPlaylists(playlists []models.Playlist, startTime time.Time) {
	if len(playlists) == 0 {
		if err := database.Db.DeletePlGroups(); err != nil {
			log.Err(err)
		}
		if err := database.Db.DeletePlChannels(); err != nil {
			log.Err(err)
		}
	} else {
		for _, playlist := range playlists {
			if err := database.Db.CleanPlaylistGroups(playlist.ID); err != nil {
				log.Debug().Msgf("Playlist Clean: %v", err)
			}
			if err := database.Db.CleanPlaylistChannels(playlist.ID); err != nil {
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
	}

}
