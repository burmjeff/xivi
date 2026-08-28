package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"xivi/backend/app/models"
	"xivi/backend/pkg/security"
	"xivi/backend/pkg/streaming"
	"xivi/backend/pkg/utils"
	"xivi/backend/pkg/virtualtuner"
	"xivi/backend/platform/cron"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

const (
	initialAdminUsername = "xivi"
	initialAdminPassword = "xivi"
)

func initializeDefaultAdministrator(ctx context.Context) (bool, error) {
	count, err := database.Db.UserCount(ctx)
	if err != nil {
		return false, err
	}
	created := false
	if count == 0 {
		passwordHash, hashErr := security.HashPassword(initialAdminPassword)
		if hashErr != nil {
			return false, hashErr
		}
		var id int64
		created, id, err = database.Db.CreateInitialAdminIfEmpty(ctx, initialAdminUsername, passwordHash)
		if err != nil {
			return false, err
		}
		if created {
			_ = database.Db.Audit(ctx, models.SecurityAuditEvent{
				TargetUserID:   &id,
				TargetUsername: initialAdminUsername,
				Action:         "initial_admin_create",
				Outcome:        "success",
				ResourceType:   "user",
				ResourceID:     fmt.Sprintf("%d", id),
				Detail:         "automatic_first_run_account_password_change_required",
			})
		}
	}
	initialSetupRequired, err := database.Db.InitialAdminPasswordChangeRequired(ctx, initialAdminUsername)
	if err != nil {
		return false, err
	}
	security.SetInitialSetupRequired(initialSetupRequired)
	return created, nil
}

// StartServer func for starting a simple server.
func StartServer(app *fiber.App) {
	var err error
	//Initialize DB
	database.Db, err = database.OpenDBConnection()
	if err != nil {
		log.Fatal().Msgf("Failed to connect to database: %v", err)
	}
	createdInitialAdmin, err := initializeDefaultAdministrator(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize the first administrator")
	}
	if createdInitialAdmin {
		log.Warn().Str("username", initialAdminUsername).Msg("Created the first-run administrator; a password change is required before Xivi can be used")
	}
	if interrupted, reconcileErr := database.Db.FailIncompleteJobs(context.Background()); reconcileErr != nil {
		log.Error().Err(reconcileErr).Msg("Failed to reconcile interrupted operation jobs")
	} else if interrupted > 0 {
		log.Warn().Int64("jobs", interrupted).Msg("Marked interrupted operation jobs as failed")
	}
	if interrupted, reconcileErr := database.Db.ReconcileStreamSessions(context.Background()); reconcileErr != nil {
		log.Error().Err(reconcileErr).Msg("Failed to reconcile interrupted stream sessions")
	} else if interrupted > 0 {
		log.Warn().Int64("sessions", interrupted).Msg("Marked interrupted stream sessions as stopped")
	}
	cron.RunMaintenance()
	streamAudit := newStreamObserver()
	streaming.DefaultManager.SetObserver(streamAudit)
	defer func() {
		streaming.DefaultManager.SetObserver(nil)
		streamAudit.Close()
	}()

	// Now that database is initialized, we can safely initialize the vector cache
	utils.InitializeCache()

	vips.LoggingSettings(nil, vips.LogLevelCritical)
	vips.Startup(nil)
	defer vips.Shutdown()

	// The bind address is deployment-managed. The container defaults to
	// 0.0.0.0, while a direct process defaults to loopback.
	settings.SERVER_PATH = fmt.Sprintf("%s:%d", settings.Current().Host, settings.Current().Port)

	//Start Cronjobs
	if err := cron.RunCronJobs(); err != nil {
		log.Fatal().Err(err).Msg("Failed to configure background schedules")
	}
	defer cron.StopCronJobs()

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
		streaming.DefaultManager.Close()

		// Shutdown Fiber app
		if err := app.Shutdown(); err != nil {
			log.Error().Msgf("Error shutting down server: %v", err)
		}
	}()

	// Run server.
	log.Printf("Server starting at http://%s ...\n", settings.SERVER_PATH)
	if err := app.Listen(settings.SERVER_PATH); err != nil {
		log.Printf("Oops... Server is not running! Reason: %v", err)
	}
}
