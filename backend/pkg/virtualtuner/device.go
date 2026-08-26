package virtualtuner

import (
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"xivi/backend/app/models"
	"xivi/backend/platform/database"
	"xivi/backend/platform/settings"
)

const (
	Manufacturer    = "Xivi"
	ModelName       = "Xivi Virtual Tuner"
	ModelNumber     = "XIVI-VT1"
	FirmwareName    = "xivi_virtual_tuner"
	FirmwareVersion = "20260824"
)

type Device struct {
	LineupID              int64  `json:"lineup_id"`
	LineupName            string `json:"lineup_name"`
	Enabled               bool   `json:"enabled"`
	FillMissingGuideSlots bool   `json:"fill_missing_guide_slots"`
	DeviceID              uint32 `json:"-"`
	DeviceIDHex           string `json:"device_id"`
	DeviceAuth            string `json:"-"`
	TunerCount            uint8  `json:"tuner_count"`
	BaseURL               string `json:"base_url"`
	LineupURL             string `json:"lineup_url"`
}

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

type LineupStatus struct {
	ScanInProgress int      `json:"ScanInProgress"`
	ScanPossible   int      `json:"ScanPossible"`
	Source         string   `json:"Source"`
	SourceList     []string `json:"SourceList"`
}

type LineupEntry struct {
	GuideNumber string `json:"GuideNumber"`
	GuideName   string `json:"GuideName"`
	URL         string `json:"URL"`
}

func Devices(origin string) ([]Device, error) {
	all, err := AllDevices(origin)
	if err != nil {
		return nil, err
	}
	devices := make([]Device, 0, len(all))
	for _, device := range all {
		if device.Enabled {
			devices = append(devices, device)
		}
	}
	return devices, nil
}

// AllDevices returns every lineup for Studio, including outputs that are not
// currently advertised. Discovery uses Devices so disabled lineups never leak.
func AllDevices(origin string) ([]Device, error) {
	templates, err := database.Db.GetTemplates()
	if err != nil {
		return nil, err
	}
	devices := make([]Device, 0, len(*templates))
	for _, template := range *templates {
		devices = append(devices, NewDevice(template, origin, settings.APP_SETTINGS.VirtualTuner.TunerCount))
	}
	return devices, nil
}

func NewDevice(lineup models.Template, origin string, tunerCount int) Device {
	deviceID := DeviceID(lineup.ID)
	baseURL := fmt.Sprintf("%s/virtual-tuner/%d", strings.TrimRight(origin, "/"), lineup.ID)
	if tunerCount < 1 {
		tunerCount = 1
	}
	if tunerCount > 255 {
		tunerCount = 255
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("xivi-virtual-tuner-%d", lineup.ID)))
	return Device{
		LineupID:              lineup.ID,
		LineupName:            lineup.Name,
		Enabled:               lineup.VirtualTunerEnabled,
		FillMissingGuideSlots: lineup.FillMissingGuideSlots,
		DeviceID:              deviceID,
		DeviceIDHex:           fmt.Sprintf("%08X", deviceID),
		DeviceAuth:            fmt.Sprintf("%x", hash[:12]),
		TunerCount:            uint8(tunerCount),
		BaseURL:               baseURL,
		LineupURL:             baseURL + "/lineup.json",
	}
}

// DeviceID creates a deterministic ID with the checksum required by
// the network tuner protocol. The lineup id occupies the variable portion so ordinary Xivi
// installations cannot produce duplicate virtual tuners.
func DeviceID(lineupID int64) uint32 {
	base := uint32(0x10500000) | (uint32(lineupID)&0xFFFF)<<4
	for checksum := uint32(0); checksum < 16; checksum++ {
		candidate := base | checksum
		if ValidDeviceID(candidate) {
			return candidate
		}
	}
	return base
}

func ValidDeviceID(deviceID uint32) bool {
	lookup := [16]uint8{0xA, 0x5, 0xF, 0x6, 0x7, 0xC, 0x1, 0xB, 0x9, 0x2, 0x8, 0xD, 0x4, 0x3, 0xE, 0x0}
	checksum := lookup[(deviceID>>28)&0x0F]
	checksum ^= uint8((deviceID >> 24) & 0x0F)
	checksum ^= lookup[(deviceID>>20)&0x0F]
	checksum ^= uint8((deviceID >> 16) & 0x0F)
	checksum ^= lookup[(deviceID>>12)&0x0F]
	checksum ^= uint8((deviceID >> 8) & 0x0F)
	checksum ^= lookup[(deviceID>>4)&0x0F]
	checksum ^= uint8(deviceID & 0x0F)
	return checksum == 0
}

func (device Device) Discover() DiscoverData {
	return DiscoverData{
		FriendlyName:    fmt.Sprintf("Xivi - %s", device.LineupName),
		Manufacturer:    Manufacturer,
		ModelNumber:     ModelNumber,
		FirmwareName:    FirmwareName,
		TunerCount:      int(device.TunerCount),
		FirmwareVersion: FirmwareVersion,
		DeviceID:        device.DeviceIDHex,
		DeviceAuth:      device.DeviceAuth,
		BaseURL:         device.BaseURL,
		LineupURL:       device.LineupURL,
	}
}

func Status() LineupStatus {
	return LineupStatus{ScanInProgress: 0, ScanPossible: 0, Source: "Cable", SourceList: []string{"Cable"}}
}

func Lineup(origin string, channels []models.TemplateChannel) []LineupEntry {
	entries := make([]LineupEntry, 0, len(channels))
	for index, channel := range channels {
		entries = append(entries, LineupEntry{
			GuideNumber: strconv.Itoa(index + 1),
			GuideName:   channel.Name,
			URL:         strings.TrimRight(origin, "/") + "/stream/" + url.PathEscape(channel.Uuid),
		})
	}
	return entries
}

func Origin(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	if parsed, err := url.Parse(host); err == nil && parsed.Host != "" {
		return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/")
	}
	return "http://" + net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
}

func (device Device) DescriptionXML() ([]byte, error) {
	type specVersion struct {
		Major int `xml:"major"`
		Minor int `xml:"minor"`
	}
	type descriptionDevice struct {
		DeviceType       string `xml:"deviceType"`
		FriendlyName     string `xml:"friendlyName"`
		Manufacturer     string `xml:"manufacturer"`
		ModelDescription string `xml:"modelDescription"`
		ModelName        string `xml:"modelName"`
		ModelNumber      string `xml:"modelNumber"`
		SerialNumber     string `xml:"serialNumber"`
		UDN              string `xml:"UDN"`
		PresentationURL  string `xml:"presentationURL"`
	}
	type root struct {
		XMLName     xml.Name          `xml:"root"`
		XMLNS       string            `xml:"xmlns,attr"`
		SpecVersion specVersion       `xml:"specVersion"`
		URLBase     string            `xml:"URLBase"`
		Device      descriptionDevice `xml:"device"`
	}
	description := root{
		XMLNS:       "urn:schemas-upnp-org:device-1-0",
		SpecVersion: specVersion{Major: 1, Minor: 0},
		URLBase:     device.BaseURL,
		Device: descriptionDevice{
			DeviceType:       "urn:schemas-upnp-org:device:MediaServer:1",
			FriendlyName:     fmt.Sprintf("Xivi - %s", device.LineupName),
			Manufacturer:     Manufacturer,
			ModelDescription: ModelName,
			ModelName:        ModelName,
			ModelNumber:      ModelNumber,
			SerialNumber:     device.DeviceIDHex,
			UDN:              "uuid:" + device.DeviceIDHex,
			PresentationURL:  device.BaseURL,
		},
	}
	encoded, err := xml.MarshalIndent(description, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), encoded...), nil
}
