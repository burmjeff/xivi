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
	for index, expected := range [][]byte{tables, keyframe, live} {
		chunk, ok := subscription.Next()
		if !ok || !bytes.Equal(chunk, expected) {
			t.Fatalf("bootstrap chunk %d did not match expected decoder-safe replay", index)
		}
	}
}

func TestInspectTransportChunkFindsRandomAccessFlag(t *testing.T) {
	metadata := inspectTransportChunk(randomAccessPacket(0x0101))
	if !metadata.randomAccess {
		t.Fatal("transport random-access indicator was not detected")
	}
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
