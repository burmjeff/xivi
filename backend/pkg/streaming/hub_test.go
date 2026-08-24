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
