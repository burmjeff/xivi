package virtualtuner

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"xivi/backend/pkg/security"
	"xivi/backend/platform/settings"

	"github.com/rs/zerolog/log"
)

type DeviceProvider func(origin string) ([]Device, error)

type Service struct {
	address  string
	provider DeviceProvider

	mu      sync.RWMutex
	conn    *net.UDPConn
	running bool
	wg      sync.WaitGroup
}

func NewService(address string, provider DeviceProvider) *Service {
	return &Service{address: address, provider: provider}
}

var defaultService = NewService(fmt.Sprintf(":%d", discoveryPort), Devices)

func Start() error { return defaultService.Start() }

func Stop() error { return defaultService.Stop() }

func Running() bool { return defaultService.Running() }

func Reconfigure() error {
	devices, err := Devices("")
	if err != nil {
		return fmt.Errorf("load enabled virtual tuner lineups: %w", err)
	}
	if len(devices) > 0 {
		return Start()
	}
	return Stop()
}

func (service *Service) Start() error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.running {
		return nil
	}
	address, err := net.ResolveUDPAddr("udp4", service.address)
	if err != nil {
		return fmt.Errorf("resolve virtual tuner discovery address: %w", err)
	}
	conn, err := net.ListenUDP("udp4", address)
	if err != nil {
		return fmt.Errorf("listen for virtual tuner discovery on UDP %d: %w", discoveryPort, err)
	}
	service.conn = conn
	service.running = true
	service.wg.Add(1)
	go service.serve(conn)
	log.Info().Str("address", conn.LocalAddr().String()).Msg("Virtual tuner discovery started")
	return nil
}

func (service *Service) Stop() error {
	service.mu.Lock()
	if !service.running {
		service.mu.Unlock()
		return nil
	}
	conn := service.conn
	service.conn = nil
	service.running = false
	service.mu.Unlock()

	err := conn.Close()
	service.wg.Wait()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	log.Info().Msg("Virtual tuner discovery stopped")
	return nil
}

func (service *Service) Running() bool {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.running
}

func (service *Service) Addr() net.Addr {
	service.mu.RLock()
	defer service.mu.RUnlock()
	if service.conn == nil {
		return nil
	}
	return service.conn.LocalAddr()
}

func (service *Service) serve(conn *net.UDPConn) {
	defer service.wg.Done()
	buffer := make([]byte, 2048)
	for {
		count, remote, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Warn().Err(err).Msg("Virtual tuner discovery read failed")
			continue
		}
		if remote == nil || !security.IsTrustedLAN(remote.IP) {
			// UDP discovery has no authentication handshake. Never confirm the
			// existence of a tuner to an address outside the explicit LAN policy.
			continue
		}
		request, err := parseDiscoveryRequest(buffer[:count])
		if err != nil || !request.wantsTuner {
			continue
		}
		origin := advertisedOrigin(remote)
		devices, err := service.provider(origin)
		if err != nil {
			log.Error().Err(err).Msg("Virtual tuner lineups could not be loaded")
			continue
		}
		for _, device := range devices {
			if request.deviceID != deviceIDWildcard && request.deviceID != device.DeviceID {
				continue
			}
			if _, err := conn.WriteToUDP(buildDiscoveryReply(device), remote); err != nil && !errors.Is(err, net.ErrClosed) {
				log.Warn().Err(err).Str("device_id", device.DeviceIDHex).Msg("Virtual tuner discovery reply failed")
			}
		}
	}
}

func advertisedOrigin(remote *net.UDPAddr) string {
	host := strings.TrimSpace(settings.APP_SETTINGS.Server.Host)
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if host == "" || strings.EqualFold(host, "localhost") || host == "0.0.0.0" || host == "::" || (ip != nil && ip.IsLoopback() && !remote.IP.IsLoopback()) {
		if conn, err := net.DialUDP("udp4", nil, remote); err == nil {
			if local, ok := conn.LocalAddr().(*net.UDPAddr); ok {
				host = local.IP.String()
			}
			_ = conn.Close()
		}
	}
	return Origin(host, settings.APP_SETTINGS.Server.Port)
}
