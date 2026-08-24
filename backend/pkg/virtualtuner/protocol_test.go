package virtualtuner

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
	"xivi/backend/app/models"
)

func discoveryRequestPacket(deviceID uint32) []byte {
	payload := make([]byte, 0, 12)
	payload = appendUint32TLV(payload, tagDeviceType, deviceTypeTuner)
	payload = appendUint32TLV(payload, tagDeviceID, deviceID)
	return sealFrame(typeDiscoverRequest, payload)
}

func decodeReplyDeviceID(t *testing.T, packet []byte) uint32 {
	t.Helper()
	payload, packetType, err := openFrame(packet)
	if err != nil {
		t.Fatalf("open reply: %v", err)
	}
	if packetType != typeDiscoverReply {
		t.Fatalf("packet type = %04x, want %04x", packetType, typeDiscoverReply)
	}
	for len(payload) > 0 {
		tag, value, rest, ok := readTLV(payload)
		if !ok {
			t.Fatal("invalid reply TLV")
		}
		if tag == tagDeviceID && len(value) == 4 {
			return binary.BigEndian.Uint32(value)
		}
		payload = rest
	}
	t.Fatal("reply has no device id")
	return 0
}

func TestDeviceIDsAreStableValidAndDistinct(t *testing.T) {
	seen := map[uint32]bool{}
	for lineupID := int64(1); lineupID <= 1000; lineupID++ {
		first := DeviceID(lineupID)
		if first != DeviceID(lineupID) {
			t.Fatalf("device id for lineup %d is not stable", lineupID)
		}
		if !ValidDeviceID(first) {
			t.Fatalf("device id %08X for lineup %d has an invalid checksum", first, lineupID)
		}
		if seen[first] {
			t.Fatalf("device id %08X is duplicated", first)
		}
		seen[first] = true
	}
}

func TestServiceRepliesOnceForEveryLineup(t *testing.T) {
	provider := func(origin string) ([]Device, error) {
		return []Device{
			NewDevice(models.Template{ID: 1, Name: "News"}, "http://192.0.2.10:3000", 4),
			NewDevice(models.Template{ID: 2, Name: "Sports"}, "http://192.0.2.10:3000", 4),
		}, nil
	}
	service := NewService("127.0.0.1:0", provider)
	if err := service.Start(); err != nil {
		t.Fatalf("start discovery: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	server := service.Addr().(*net.UDPAddr)
	client, err := net.DialUDP("udp4", nil, server)
	if err != nil {
		t.Fatalf("dial discovery: %v", err)
	}
	defer client.Close()
	if _, err := client.Write(discoveryRequestPacket(deviceIDWildcard)); err != nil {
		t.Fatalf("send discovery: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	found := map[uint32]bool{}
	buffer := make([]byte, 2048)
	for range 2 {
		count, err := client.Read(buffer)
		if err != nil {
			t.Fatalf("read discovery reply: %v", err)
		}
		found[decodeReplyDeviceID(t, buffer[:count])] = true
	}
	if !found[DeviceID(1)] || !found[DeviceID(2)] {
		t.Fatalf("discovered ids = %v", found)
	}
}

func TestServiceHonorsTargetDeviceID(t *testing.T) {
	devices := []Device{
		NewDevice(models.Template{ID: 1, Name: "One"}, "http://127.0.0.1:3000", 2),
		NewDevice(models.Template{ID: 2, Name: "Two"}, "http://127.0.0.1:3000", 2),
	}
	service := NewService("127.0.0.1:0", func(string) ([]Device, error) { return devices, nil })
	if err := service.Start(); err != nil {
		t.Fatalf("start discovery: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	client, err := net.DialUDP("udp4", nil, service.Addr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write(discoveryRequestPacket(devices[1].DeviceID)); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	buffer := make([]byte, 2048)
	count, err := client.Read(buffer)
	if err != nil {
		t.Fatalf("read targeted reply: %v", err)
	}
	if got := decodeReplyDeviceID(t, buffer[:count]); got != devices[1].DeviceID {
		t.Fatalf("device id = %08X, want %08X", got, devices[1].DeviceID)
	}
	_ = client.SetReadDeadline(time.Now().Add(75 * time.Millisecond))
	if _, err := client.Read(buffer); err == nil {
		t.Fatal("targeted request returned more than one device")
	}
}

func TestHTTPContractUsesOrderedProxyStreamsAndEscapedXML(t *testing.T) {
	device := NewDevice(models.Template{ID: 7, Name: "Kids & Family", VirtualTunerEnabled: true}, "http://xivi.local:3000", 6)
	if !device.Enabled {
		t.Fatal("device did not retain lineup enablement")
	}
	if device.Discover().LineupURL != "http://xivi.local:3000/virtual-tuner/7/lineup.json" {
		t.Fatalf("lineup URL = %q", device.Discover().LineupURL)
	}
	entries := Lineup("http://xivi.local:3000", []models.TemplateChannel{
		{Name: "First", Uuid: "first"},
		{Name: "Second", Uuid: "second"},
	})
	if entries[0].GuideNumber != "1" || entries[1].GuideNumber != "2" {
		t.Fatalf("guide numbers = %#v", entries)
	}
	if entries[1].URL != "http://xivi.local:3000/stream/second" {
		t.Fatalf("stream URL = %q", entries[1].URL)
	}
	description, err := device.DescriptionXML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(description), "Kids &amp; Family") {
		t.Fatalf("description does not XML-escape lineup name: %s", description)
	}
}
