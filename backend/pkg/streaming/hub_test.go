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
	hub.Publish(keyframe)
	hub.Publish(live)
	if !hub.HasDecoderBootstrap(true) {
		t.Fatal("video bootstrap did not recognize programme tables and keyframe")
	}

	subscription := hub.Subscribe()
	t.Cleanup(subscription.Close)
	programmeTables := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	for index, expected := range [][]byte{programmeTables, keyframe, live} {
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
	keyframe := randomAccessPacket(0x0101)
	afterKeyframe := tsPacket(0x0102, nil)
	combined := append([]byte{}, pat...)
	combined = append(combined, pmt...)
	combined = append(combined, delta...)
	combined = append(combined, keyframe...)
	combined = append(combined, afterKeyframe...)

	hub.Publish(combined)
	subscription := hub.Subscribe()
	t.Cleanup(subscription.Close)

	prefix, ok := subscription.Next()
	if !ok || !bytes.Equal(prefix, append(append([]byte{}, pat...), pmt...)) {
		t.Fatal("video bootstrap did not emit a compact PAT/PMT prefix")
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

func TestHubIgnoresRandomAccessOutsideVideoPID(t *testing.T) {
	hub := NewHub(1024 * 1024)
	tables := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	tables = append(tables, tsPacket(0x1fff, nil)...)
	hub.Publish(tables)
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
	keyframe := randomAccessPacket(0x0101)
	delta := transportPackets(0x0101, 7)

	hub.Publish(tables)
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
	hub.Publish(keyframe)

	programmeTables := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	for index, expected := range [][]byte{programmeTables, keyframe} {
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
	keyframe := randomAccessPacket(0x0101)
	delta := transportPackets(0x0101, 7)
	hub.Publish(tables)
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
	hub.Publish(keyframe)
	programmeTables := append(tsPacket(0, patSection()), tsPacket(0x0100, pmtSection())...)
	if chunk, ok := subscription.Next(); !ok || !bytes.Equal(chunk, programmeTables) {
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
	packet := tsPacket(pid, nil)
	packet[3] = 0x30
	packet[4] = 1
	packet[5] = 0x40
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
