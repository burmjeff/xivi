package streaming

import "testing"

func TestTSProbeRequiresPATAndPMT(t *testing.T) {
	probe := &tsProbe{}
	transport := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	transport = append(transport, tsPacket(0x1fff, nil)...)

	if probe.Push(transport[:200]) {
		t.Fatal("probe became ready before complete programme tables")
	}
	if !probe.Push(transport[200:]) {
		t.Fatal("probe did not recognize valid PAT and PMT")
	}
	if !probe.HasVideo() {
		t.Fatal("probe did not recognize the H.264 PMT as video")
	}
}

func TestTSProbeRecognizesAudioOnlyPMT(t *testing.T) {
	probe := &tsProbe{}
	transport := append(tsPacket(0, patSection()), tsPacket(0x0100, audioPMTSection())...)
	transport = append(transport, tsPacket(0x1fff, nil)...)
	if !probe.Push(transport) {
		t.Fatal("probe did not recognize audio-only PAT and PMT")
	}
	if probe.HasVideo() {
		t.Fatal("audio-only PMT was classified as video")
	}
	if !probe.HasAudio() {
		t.Fatal("audio-only PMT was not classified as audio")
	}
}

func TestTSProbeTracksExpandedProgrammeMap(t *testing.T) {
	probe := &tsProbe{}
	video := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	video = append(video, tsPacket(0x1fff, nil)...)
	if !probe.Push(video) || !probe.HasVideo() || probe.HasAudio() {
		t.Fatal("probe did not recognize the initial video-only programme")
	}
	expanded := append(tsPacket(0, patSection()), tsPacket(0x0100, audioVideoPMTSection())...)
	expanded = append(expanded, tsPacket(0x1fff, nil)...)
	if !probe.Push(expanded) || !probe.HasVideo() || !probe.HasAudio() {
		t.Fatal("probe froze on the first PMT instead of tracking the added audio track")
	}
}

func TestTSProbeRejectsNonTransportResponse(t *testing.T) {
	probe := &tsProbe{}
	if probe.Push([]byte("<html><title>provider error</title></html>")) {
		t.Fatal("HTML response was accepted as MPEG-TS")
	}
}

func tsPacket(pid uint16, section []byte) []byte {
	packet := make([]byte, 188)
	for index := range packet {
		packet[index] = 0xff
	}
	packet[0] = 0x47
	packet[1] = byte(pid>>8) & 0x1f
	packet[2] = byte(pid)
	packet[3] = 0x10
	if section != nil {
		packet[1] |= 0x40
		packet[4] = 0
		copy(packet[5:], section)
	}
	return packet
}

func patSection() []byte {
	return []byte{
		0x00, 0xb0, 0x0d,
		0x00, 0x01, 0xc1, 0x00, 0x00,
		0x00, 0x01, 0xe1, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
}

func pmtSection() []byte {
	return pmtSectionForType(0x1b)
}

func pmtSectionForType(streamType byte) []byte {
	return []byte{
		0x02, 0xb0, 0x12,
		0x00, 0x01, 0xc1, 0x00, 0x00,
		0xe1, 0x01, 0xf0, 0x00,
		streamType, 0xe1, 0x01, 0xf0, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
}

func audioPMTSection() []byte {
	return []byte{
		0x02, 0xb0, 0x12,
		0x00, 0x01, 0xc1, 0x00, 0x00,
		0xe1, 0x01, 0xf0, 0x00,
		0x0f, 0xe1, 0x01, 0xf0, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
}
