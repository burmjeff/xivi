package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"xivi/backend/pkg/utils"
	"xivi/backend/pkg/virtualtuner"
	"xivi/backend/platform/cron"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// StartServer func for starting a simple server.
func StartServer(app *fiber.App) {
	var err error
	//Initialize DB
	database.Db, err = database.OpenDBConnection()
	if err != nil {
		log.Fatal().Msgf("Failed to connect to database: %v", err)
	}
	if interrupted, reconcileErr := database.Db.FailIncompleteJobs(context.Background()); reconcileErr != nil {
		log.Error().Err(reconcileErr).Msg("Failed to reconcile interrupted operation jobs")
	} else if interrupted > 0 {
		log.Warn().Int64("jobs", interrupted).Msg("Marked interrupted operation jobs as failed")
	}

	// Now that database is initialized, we can safely initialize the vector cache
	utils.InitializeCache()

	vips.LoggingSettings(nil, vips.LogLevelCritical)
	vips.Startup(nil)
	defer vips.Shutdown()

	//config values
	host := flag.String("host", settings.APP_SETTINGS.Host, "Server Host")
	port := flag.Int("port", settings.APP_SETTINGS.Port, "Server Port")
	settings.SERVER_PATH = fmt.Sprintf("%s:%d", *host, *port)

	//Start Cronjobs
	cron.RunCronJobs()
	go cron.RunUpdates()

	if err := virtualtuner.Reconfigure(); err != nil {
		log.Error().Err(err).Msg("Failed to configure virtual tuner discovery")
	}
	defer func() {
		if err := virtualtuner.Stop(); err != nil {
			log.Error().Err(err).Msg("Failed to stop virtual tuner discovery")
		}
	}()

	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Info().Msg("Gracefully shutting down...")

		// Shutdown Fiber app
		if err := app.Shutdown(); err != nil {
			log.Error().Msgf("Error shutting down server: %v", err)
		}
	}()

	// Run server.
	log.Printf("Server starting at http://%s ...\n", settings.SERVER_PATH)
	if err := app.Listen(fmt.Sprintf("0.0.0.0:%d", *port)); err != nil {
		log.Printf("Oops... Server is not running! Reason: %v", err)
	}
}
