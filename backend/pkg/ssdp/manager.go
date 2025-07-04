package ssdp

import (
	"crypto/md5"
	"fmt"
	"sync"
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
