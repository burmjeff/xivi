package streaming

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

func TestHubFansOutAndReplaysWarmData(t *testing.T) {
	hub := NewHub(1024 * 1024)
	warm := []byte("warm transport data")
	hub.Publish(warm)

	first := hub.Subscribe()
	second := hub.Subscribe()
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	for name, subscription := range map[string]*Subscription{"first": first, "second": second} {
		chunk, ok := subscription.Next()
		if !ok || !bytes.Equal(chunk, warm) {
			t.Fatalf("%s subscriber did not receive the warm buffer", name)
		}
	}

	live := []byte("live transport data")
	hub.Publish(live)
	for name, subscription := range map[string]*Subscription{"first": first, "second": second} {
		chunk, ok := subscription.Next()
		if !ok || !bytes.Equal(chunk, live) {
			t.Fatalf("%s subscriber did not receive live fan-out", name)
		}
	}
}

func TestHubReplaysVideoFromLatestDecoderBootstrap(t *testing.T) {
	hub := NewHub(1024 * 1024)
	oldDelta := append(tsPacket(0x0101, nil), tsPacket(0x0101, nil)...)
	oldDelta = append(oldDelta, tsPacket(0x0101, nil)...)
	tables := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	tables = append(tables, tsPacket(0x1fff, nil)...)
	clock := pcrPacket(0x0101)
	keyframe := randomAccessPacket(0x0101)
	live := append(tsPacket(0x0101, nil), tsPacket(0x0101, nil)...)
	live = append(live, tsPacket(0x0101, nil)...)

	hub.Publish(oldDelta)
	hub.Publish(tables)
	if hub.HasDecoderBootstrap(true) {
		t.Fatal("video bootstrap became ready without a random-access packet")
	}
	if !hub.HasDecoderBootstrap(false) {
		t.Fatal("audio bootstrap did not accept valid programme tables")
	}
	hub.Publish(clock)
	hub.Publish(keyframe)
	hub.Publish(live)
	if !hub.HasDecoderBootstrap(true) {
		t.Fatal("video bootstrap did not recognize programme tables and keyframe")
	}

	subscription := hub.Subscribe()
	t.Cleanup(subscription.Close)
	programmeClock := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	programmeClock = append(programmeClock, bootstrapPCRPacket(clock)...)
	for index, expected := range [][]byte{programmeClock, keyframe, live} {
		chunk, ok := subscription.Next()
		if !ok || !bytes.Equal(chunk, expected) {
			t.Fatalf("bootstrap chunk %d did not match expected decoder-safe replay", index)
		}
	}
}

func TestHubStartsOnExactVideoRandomAccessPacket(t *testing.T) {
	hub := NewHub(1024 * 1024)
	pat := tsPacket(0, patSection())
	pmt := tsPacket(0x0100, pmtSection())
	delta := tsPacket(0x0101, nil)
	clock := pcrPacket(0x0101)
	keyframe := randomAccessPacket(0x0101)
	afterKeyframe := tsPacket(0x0102, nil)
	combined := append([]byte{}, pat...)
	combined = append(combined, pmt...)
	combined = append(combined, delta...)
	combined = append(combined, clock...)
	combined = append(combined, keyframe...)
	combined = append(combined, afterKeyframe...)

	hub.Publish(combined)
	subscription := hub.Subscribe()
	t.Cleanup(subscription.Close)

	prefix, ok := subscription.Next()
	wantPrefix := append(append([]byte{}, pat...), pmt...)
	wantPrefix = append(wantPrefix, bootstrapPCRPacket(clock)...)
	if !ok || !bytes.Equal(prefix, wantPrefix) {
		t.Fatal("video bootstrap did not emit a compact PAT/PMT/PCR prefix")
	}
	media, ok := subscription.Next()
	wantMedia := append(append([]byte{}, keyframe...), afterKeyframe...)
	if !ok || !bytes.Equal(media, wantMedia) {
		t.Fatal("video bootstrap included multiplexed packets from before the keyframe")
	}
}

func TestInspectTransportChunkFindsRandomAccessFlag(t *testing.T) {
	metadata := inspectTransportChunk(randomAccessPacket(0x0101))
	if !metadata.isTransport {
		t.Fatal("valid transport packet was not recognized")
	}
	if !metadata.randomAccess {
		t.Fatal("transport random-access indicator was not detected")
	}
}

func TestHubRequiresProgramClockForVideoBootstrap(t *testing.T) {
	hub := NewHub(1024 * 1024)
	tables := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	hub.Publish(tables)
	hub.Publish(randomAccessPacket(0x0101))
	if hub.HasDecoderBootstrap(true) {
		t.Fatal("video bootstrap became ready without a program clock reference")
	}
	hub.Publish(pcrPacket(0x0101))
	hub.Publish(randomAccessPacket(0x0101))
	if !hub.HasDecoderBootstrap(true) {
		t.Fatal("video bootstrap did not become ready with PAT, PMT, PCR, and keyframe")
	}
}

func TestHubRequiresFreshH264ParameterSets(t *testing.T) {
	hub := NewHub(1024 * 1024)
	hub.Publish(append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...))
	hub.Publish(pcrPacket(0x0101))
	hub.Publish(randomAccessPacketForCodec(0x0101, videoCodecH264, false))
	if hub.HasDecoderBootstrap(true) {
		t.Fatal("H.264 bootstrap became ready without SPS and PPS")
	}
	incomplete := hub.BootstrapStatus()
	if !containsAll(incomplete.Missing, "sps", "pps") {
		t.Fatalf("missing H.264 initialization was not diagnosed: %+v", incomplete)
	}
	hub.Publish(randomAccessPacket(0x0101))
	if !hub.HasDecoderBootstrap(true) {
		t.Fatal("H.264 bootstrap did not become ready with SPS, PPS, and IDR")
	}
	status := hub.BootstrapStatus()
	if status.VideoCodec != "h264" || !status.ParameterSetsComplete || !status.Complete {
		t.Fatalf("unexpected H.264 bootstrap status: %+v", status)
	}
}

func containsAll(values []string, expected ...string) bool {
	found := make(map[string]bool, len(values))
	for _, value := range values {
		found[value] = true
	}
	for _, value := range expected {
		if !found[value] {
			return false
		}
	}
	return true
}

func TestHubReplaysParameterOnlyPESBeforeRandomAccessPES(t *testing.T) {
	hub := NewHub(1024 * 1024)
	tables := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	clock := pcrPacket(0x0101)
	parameters := parameterOnlyPacket(0x0101, videoCodecH264)
	keyframe := randomAccessPacketForCodec(0x0101, videoCodecH264, false)
	hub.Publish(tables)
	hub.Publish(clock)
	hub.Publish(parameters)
	hub.Publish(keyframe)

	subscription := hub.Subscribe()
	t.Cleanup(subscription.Close)
	prefix, ok := subscription.Next()
	wantPrefix := append(append([]byte{}, tables...), bootstrapPCRPacket(clock)...)
	if !ok || !bytes.Equal(prefix, wantPrefix) {
		t.Fatal("split-parameter bootstrap did not emit programme prefix")
	}
	for index, expected := range [][]byte{parameters, keyframe} {
		chunk, next := subscription.Next()
		if !next || !bytes.Equal(chunk, expected) {
			t.Fatalf("split-parameter bootstrap chunk %d was not replayed", index)
		}
	}
}

func TestHubRecognizesH265AndMPEG2Initialization(t *testing.T) {
	for name, test := range map[string]struct {
		streamType byte
		codec      videoCodec
	}{
		"h265":  {streamType: 0x24, codec: videoCodecH265},
		"mpeg2": {streamType: 0x02, codec: videoCodecMPEG2},
	} {
		t.Run(name, func(t *testing.T) {
			hub := NewHub(1024 * 1024)
			hub.Publish(append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSectionForType(test.streamType))...))
			hub.Publish(pcrPacket(0x0101))
			hub.Publish(randomAccessPacketForCodec(0x0101, test.codec, true))
			if !hub.HasDecoderBootstrap(true) {
				t.Fatalf("%s bootstrap did not recognize codec initialization", name)
			}
			status := hub.BootstrapStatus()
			if status.VideoCodec != string(test.codec) || !status.ParameterSetsComplete {
				t.Fatalf("unexpected %s bootstrap status: %+v", name, status)
			}
		})
	}
}

func TestBootstrapStatusReportsFirstAudioVideoPTS(t *testing.T) {
	hub := NewHub(1024 * 1024)
	tables := append(tsPacket(0, patSection()), tsPacket(0x0100, audioVideoPMTSection())...)
	hub.Publish(tables)
	hub.Publish(pcrPacket(0x0101))
	hub.Publish(audioPESPacket(0x0102, 89_550))
	hub.Publish(randomAccessPacket(0x0101))
	status := hub.BootstrapStatus()
	if status.FirstVideoPTS90K == nil || *status.FirstVideoPTS90K != 90_000 {
		t.Fatalf("first video PTS was not reported: %+v", status.FirstVideoPTS90K)
	}
	if status.FirstAudioPTS90K == nil || *status.FirstAudioPTS90K != 89_550 {
		t.Fatalf("first audio PTS was not reported: %+v", status.FirstAudioPTS90K)
	}
	if status.AVStartDeltaMS == nil || *status.AVStartDeltaMS != 5 {
		t.Fatalf("A/V start delta = %v, want 5ms", status.AVStartDeltaMS)
	}
}

func TestBootstrapPCRPacketStripsMediaPayload(t *testing.T) {
	source := pcrPacket(0x0101)
	source[12] = 0x12
	source[13] = 0x34
	packet := bootstrapPCRPacket(source)
	if len(packet) != 188 || packet[0] != 0x47 {
		t.Fatal("bootstrap PCR packet was not valid transport size")
	}
	if packet[3]&0x30 != 0x20 || packet[5]&0x90 != 0x90 {
		t.Fatal("bootstrap PCR packet was not adaptation-only and discontinuous")
	}
	if !bytes.Equal(packet[6:12], source[6:12]) {
		t.Fatal("bootstrap PCR value changed")
	}
	for _, value := range packet[12:] {
		if value != 0xff {
			t.Fatal("bootstrap PCR leaked source media payload")
		}
	}
}

func TestHubIgnoresRandomAccessOutsideVideoPID(t *testing.T) {
	hub := NewHub(1024 * 1024)
	tables := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	tables = append(tables, tsPacket(0x1fff, nil)...)
	hub.Publish(tables)
	hub.Publish(pcrPacket(0x0101))
	hub.Publish(randomAccessPacket(0x0102))
	if hub.HasDecoderBootstrap(true) {
		t.Fatal("non-video random-access packet was accepted as a video keyframe")
	}
	hub.Publish(randomAccessPacket(0x0101))
	if !hub.HasDecoderBootstrap(true) {
		t.Fatal("declared video PID random-access packet was not accepted")
	}
}

func TestHubWaitsForNextBootstrapAfterWarmKeyframeRotatesOut(t *testing.T) {
	hub := NewHub(1024 * 1024)
	tables := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	tables = append(tables, tsPacket(0x1fff, nil)...)
	clock := pcrPacket(0x0101)
	keyframe := randomAccessPacket(0x0101)
	delta := transportPackets(0x0101, 7)

	hub.Publish(tables)
	hub.Publish(clock)
	hub.Publish(keyframe)
	for range 500 {
		hub.Publish(delta)
	}
	if hub.HasDecoderBootstrap(true) {
		t.Fatal("test did not rotate the old keyframe out of the warm ring")
	}

	subscription := hub.Subscribe()
	t.Cleanup(subscription.Close)
	if pending := subscriptionPending(subscription); pending != 0 {
		t.Fatalf("mid-GOP subscriber received %d bytes before a safe bootstrap", pending)
	}
	hub.Publish(delta)
	if pending := subscriptionPending(subscription); pending != 0 {
		t.Fatalf("mid-GOP delta data was queued while waiting: %d bytes", pending)
	}
	hub.Publish(tables)
	if pending := subscriptionPending(subscription); pending != 0 {
		t.Fatalf("video subscriber resumed from tables without a keyframe: %d bytes", pending)
	}
	hub.Publish(clock)
	hub.Publish(keyframe)

	programmeClock := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	programmeClock = append(programmeClock, bootstrapPCRPacket(clock)...)
	for index, expected := range [][]byte{programmeClock, keyframe} {
		chunk, ok := subscription.Next()
		if !ok || !bytes.Equal(chunk, expected) {
			t.Fatalf("resumed bootstrap chunk %d was not decoder-safe", index)
		}
	}
}

func TestHubAudioOnlySubscriberResumesAtProgrammeTables(t *testing.T) {
	hub := NewHub(1024 * 1024)
	subscription := hub.Subscribe()
	t.Cleanup(subscription.Close)
	tables := append(tsPacket(0, patSection()), tsPacket(0x0100, audioPMTSection())...)
	tables = append(tables, tsPacket(0x1fff, nil)...)

	hub.Publish(tables)
	chunk, ok := subscription.Next()
	if !ok || !bytes.Equal(chunk, tables) {
		t.Fatal("audio-only subscriber did not resume from PAT/PMT")
	}
}

func TestHubDiscontinuityRegatesExistingViewer(t *testing.T) {
	hub := NewHub(1024 * 1024)
	tables := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	tables = append(tables, tsPacket(0x1fff, nil)...)
	clock := pcrPacket(0x0101)
	keyframe := randomAccessPacket(0x0101)
	delta := transportPackets(0x0101, 7)
	hub.Publish(tables)
	hub.Publish(clock)
	hub.Publish(keyframe)
	subscription := hub.Subscribe()
	t.Cleanup(subscription.Close)
	_, _ = subscription.Next()
	_, _ = subscription.Next()

	hub.ResetForDiscontinuity()
	hub.Publish(delta)
	if pending := subscriptionPending(subscription); pending != 0 {
		t.Fatalf("viewer received %d bytes from a discontinuous mid-GOP source", pending)
	}
	hub.Publish(tables)
	hub.Publish(clock)
	hub.Publish(keyframe)
	programmeClock := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	programmeClock = append(programmeClock, bootstrapPCRPacket(clock)...)
	if chunk, ok := subscription.Next(); !ok || !bytes.Equal(chunk, programmeClock) {
		t.Fatal("viewer did not resume safely after source discontinuity")
	}
}

func subscriptionPending(subscription *Subscription) int {
	subscription.hub.mu.Lock()
	defer subscription.hub.mu.Unlock()
	return subscription.sub.pending
}

func transportPackets(pid uint16, count int) []byte {
	data := make([]byte, 0, count*188)
	for range count {
		data = append(data, tsPacket(pid, nil)...)
	}
	return data
}

func randomAccessPacket(pid uint16) []byte {
	return randomAccessPacketForCodec(pid, videoCodecH264, true)
}

func randomAccessPacketForCodec(pid uint16, codec videoCodec, includeParameters bool) []byte {
	packet := tsPacket(pid, nil)
	packet[1] |= 0x40
	packet[3] = 0x30
	packet[4] = 1
	packet[5] = 0x40
	payload := pesHeader(90_000)
	if includeParameters {
		switch codec {
		case videoCodecH264:
			payload = append(payload,
				0, 0, 0, 1, 0x67, 0x64, 0, 0x28,
				0, 0, 0, 1, 0x68, 0xee, 0x3c, 0x80)
		case videoCodecH265:
			payload = append(payload,
				0, 0, 0, 1, 32<<1, 1,
				0, 0, 0, 1, 33<<1, 1,
				0, 0, 0, 1, 34<<1, 1)
		case videoCodecMPEG2:
			payload = append(payload, 0, 0, 1, 0xb3, 0x2d, 0x01)
		}
	}
	if codec == videoCodecH264 {
		payload = append(payload, 0, 0, 0, 1, 0x65, 0x88)
	} else if codec == videoCodecH265 {
		payload = append(payload, 0, 0, 0, 1, 19<<1, 1, 0x88)
	} else {
		payload = append(payload, 0, 0, 1, 0x00, 0x88)
	}
	copy(packet[6:], payload)
	return packet
}

func parameterOnlyPacket(pid uint16, codec videoCodec) []byte {
	packet := tsPacket(pid, nil)
	packet[1] |= 0x40
	payload := pesHeader(90_000)
	switch codec {
	case videoCodecH264:
		payload = append(payload,
			0, 0, 0, 1, 0x67, 0x64, 0, 0x28,
			0, 0, 0, 1, 0x68, 0xee, 0x3c, 0x80)
	case videoCodecH265:
		payload = append(payload,
			0, 0, 0, 1, 32<<1, 1,
			0, 0, 0, 1, 33<<1, 1,
			0, 0, 0, 1, 34<<1, 1)
	case videoCodecMPEG2:
		payload = append(payload, 0, 0, 1, 0xb3, 0x2d, 0x01)
	}
	copy(packet[4:], payload)
	return packet
}

func audioPESPacket(pid uint16, pts uint64) []byte {
	packet := tsPacket(pid, nil)
	packet[1] |= 0x40
	payload := pesHeader(pts)
	payload[3] = 0xc0
	payload = append(payload, 0xff, 0xf1, 0x50, 0x80)
	copy(packet[4:], payload)
	return packet
}

func audioVideoPMTSection() []byte {
	return []byte{
		0x02, 0xb0, 0x17,
		0x00, 0x01, 0xc1, 0x00, 0x00,
		0xe1, 0x01, 0xf0, 0x00,
		0x1b, 0xe1, 0x01, 0xf0, 0x00,
		0x0f, 0xe1, 0x02, 0xf0, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
}

func pesHeader(pts uint64) []byte {
	return []byte{
		0x00, 0x00, 0x01, 0xe0, 0x00, 0x00, 0x80, 0x80, 0x05,
		byte(0x21 | (pts>>29)&0x0e),
		byte(pts >> 22),
		byte(0x01 | (pts>>14)&0xfe),
		byte(pts >> 7),
		byte(0x01 | (pts<<1)&0xfe),
	}
}

func pcrPacket(pid uint16) []byte {
	packet := tsPacket(pid, nil)
	packet[3] = 0x30
	packet[4] = 7
	packet[5] = 0x10
	copy(packet[6:12], []byte{0x00, 0x12, 0x34, 0x56, 0x7e, 0x00})
	return packet
}

func TestHubDropsOnlySlowSubscriber(t *testing.T) {
	hub := NewHub(1024 * 1024)
	slow := hub.Subscribe()
	chunk := make([]byte, 300*1024)
	for range 5 {
		hub.Publish(chunk)
	}
	if hub.SubscriberCount() != 0 {
		t.Fatalf("slow subscriber remained attached")
	}
	_, drops := hub.Metrics()
	if drops != 1 {
		t.Fatalf("slow client drops = %d, want 1", drops)
	}
	if _, ok := slow.Next(); ok {
		t.Fatalf("dropped subscriber still received data")
	}
	if reason := slow.CloseReason(); reason != "slow_client_dropped" {
		t.Fatalf("slow subscriber close reason = %q, want slow_client_dropped", reason)
	}
}

func TestHubColdBurstUsesConfiguredByteBudget(t *testing.T) {
	hub := NewHub(2 * 1024 * 1024)
	subscription := hub.Subscribe()
	t.Cleanup(subscription.Close)
	chunk := make([]byte, canonicalTransportChunkBytes)

	// This burst is much larger than the old 64-chunk cold queue but remains
	// comfortably below the configured two-megabyte viewer allowance.
	for range 1000 {
		hub.Publish(chunk)
	}
	if count := hub.SubscriberCount(); count != 1 {
		t.Fatalf("cold burst evicted a healthy subscriber; subscribers = %d", count)
	}
	for range 1000 {
		if data, ok := subscription.Next(); !ok || len(data) != len(chunk) {
			t.Fatal("cold burst was not retained for the subscriber")
		}
	}
	if reason := subscription.CloseReason(); reason != "" {
		t.Fatalf("healthy subscriber unexpectedly closed: %s", reason)
	}
}

func TestHubConcurrentPublishSubscribeAndClose(t *testing.T) {
	hub := NewHub(1024 * 1024)
	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := 0; index < 100; index++ {
				subscription := hub.Subscribe()
				hub.Publish([]byte{0x47, byte(index)})
				subscription.Close()
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent hub operations deadlocked")
	}
	hub.Close(nil)
}
