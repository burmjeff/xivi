package cron

import (
	"path/filepath"
	"testing"
	"xivi/backend/platform/settings"
)

func schedulerSettings(t *testing.T) *settings.AppSettings {
	t.Helper()
	configured, err := settings.SetDefaults()
	if err != nil {
		t.Fatal(err)
	}
	return configured
}

func TestValidateConfigurationAcceptsAppOwnedSchedules(t *testing.T) {
	configured := schedulerSettings(t)
	configured.Application.TZ = "America/New_York"
	configured.Application.UpdateCron = "0 */6 * * *"
	configured.Maintenance.CleanupIntervalHours = 12
	if err := ValidateConfiguration(configured); err != nil {
		t.Fatalf("valid schedule rejected: %v", err)
	}

	configured.Application.UpdateCron = "0 0 */6 * * *"
	if err := ValidateConfiguration(configured); err != nil {
		t.Fatalf("valid schedule with seconds rejected: %v", err)
	}
}

func TestValidateConfigurationRejectsInvalidCapturedValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*settings.AppSettings)
	}{
		{"timezone", func(config *settings.AppSettings) { config.Application.TZ = "Not/A_Timezone" }},
		{"cron", func(config *settings.AppSettings) { config.Application.UpdateCron = "not a schedule" }},
		{"maintenance interval", func(config *settings.AppSettings) { config.Maintenance.CleanupIntervalHours = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configured := schedulerSettings(t)
			test.mutate(configured)
			if err := ValidateConfiguration(configured); err == nil {
				t.Fatal("invalid scheduler settings were accepted")
			}
		})
	}
}

func TestReconfigureAppliesScheduleWithoutProcessRestart(t *testing.T) {
	originalPath := settings.CONFIG_PATH
	originalSettings := settings.Current()
	settings.CONFIG_PATH = filepath.Join(t.TempDir(), "config")
	if err := settings.InitPaths(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		StopCronJobs()
		_ = settings.WriteSettings(originalSettings)
		settings.CONFIG_PATH = originalPath
	})

	configured := schedulerSettings(t)
	configured.Application.TZ = "UTC"
	configured.Application.UpdateCron = "0 4 * * *"
	configured.Maintenance.CleanupIntervalHours = 24
	if err := settings.WriteSettings(configured); err != nil {
		t.Fatal(err)
	}
	if err := Reconfigure(); err != nil {
		t.Fatal(err)
	}
	if activeScheduler == nil || activeScheduler.Location().String() != "UTC" || len(activeScheduler.Jobs()) != 2 {
		t.Fatal("initial runtime schedule was not applied")
	}

	updated := *configured
	updated.Application.TZ = "America/Los_Angeles"
	updated.Application.UpdateCron = "30 5 * * *"
	updated.Maintenance.CleanupIntervalHours = 12
	if err := settings.WriteSettings(&updated); err != nil {
		t.Fatal(err)
	}
	if err := Reconfigure(); err != nil {
		t.Fatal(err)
	}
	if activeScheduler.Location().String() != "America/Los_Angeles" || len(activeScheduler.Jobs()) != 2 {
		t.Fatal("updated runtime schedule was not applied")
	}
}
