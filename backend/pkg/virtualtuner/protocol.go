package virtualtuner

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

const (
	discoveryPort = 65001

	typeDiscoverRequest = 0x0002
	typeDiscoverReply   = 0x0003

	tagDeviceType = 0x01
	tagDeviceID   = 0x02
	tagTunerCount = 0x10
	tagLineupURL  = 0x27
	tagBaseURL    = 0x2A
	tagDeviceAuth = 0x2B
	tagMultiType  = 0x2D

	deviceTypeWildcard = 0xFFFFFFFF
	deviceTypeTuner    = 0x00000001
	deviceIDWildcard   = 0xFFFFFFFF
)

type discoveryRequest struct {
	deviceID   uint32
	wantsTuner bool
}

func parseDiscoveryRequest(packet []byte) (discoveryRequest, error) {
	request := discoveryRequest{deviceID: deviceIDWildcard}
	payload, packetType, err := openFrame(packet)
	if err != nil {
		return request, err
	}
	if packetType != typeDiscoverRequest {
		return request, errors.New("not a virtual tuner discovery request")
	}

	for len(payload) > 0 {
		tag, value, rest, ok := readTLV(payload)
		if !ok {
			return request, errors.New("invalid discovery request payload")
		}
		payload = rest
		switch tag {
		case tagDeviceType:
			if len(value) == 4 {
				kind := binary.BigEndian.Uint32(value)
				request.wantsTuner = kind == deviceTypeTuner || kind == deviceTypeWildcard
			}
		case tagMultiType:
			for len(value) >= 4 {
				kind := binary.BigEndian.Uint32(value[:4])
				if kind == deviceTypeTuner || kind == deviceTypeWildcard {
					request.wantsTuner = true
				}
				value = value[4:]
			}
		case tagDeviceID:
			if len(value) == 4 {
				request.deviceID = binary.BigEndian.Uint32(value)
			}
		}
	}

	return request, nil
}

func buildDiscoveryReply(device Device) []byte {
	payload := make([]byte, 0, 160)
	payload = appendUint32TLV(payload, tagDeviceType, deviceTypeTuner)
	payload = appendUint32TLV(payload, tagDeviceID, device.DeviceID)
	payload = appendTLV(payload, tagTunerCount, []byte{device.TunerCount})
	payload = appendTLV(payload, tagBaseURL, []byte(device.BaseURL))
	payload = appendTLV(payload, tagLineupURL, []byte(device.LineupURL))
	if device.DeviceAuth != "" {
		payload = appendTLV(payload, tagDeviceAuth, []byte(device.DeviceAuth))
	}
	return sealFrame(typeDiscoverReply, payload)
}

func openFrame(packet []byte) ([]byte, uint16, error) {
	if len(packet) < 8 {
		return nil, 0, errors.New("virtual tuner frame is too short")
	}
	length := int(binary.BigEndian.Uint16(packet[2:4]))
	frameLength := 4 + length + 4
	if frameLength != len(packet) {
		return nil, 0, errors.New("virtual tuner frame length is invalid")
	}
	wantCRC := binary.LittleEndian.Uint32(packet[4+length:])
	if crc32.ChecksumIEEE(packet[:4+length]) != wantCRC {
		return nil, 0, errors.New("virtual tuner frame checksum is invalid")
	}
	return packet[4 : 4+length], binary.BigEndian.Uint16(packet[:2]), nil
}

func sealFrame(packetType uint16, payload []byte) []byte {
	packet := make([]byte, 4+len(payload)+4)
	binary.BigEndian.PutUint16(packet[:2], packetType)
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(payload)))
	copy(packet[4:], payload)
	binary.LittleEndian.PutUint32(packet[4+len(payload):], crc32.ChecksumIEEE(packet[:4+len(payload)]))
	return packet
}

func appendUint32TLV(payload []byte, tag byte, value uint32) []byte {
	encoded := make([]byte, 4)
	binary.BigEndian.PutUint32(encoded, value)
	return appendTLV(payload, tag, encoded)
}

func appendTLV(payload []byte, tag byte, value []byte) []byte {
	payload = append(payload, tag)
	length := len(value)
	if length <= 127 {
		payload = append(payload, byte(length))
	} else {
		payload = append(payload, byte(length&0x7F)|0x80, byte(length>>7))
	}
	return append(payload, value...)
}

func readTLV(payload []byte) (byte, []byte, []byte, bool) {
	if len(payload) < 2 {
		return 0, nil, nil, false
	}
	tag := payload[0]
	length := int(payload[1])
	headerLength := 2
	if length&0x80 != 0 {
		if len(payload) < 3 {
			return 0, nil, nil, false
		}
		length = length&0x7F | int(payload[2])<<7
		headerLength = 3
	}
	if length < 0 || headerLength+length > len(payload) {
		return 0, nil, nil, false
	}
	return tag, payload[headerLength : headerLength+length], payload[headerLength+length:], true
}
