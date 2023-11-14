package cron

import (
	"time"
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
	playlistJob, err := s.Cron(settings.APP_SETTINGS.UpdateCron).Do(updatePlaylists)
	epgJob, err := s.Cron(settings.APP_SETTINGS.UpdateCron).Do(updateEpgs)

	log.Printf("Playlist update scheduled at: %s", playlistJob.ScheduledAtTime())
	log.Printf("Playlist update scheduled at: %s", epgJob.ScheduledAtTime())

	s.StartAsync()
}

func updatePlaylists() {
	// Create database connection.
	db, err := database.OpenDBConnection()
	if err != nil {
		log.Err(err)
		return
	}

	// get playlists.
	playlists, err := db.GetPlaylists()
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
		m3uParser.ParseM3u(&playlist)
	}

	m3uTools := utils.M3uTools{Db: db}
	for _, template := range templates {
		go m3uTools.CreateM3u(template)
	}

}

func updateEpgs() {
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
		epgParser.ParseEpg(&epg)
	}

	for _, template := range templates {
		go utils.CreateEpgXML(db, template)
	}

}
