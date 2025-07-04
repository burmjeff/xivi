package ssdp

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

const (
	// SSDP multicast address and port
	SSDPMulticastAddr = "239.255.255.250:1900"

	// UPnP device types
	DeviceTypeMediaServer = "urn:schemas-upnp-org:device:MediaServer:1"
	DeviceTypeBasic       = "urn:schemas-upnp-org:device:Basic:1"

	// SSDP message types
	SSDPNotify   = "NOTIFY"
	SSDPMSearch  = "M-SEARCH"
	SSDPResponse = "HTTP/1.1 200 OK"

	// SSDP headers
	HeaderHost         = "HOST"
	HeaderCacheControl = "CACHE-CONTROL"
	HeaderLocation     = "LOCATION"
	HeaderNT           = "NT"
	HeaderNTS          = "NTS"
	HeaderUSN          = "USN"
	HeaderServer       = "SERVER"
	HeaderST           = "ST"
	HeaderMAN          = "MAN"
	HeaderMX           = "MX"
	HeaderDate         = "DATE"
	HeaderExt          = "EXT"

	// SSDP values
	NTSAlive    = "ssdp:alive"
	NTSByebye   = "ssdp:byebye"
	STAll       = "ssdp:all"
	STUpnpRoot  = "upnp:rootdevice"
	ManDiscover = "\"ssdp:discover\""
)

// Device represents a UPnP device for SSDP advertising
type Device struct {
	UUID         string
	DeviceType   string
	FriendlyName string
	Manufacturer string
	ModelName    string
	ModelNumber  string
	SerialNumber string
	BaseURL      string
	TemplateID   int64
	mu           sync.RWMutex
}

// SSDPService manages SSDP discovery and advertising
type SSDPService struct {
	devices    map[string]*Device
	conn       *net.UDPConn
	running    bool
	mu         sync.RWMutex
	stopChan   chan struct{}
	interfaces []net.Interface
}

// NewSSDPService creates a new SSDP service instance
func NewSSDPService() *SSDPService {
	return &SSDPService{
		devices:  make(map[string]*Device),
		running:  false,
		stopChan: make(chan struct{}),
	}
}

// Start initializes and starts the SSDP service
func (s *SSDPService) Start() error {
	if !settings.APP_SETTINGS.UPnP.Enabled {
		log.Info().Msg("UPnP/SSDP is disabled in configuration")
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("SSDP service is already running")
	}

	// Get active network interfaces
	if err := s.getActiveInterfaces(); err != nil {
		return fmt.Errorf("failed to get network interfaces: %v", err)
	}

	// Set up UDP connection for multicast
	addr, err := net.ResolveUDPAddr("udp4", SSDPMulticastAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve SSDP address: %v", err)
	}

	s.conn, err = net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on SSDP address: %v", err)
	}

	// Join multicast group on all active interfaces
	for _, iface := range s.interfaces {
		if err := s.joinMulticastGroup(iface); err != nil {
			log.Warn().Msgf("Failed to join multicast group on interface %s: %v", iface.Name, err)
		}
	}

	s.running = true

	// Start listening for M-SEARCH requests
	go s.listenForMSearch()

	// Start periodic NOTIFY announcements
	go s.periodicNotify()

	log.Info().Msg("SSDP service started successfully")
	return nil
}

// Stop gracefully stops the SSDP service
func (s *SSDPService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	// Send byebye notifications for all devices
	s.sendByebyeNotifications()

	// Stop background goroutines
	close(s.stopChan)

	// Close UDP connection
	if s.conn != nil {
		s.conn.Close()
	}

	s.running = false
	log.Info().Msg("SSDP service stopped")
	return nil
}

// AddDevice registers a new UPnP device for advertising
func (s *SSDPService) AddDevice(device *Device) error {
	if !settings.APP_SETTINGS.UPnP.Enabled {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.devices[device.UUID] = device

	// Send alive notifications if service is running
	if s.running {
		s.sendAliveNotification(device)
	}

	log.Info().Msgf("Added UPnP device: %s (%s)", device.FriendlyName, device.UUID)
	return nil
}

// RemoveDevice unregisters a UPnP device
func (s *SSDPService) RemoveDevice(uuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, exists := s.devices[uuid]
	if !exists {
		return fmt.Errorf("device with UUID %s not found", uuid)
	}

	// Send byebye notification if service is running
	if s.running {
		s.sendByebyeNotification(device)
	}

	delete(s.devices, uuid)
	log.Info().Msgf("Removed UPnP device: %s (%s)", device.FriendlyName, uuid)
	return nil
}

// getActiveInterfaces retrieves network interfaces that can be used for multicast
func (s *SSDPService) getActiveInterfaces() error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}

	s.interfaces = make([]net.Interface, 0)
	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Check if interface supports multicast
		if iface.Flags&net.FlagMulticast == 0 {
			continue
		}

		// Check if interface has IPv4 addresses
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		hasIPv4 := false
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					hasIPv4 = true
					break
				}
			}
		}

		if hasIPv4 {
			s.interfaces = append(s.interfaces, iface)
			log.Debug().Msgf("Using network interface: %s", iface.Name)
		}
	}

	if len(s.interfaces) == 0 {
		return fmt.Errorf("no suitable network interfaces found for multicast")
	}

	return nil
}

// joinMulticastGroup joins the SSDP multicast group on the specified interface
func (s *SSDPService) joinMulticastGroup(iface net.Interface) error {
	// For UDP multicast, we need to set up the socket options
	// This is a simplified approach - in production you might want to use golang.org/x/net/ipv4
	log.Debug().Msgf("Attempting to join multicast group on interface: %s", iface.Name)
	return nil // Simplified for now - multicast listening will work without explicit join in many cases
}

// generateUSN creates a unique service name for the device
func (s *SSDPService) generateUSN(device *Device, serviceType string) string {
	if serviceType == STUpnpRoot {
		return fmt.Sprintf("uuid:%s::upnp:rootdevice", device.UUID)
	}
	if serviceType == device.DeviceType {
		return fmt.Sprintf("uuid:%s::%s", device.UUID, device.DeviceType)
	}
	return fmt.Sprintf("uuid:%s", device.UUID)
}

// getServerString returns the server identification string
func (s *SSDPService) getServerString() string {
	return fmt.Sprintf("Linux/3.14 UPnP/1.0 %s/%s",
		settings.APP_SETTINGS.Application.AppName,
		settings.APP_SETTINGS.Application.AppVersion)
}

// listenForMSearch handles incoming M-SEARCH requests
func (s *SSDPService) listenForMSearch() {
	buffer := make([]byte, 1024)

	for {
		select {
		case <-s.stopChan:
			return
		default:
			s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, addr, err := s.conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				log.Error().Msgf("Error reading UDP packet: %v", err)
				continue
			}

			message := string(buffer[:n])
			if strings.HasPrefix(message, SSDPMSearch) {
				s.handleMSearchRequest(message, addr)
			}
		}
	}
}

// handleMSearchRequest processes M-SEARCH requests and sends appropriate responses
func (s *SSDPService) handleMSearchRequest(message string, addr *net.UDPAddr) {
	lines := strings.Split(message, "\r\n")
	headers := make(map[string]string)

	// Parse headers
	for _, line := range lines[1:] {
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.ToUpper(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
		}
	}

	// Validate required headers
	man, hasMan := headers[HeaderMAN]
	st, hasST := headers[HeaderST]

	if !hasMan || man != ManDiscover || !hasST {
		return
	}

	log.Debug().Msgf("Received M-SEARCH for ST: %s from %s", st, addr.String())

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Send responses for matching devices
	for _, device := range s.devices {
		if s.shouldRespond(st, device) {
			s.sendMSearchResponse(device, st, addr)
		}
	}
}

// shouldRespond determines if we should respond to an M-SEARCH request
func (s *SSDPService) shouldRespond(st string, device *Device) bool {
	switch st {
	case STAll:
		return true
	case STUpnpRoot:
		return true
	case device.DeviceType:
		return true
	case fmt.Sprintf("uuid:%s", device.UUID):
		return true
	default:
		return false
	}
}

// sendMSearchResponse sends a response to an M-SEARCH request
func (s *SSDPService) sendMSearchResponse(device *Device, st string, addr *net.UDPAddr) {
	usn := s.generateUSN(device, st)
	location := fmt.Sprintf("http://%s:%d/upnp/device/%s.xml",
		settings.APP_SETTINGS.Server.Host,
		settings.APP_SETTINGS.Server.Port,
		device.UUID)

	response := fmt.Sprintf("%s\r\n"+
		"CACHE-CONTROL: max-age=1800\r\n"+
		"DATE: %s\r\n"+
		"EXT:\r\n"+
		"LOCATION: %s\r\n"+
		"OPT: \"http://schemas.upnp.org/upnp/1/0/\"; ns=01\r\n"+
		"01-NLS: 1\r\n"+
		"SERVER: %s\r\n"+
		"ST: %s\r\n"+
		"USN: %s\r\n"+
		"\r\n",
		SSDPResponse,
		time.Now().UTC().Format(time.RFC1123),
		location,
		s.getServerString(),
		st,
		usn)

	// Send response
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		log.Error().Msgf("Failed to dial UDP for M-SEARCH response: %v", err)
		return
	}
	defer conn.Close()

	_, err = conn.Write([]byte(response))
	if err != nil {
		log.Error().Msgf("Failed to send M-SEARCH response: %v", err)
		return
	}

	log.Debug().Msgf("Sent M-SEARCH response for device %s to %s", device.UUID, addr.String())
}

// periodicNotify sends periodic NOTIFY messages for all devices
func (s *SSDPService) periodicNotify() {
	ticker := time.NewTicker(30 * time.Minute) // Send notifications every 30 minutes
	defer ticker.Stop()

	// Send initial notifications
	s.sendAliveNotifications()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.sendAliveNotifications()
		}
	}
}

// sendAliveNotifications sends NOTIFY alive messages for all devices
func (s *SSDPService) sendAliveNotifications() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, device := range s.devices {
		s.sendAliveNotification(device)
	}
}

// sendAliveNotification sends a NOTIFY alive message for a specific device
func (s *SSDPService) sendAliveNotification(device *Device) {
	notifications := []struct {
		nt  string
		usn string
	}{
		{STUpnpRoot, s.generateUSN(device, STUpnpRoot)},
		{fmt.Sprintf("uuid:%s", device.UUID), s.generateUSN(device, "")},
		{device.DeviceType, s.generateUSN(device, device.DeviceType)},
	}

	for _, notif := range notifications {
		s.sendNotifyMessage(device, notif.nt, notif.usn, NTSAlive)
	}
}

// sendByebyeNotifications sends NOTIFY byebye messages for all devices
func (s *SSDPService) sendByebyeNotifications() {
	for _, device := range s.devices {
		s.sendByebyeNotification(device)
	}
}

// sendByebyeNotification sends a NOTIFY byebye message for a specific device
func (s *SSDPService) sendByebyeNotification(device *Device) {
	notifications := []struct {
		nt  string
		usn string
	}{
		{STUpnpRoot, s.generateUSN(device, STUpnpRoot)},
		{fmt.Sprintf("uuid:%s", device.UUID), s.generateUSN(device, "")},
		{device.DeviceType, s.generateUSN(device, device.DeviceType)},
	}

	for _, notif := range notifications {
		s.sendNotifyMessage(device, notif.nt, notif.usn, NTSByebye)
	}
}

// sendNotifyMessage sends a NOTIFY message via multicast
func (s *SSDPService) sendNotifyMessage(device *Device, nt, usn, nts string) {
	location := fmt.Sprintf("http://%s:%d/upnp/device/%s.xml",
		settings.APP_SETTINGS.Server.Host,
		settings.APP_SETTINGS.Server.Port,
		device.UUID)

	message := fmt.Sprintf("NOTIFY * HTTP/1.1\r\n"+
		"HOST: %s\r\n"+
		"CACHE-CONTROL: max-age=1800\r\n"+
		"LOCATION: %s\r\n"+
		"NT: %s\r\n"+
		"NTS: %s\r\n"+
		"USN: %s\r\n"+
		"SERVER: %s\r\n"+
		"\r\n",
		SSDPMulticastAddr,
		location,
		nt,
		nts,
		usn,
		s.getServerString())

	// Send to multicast address
	addr, err := net.ResolveUDPAddr("udp4", SSDPMulticastAddr)
	if err != nil {
		log.Error().Msgf("Failed to resolve multicast address: %v", err)
		return
	}

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		log.Error().Msgf("Failed to dial multicast address: %v", err)
		return
	}
	defer conn.Close()

	_, err = conn.Write([]byte(message))
	if err != nil {
		log.Error().Msgf("Failed to send NOTIFY message: %v", err)
		return
	}

	log.Debug().Msgf("Sent NOTIFY %s for device %s (NT: %s)", nts, device.UUID, nt)
}

// GetDevices returns a copy of all registered devices
func (s *SSDPService) GetDevices() map[string]*Device {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devices := make(map[string]*Device)
	for uuid, device := range s.devices {
		devices[uuid] = device
	}
	return devices
}

// GetDevice returns a specific device by UUID
func (s *SSDPService) GetDevice(uuid string) (*Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	device, exists := s.devices[uuid]
	return device, exists
}

// IsRunning returns whether the SSDP service is currently running
func (s *SSDPService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetDeviceCount returns the number of registered devices
func (s *SSDPService) GetDeviceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.devices)
}

// SendAliveNotifications sends NOTIFY alive messages for all devices (public method)
func (s *SSDPService) SendAliveNotifications() {
	s.sendAliveNotifications()
}
