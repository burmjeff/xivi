package streaming

import (
	"context"
	"testing"
	"time"
)

func newPlaybackTestManager(t *testing.T, limit int) (*Manager, Source) {
	t.Helper()
	config := testConfig(t.TempDir())
	config.RetryLimit = 1
	config.StartupHedge = 0
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return newReadyFake()
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	return manager, Source{URL: "https://provider.example/live.ts", PoolID: 40, PoolName: "Provider", ConnectionLimit: limit}
}

func TestPlaybackSwitchReleasesConstrainedSourceImmediately(t *testing.T) {
	manager, source := newPlaybackTestManager(t, 1)
	first, firstHandle, err := manager.AcquirePlaybackSources(context.Background(), "player-one", "channel-one", "hls", []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if _, allowed := first.RegisterClient(ClientMetadata{ID: firstHandle.ClientID, Protocol: "hls"}, nil); !allowed {
		t.Fatal("first playback registration failed")
	}
	second, secondHandle, err := manager.AcquirePlaybackSources(context.Background(), "player-one", "channel-two", "hls", []Source{source})
	if err != nil {
		t.Fatalf("constrained channel switch failed: %v", err)
	}
	if firstHandle.ClientID == secondHandle.ClientID {
		t.Fatal("channel switch reused a stale tune generation")
	}
	if second == nil || second.id != "channel-two" {
		t.Fatalf("unexpected target session: %#v", second)
	}
	if _, ok := manager.ResolvePlayback("player-one", "channel-one"); ok {
		t.Fatal("old channel still resolved after switch")
	}
	if _, ok := manager.ResolvePlayback("player-one", "channel-two"); !ok {
		t.Fatal("new channel did not resolve after switch")
	}
}

func TestPlaybackSwitchNeverEvictsAnotherSharedViewer(t *testing.T) {
	manager, source := newPlaybackTestManager(t, 1)
	first, handle, err := manager.AcquirePlaybackSources(context.Background(), "player-one", "shared-channel", "hls", []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if _, allowed := first.RegisterClient(ClientMetadata{ID: handle.ClientID, Protocol: "hls"}, nil); !allowed {
		t.Fatal("tracked viewer registration failed")
	}
	if _, allowed := first.RegisterClient(ClientMetadata{ID: "other-viewer", Protocol: "mpegts"}, nil); !allowed {
		t.Fatal("shared viewer registration failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, _, err := manager.AcquirePlaybackSources(ctx, "player-one", "blocked-channel", "hls", []Source{source}); err == nil {
		t.Fatal("switch unexpectedly evicted a shared upstream")
	}
	if first.ActiveClientCount() != 2 {
		t.Fatalf("shared session viewers changed after refused switch: %d", first.ActiveClientCount())
	}
	if _, ok := manager.ResolvePlayback("player-one", "shared-channel"); !ok {
		t.Fatal("working playback mapping was lost after refused switch")
	}
}

func TestMPEGTSReconnectReplacesOnlyItsTransportConnection(t *testing.T) {
	manager, source := newPlaybackTestManager(t, 1)
	session, firstHandle, err := manager.AcquirePlaybackSources(context.Background(), "plex-session", "channel-one", "mpegts", []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	firstCancelled := false
	if _, allowed := session.RegisterClient(ClientMetadata{ID: firstHandle.ClientID, Protocol: "mpegts"}, func() { firstCancelled = true }); !allowed {
		t.Fatal("first MPEG-TS connection registration failed")
	}
	reused, secondHandle, err := manager.AcquirePlaybackSources(context.Background(), "plex-session", "channel-one", "mpegts", []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if reused != session {
		t.Fatal("same-channel reconnect created another upstream session")
	}
	if firstHandle.ClientID == secondHandle.ClientID || !firstCancelled {
		t.Fatal("old MPEG-TS transport connection was not replaced")
	}
	if _, allowed := reused.RegisterClient(ClientMetadata{ID: secondHandle.ClientID, Protocol: "mpegts"}, nil); !allowed {
		t.Fatal("replacement MPEG-TS connection registration failed")
	}
	if reused.ActiveClientCount() != 1 {
		t.Fatalf("unexpected active connection count: %d", reused.ActiveClientCount())
	}
}

func TestExplicitPlaybackReleasePreservesOtherViewers(t *testing.T) {
	manager, source := newPlaybackTestManager(t, 1)
	session, handle, err := manager.AcquirePlaybackSources(context.Background(), "player-one", "shared-channel", "hls", []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	session.RegisterClient(ClientMetadata{ID: handle.ClientID, Protocol: "hls"}, nil)
	session.RegisterClient(ClientMetadata{ID: "other-viewer", Protocol: "mpegts"}, nil)
	if !manager.ReleasePlayback(context.Background(), "player-one", "shared-channel") {
		t.Fatal("playback release was not accepted")
	}
	if session.ActiveClientCount() != 1 {
		t.Fatalf("release affected another viewer: %d active", session.ActiveClientCount())
	}
	if snapshot := session.Snapshot(); snapshot.State == StateStopping || snapshot.State == StateStopped {
		t.Fatalf("shared session stopped after one viewer left: %s", snapshot.State)
	}
}

func TestHLSFallbackViewerUsesSegmentAwareTimeout(t *testing.T) {
	manager, source := newPlaybackTestManager(t, 1)
	session, err := manager.AcquireSources(context.Background(), "hls-timeout", []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if _, allowed := session.RegisterClient(ClientMetadata{ID: "legacy-hls", Protocol: "hls"}, nil); !allowed {
		t.Fatal("HLS viewer registration failed")
	}
	now := time.Now()
	session.mu.Lock()
	session.clients["legacy-hls"].lastSeenAt = now.Add(-minimumHLSClientTimeout - time.Second)
	session.mu.Unlock()
	session.sample(now)
	if session.ActiveClientCount() != 0 {
		t.Fatal("stale HLS viewer remained active past its segment-aware timeout")
	}
}
