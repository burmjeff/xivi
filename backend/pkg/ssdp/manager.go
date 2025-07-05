package ssdp

import (
	"crypto/md5"
	"fmt"
	"sync"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

var (
	// Global SSDP service instance
	globalService *SSDPService
	serviceOnce   sync.Once
)

// GetService returns the global SSDP service instance
func GetService() *SSDPService {
	serviceOnce.Do(func() {
		globalService = NewSSDPService()
	})
	return globalService
}

// InitializeService starts the global SSDP service and registers device
func InitializeService() error {
	service := GetService()
	if err := service.Start(); err != nil {
		return err
	}

	// Register the single SSDP device for Plex discovery
	return RegisterSSDPDevice()
}

// ShutdownService stops the global SSDP service
func ShutdownService() error {
	if globalService != nil {
		UnregisterSSDPDevice() // Clean shutdown
		return globalService.Stop()
	}
	return nil
}

// CreateSSDPDevice creates a single SSDP device for Plex discovery
func CreateSSDPDevice() *Device {
	// Generate a consistent UUID for the SSDP device
	uuid := generateSSDPUUID()

	// Create SSDP device for Plex compatibility
	device := &Device{
		UUID:         uuid,
		DeviceType:   DeviceTypeMediaServer,
		FriendlyName: fmt.Sprintf("%s SSDP", settings.APP_SETTINGS.Application.AppName),
		Manufacturer: settings.APP_SETTINGS.UPnP.Manufacturer,
		ModelName:    settings.APP_SETTINGS.UPnP.ModelName,
		ModelNumber:  settings.APP_SETTINGS.UPnP.ModelNumber,
		BaseURL:      fmt.Sprintf("http://%s:%d", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port),
		TemplateID:   0, // Not template-specific anymore
	}

	return device
}

// generateSSDPUUID creates a consistent UUID for the SSDP device
func generateSSDPUUID() string {
	// Create a deterministic UUID based on app settings
	data := fmt.Sprintf("%s-%s-ssdp",
		settings.APP_SETTINGS.Application.AppName)

	hash := md5.Sum([]byte(data))

	// Format as UUID (8-4-4-4-12)
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		hash[0:4],
		hash[4:6],
		hash[6:8],
		hash[8:10],
		hash[10:16])
}

// RegisterSSDPDevice registers the SSDP device for Plex discovery
func RegisterSSDPDevice() error {
	if !settings.APP_SETTINGS.UPnP.Enabled {
		return nil
	}

	service := GetService()
	device := CreateSSDPDevice()

	err := service.AddDevice(device)
	if err != nil {
		return fmt.Errorf("failed to register SSDP device: %v", err)
	}

	log.Info().Msgf("Registered SSDP device for Plex discovery: %s", device.UUID)
	return nil
}

// UnregisterSSDPDevice removes the SSDP device
func UnregisterSSDPDevice() error {
	if !settings.APP_SETTINGS.UPnP.Enabled {
		return nil
	}

	service := GetService()
	uuid := generateSSDPUUID()

	err := service.RemoveDevice(uuid)
	if err != nil {
		return fmt.Errorf("failed to unregister SSDP device: %v", err)
	}

	log.Info().Msgf("Unregistered SSDP device: %s", uuid)
	return nil
}

// RegisterTemplateDevices registers separate SSDP devices for each template
func RegisterTemplateDevices() error {
	if !settings.APP_SETTINGS.UPnP.Enabled {
		return nil
	}

	service := GetService()

	// Get all templates
	templates, err := database.Db.GetTemplates()
	if err != nil {
		return fmt.Errorf("failed to get templates: %v", err)
	}

	if len(*templates) == 0 {
		log.Info().Msg("No templates found, skipping template device registration")
		return nil
	}

	// Register a device for each template
	for _, template := range *templates {
		device := CreateTemplateDevice(template)
		if err := service.AddDevice(device); err != nil {
			log.Error().Msgf("Failed to register device for template %s: %v", template.Name, err)
			continue
		}
		log.Info().Msgf("Registered SSDP device for template: %s (%s)", template.Name, device.UUID)
	}

	return nil
}

// CreateTemplateDevice creates an SSDP device for a specific template
func CreateTemplateDevice(template models.Template) *Device {
	uuid := GenerateTemplateUUID(template.ID)

	device := &Device{
		UUID:         uuid,
		DeviceType:   DeviceTypeMediaServer,
		FriendlyName: fmt.Sprintf("%s - %s", settings.APP_SETTINGS.Application.AppName, template.Name),
		Manufacturer: settings.APP_SETTINGS.UPnP.Manufacturer,
		ModelName:    settings.APP_SETTINGS.UPnP.ModelName,
		ModelNumber:  settings.APP_SETTINGS.UPnP.ModelNumber,
		BaseURL:      fmt.Sprintf("http://%s:%d", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port),
		TemplateID:   template.ID,
	}

	return device
}

// GenerateTemplateUUID creates a consistent UUID for a template device
func GenerateTemplateUUID(templateID int64) string {
	// Create a deterministic UUID based on app settings and template ID
	data := fmt.Sprintf("%s-template-%d",
		settings.APP_SETTINGS.Application.AppName, templateID)

	hash := md5.Sum([]byte(data))

	// Format as UUID (8-4-4-4-12)
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		hash[0:4],
		hash[4:6],
		hash[6:8],
		hash[8:10],
		hash[10:16])
}

// RegisterTemplateDevice registers a single template device
func RegisterTemplateDevice(template models.Template) error {
	if !settings.APP_SETTINGS.UPnP.Enabled {
		return nil
	}

	service := GetService()
	device := CreateTemplateDevice(template)

	err := service.AddDevice(device)
	if err != nil {
		return fmt.Errorf("failed to register template device: %v", err)
	}

	log.Info().Msgf("Registered SSDP device for template: %s (%s)", template.Name, device.UUID)
	return nil
}

// UnregisterTemplateDevice removes a template device
func UnregisterTemplateDevice(templateID int64) error {
	if !settings.APP_SETTINGS.UPnP.Enabled {
		return nil
	}

	service := GetService()
	uuid := GenerateTemplateUUID(templateID)

	err := service.RemoveDevice(uuid)
	if err != nil {
		return fmt.Errorf("failed to unregister template device: %v", err)
	}

	log.Info().Msgf("Unregistered template device: %s", uuid)
	return nil
}

// RefreshTemplateDevices re-registers all template devices
func RefreshTemplateDevices() error {
	if !settings.APP_SETTINGS.UPnP.Enabled {
		return nil
	}

	// Clear existing template devices
	service := GetService()
	devices := service.GetDevices()

	for uuid, device := range devices {
		if device.TemplateID > 0 { // Template devices have TemplateID > 0
			if err := service.RemoveDevice(uuid); err != nil {
				log.Error().Msgf("Failed to remove template device %s: %v", uuid, err)
			}
		}
	}

	// Re-register all template devices
	return RegisterTemplateDevices()
}
