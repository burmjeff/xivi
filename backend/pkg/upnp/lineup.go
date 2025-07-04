package upnp

import (
	"fmt"
	"strconv"
	"xivi/backend/platform/settings"
)

// DiscoverData represents the SSDP discovery response
type DiscoverData struct {
	FriendlyName    string `json:"FriendlyName"`
	Manufacturer    string `json:"Manufacturer"`
	ModelNumber     string `json:"ModelNumber"`
	FirmwareName    string `json:"FirmwareName"`
	TunerCount      int    `json:"TunerCount"`
	FirmwareVersion string `json:"FirmwareVersion"`
	DeviceID        string `json:"DeviceID"`
	DeviceAuth      string `json:"DeviceAuth"`
	BaseURL         string `json:"BaseURL"`
	LineupURL       string `json:"LineupURL"`
}

// LineupStatus represents the SSDP lineup status response
type LineupStatus struct {
	ScanInProgress int      `json:"ScanInProgress"`
	ScanPossible   int      `json:"ScanPossible"`
	Source         string   `json:"Source"`
	SourceList     []string `json:"SourceList"`
}

// LineupEntry represents a single channel in the lineup
type LineupEntry struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	URL         string `json:"URL"`
}

// Channel represents a template channel for lineup generation
type Channel struct {
	ID     int64
	Name   string
	UUID   string
	Number int
}

// GenerateDiscoverData creates discovery data
func GenerateDiscoverData(templateID int64, templateName, deviceUUID string) DiscoverData {
	baseURL := fmt.Sprintf("http://%s:%d", settings.APP_SETTINGS.Server.Host, settings.APP_SETTINGS.Server.Port)

	return DiscoverData{
		FriendlyName:    fmt.Sprintf("%s - %s", settings.APP_SETTINGS.Application.AppName, templateName),
		Manufacturer:    settings.APP_SETTINGS.UPnP.Manufacturer,
		ModelNumber:     settings.APP_SETTINGS.UPnP.ModelNumber,
		FirmwareName:    settings.APP_SETTINGS.UPnP.FirmwareName,
		TunerCount:      settings.APP_SETTINGS.UPnP.TunerCount,
		FirmwareVersion: settings.APP_SETTINGS.UPnP.FirmwareVersion,
		DeviceID:        fmt.Sprintf("%s-%d", settings.APP_SETTINGS.UPnP.Manufacturer, templateID),
		DeviceAuth:      settings.APP_SETTINGS.UPnP.DeviceAuth,
		BaseURL:         baseURL,
		LineupURL:       fmt.Sprintf("%s/lineup.json", baseURL),
	}
}

// GenerateLineupStatus creates lineup status for a template
func GenerateLineupStatus() LineupStatus {
	return LineupStatus{
		ScanInProgress: 0,
		ScanPossible:   1,
		Source:         "Cable",
		SourceList:     []string{"Cable"},
	}
}

// GenerateLineup creates a channel lineup for a template
func GenerateLineup(templateID int64, channels []Channel) []LineupEntry {
	lineup := make([]LineupEntry, 0, len(channels))

	for i, channel := range channels {
		// Generate channel number if not provided
		channelNumber := channel.Number
		if channelNumber == 0 {
			channelNumber = i + 1
		}

		// Create stream URL
		streamURL := fmt.Sprintf("http://%s:%d/stream/%s",
			settings.APP_SETTINGS.Server.Host,
			settings.APP_SETTINGS.Server.Port,
			channel.UUID)

		entry := LineupEntry{
			GuideNumber: strconv.Itoa(channelNumber),
			GuideName:   channel.Name,
			URL:         streamURL,
		}

		lineup = append(lineup, entry)
	}

	return lineup
}
