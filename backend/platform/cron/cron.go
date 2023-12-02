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
	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		log.Err(err)
		return
	}

	// get playlists.
	playlists, err := db.GetPlaylists()
	cleanPlaylists(db, playlists)
	if err != nil {
		log.Err(err)
		return
	}

	// get templates.
	templates, err := db.GetTemplates()
	if err != nil {
		log.Err(err)
		return
	}

	m3uParser := utils.M3uParser{Db: db}
	for _, playlist := range playlists {
		log.Log().Msgf("Updating Playlist: %s", playlist.Name)
		m3uParser.ParseM3u(&playlist)

		if channels, err := db.GetPlChannels(playlist.ID); err != nil {
			log.Err(err)
		} else {
			for _, channel := range channels {
				if channel.UpdatedAt.Before(time.Now().Add(-24*time.Hour)) && channel.CreatedAt.Before(time.Now().Add(-24*time.Hour)) {
					db.DeletePlChannel(channel.ID)
				}
			}
		}
		log.Log().Msgf("Finished Updating Playlist: %s", playlist.Name)
	}

	m3uTools := utils.M3uTools{Db: db}
	for _, template := range templates {
		go m3uTools.CreateM3u(template)
	}

}

func UpdateEpgs() {
	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		log.Err(err)
		return
	}

	// get epgs.
	epgs, err := db.GetEpgs()
	if err != nil {
		log.Err(err)
		return
	}

	// get templates.
	templates, err := db.GetTemplates()
	if err != nil {
		log.Err(err)
		return
	}

	epgParser := utils.EpgParser{Db: db}
	for _, epg := range epgs {
		log.Log().Msgf("Updating EPG: %s", epg.Name)
		epgParser.ParseEpg(&epg)
	}

	for _, template := range templates {
		go utils.CreateEpgXML(db, template)
	}

}

func cleanPlaylists(db *database.Queries, playlists []models.Playlist) {
	if len(playlists) == 0 {
		if err := db.DeletePlGroups(); err != nil {
			log.Err(err)
		}
		if err := db.DeletePlChannels(); err != nil {
			log.Err(err)
		}
	} else {
		var playlistIds []int64
		for _, playlist := range playlists {
			playlistIds = append(playlistIds, playlist.ID)
		}
		if err := db.CleanPlaylistGroups(playlistIds); err != nil {
			log.Debug().Msgf("Playlist Clean: %v", err)
		}
		if err := db.CleanPlaylistChannels(playlistIds); err != nil {
			log.Debug().Msgf("Playlist Clean: %v", err)
		}
	}

}
