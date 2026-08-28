package streaming

import (
	"context"
	"errors"
	"testing"
	"time"
	"unsafe"
)

func TestClassifyErrorProvidesActionableCodes(t *testing.T) {
	cases := map[string]string{
		"dial tcp: lookup provider.test: server misbehaving": "dns_failure",
		"connect: connection refused":                        "connection_refused",
		"upstream request timed out":                         "upstream_timeout",
		"x509 certificate is invalid":                        "tls_failure",
		"HTTP 403 forbidden":                                 "upstream_authorization",
		"missing MPEG-TS PAT":                                "invalid_transport_stream",
		"HLS playlist has no segment":                        "hls_not_ready",
		"media stalled":                                      "media_stalled",
		"stream producer cleanup timed out after 6s":         "producer_cleanup_timeout",
	}
	for message, expected := range cases {
		code, _ := ClassifyError(errors.New(message))
		if code != expected {
			t.Errorf("ClassifyError(%q) = %q, want %q", message, code, expected)
		}
	}
}

func TestSanitizeDiagnosticRemovesUpstreamCredentials(t *testing.T) {
	message := SanitizeDiagnostic("request https://viewer:secret@provider.test/live.ts?token=private failed")
	if message != "request https://provider.test/… failed" {
		t.Fatalf("unexpected sanitized diagnostic: %q", message)
	}
}

func TestSessionTracksAndTerminatesLogicalClients(t *testing.T) {
	config := testConfig(t.TempDir())
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return newReadyFake()
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	session, err := manager.Acquire(context.Background(), "observed-channel", []string{"https://example.test/live"})
	if err != nil {
		t.Fatal(err)
	}
	clientID, allowed := session.RegisterClient(ClientMetadata{ID: "viewer-1", Protocol: "hls", RemoteIP: "192.0.2.10", UserAgent: "Test Player"}, nil)
	if !allowed || clientID != "viewer-1" {
		t.Fatalf("client registration failed: id=%q allowed=%v", clientID, allowed)
	}
	if !session.AddClientBytes(clientID, 4096) {
		t.Fatal("client bytes were rejected")
	}
	session.sample(time.Now().Add(time.Second))
	snapshot := session.Snapshot()
	if snapshot.Clients != 1 || snapshot.BytesDelivered != 4096 || len(snapshot.ClientDetails) != 1 {
		t.Fatalf("unexpected client snapshot: %#v", snapshot)
	}
	if !manager.DisconnectClient("observed-channel", clientID) {
		t.Fatal("active client was not disconnected")
	}
	if _, allowed := session.RegisterClient(ClientMetadata{ID: clientID, Protocol: "hls"}, nil); allowed {
		t.Fatal("manually terminated HLS viewer was allowed to resume")
	}
}

func TestStreamSnapshotsHaveStableOperationalOrdering(t *testing.T) {
	config := testConfig(t.TempDir())
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return newReadyFake()
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	older, err := manager.Acquire(context.Background(), "older-channel", []string{"https://example.test/older"})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := manager.Acquire(context.Background(), "newer-channel", []string{"https://example.test/newer"})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Minute)
	older.mu.Lock()
	older.startedAt = base
	older.mu.Unlock()
	newer.mu.Lock()
	newer.startedAt = base.Add(time.Second)
	newer.mu.Unlock()

	if _, allowed := newer.RegisterClient(ClientMetadata{ID: "newer-viewer", Protocol: "hls"}, nil); !allowed {
		t.Fatal("newer viewer registration failed")
	}
	if _, allowed := newer.RegisterClient(ClientMetadata{ID: "older-viewer", Protocol: "mpegts"}, nil); !allowed {
		t.Fatal("older viewer registration failed")
	}
	newer.mu.Lock()
	newer.clients["newer-viewer"].startedAt = base.Add(3 * time.Second)
	newer.clients["older-viewer"].startedAt = base.Add(2 * time.Second)
	newer.mu.Unlock()

	snapshots := manager.Snapshots()
	if len(snapshots) != 2 || snapshots[0].ID != "newer-channel" || snapshots[1].ID != "older-channel" {
		t.Fatalf("active streams were not sorted newest first: %#v", snapshots)
	}
	clients := snapshots[0].ClientDetails
	if len(clients) != 2 || clients[0].ID != "older-viewer" || clients[1].ID != "newer-viewer" {
		t.Fatalf("viewer connections were not sorted oldest first: %#v", clients)
	}
}

func TestManagerCountsCredentialAndOwnerConnectionsAcrossStreams(t *testing.T) {
	config := testConfig(t.TempDir())
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return newReadyFake()
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	for index, streamID := range []string{"channel-a", "channel-b"} {
		session, err := manager.Acquire(context.Background(), streamID, []string{"https://example.test/live"})
		if err != nil {
			t.Fatal(err)
		}
		if _, allowed := session.RegisterClient(ClientMetadata{ID: streamID, Protocol: "mpegts", AuthKind: "media_key", AuthID: 9, OwnerUserID: 4}, nil); !allowed {
			t.Fatalf("viewer %d was not registered", index)
		}
	}
	credential, owner := manager.ActiveViewerCounts("media_key", 9, 4)
	if credential != 2 || owner != 2 {
		t.Fatalf("unexpected active viewer counts: credential=%d owner=%d", credential, owner)
	}
	otherCredential, sameOwner := manager.ActiveViewerCounts("media_key", 10, 4)
	if otherCredential != 0 || sameOwner != 2 {
		t.Fatalf("credential and owner budgets were not independent: credential=%d owner=%d", otherCredential, sameOwner)
	}
}

func TestManagerClonesRequestOwnedIdentifiers(t *testing.T) {
	config := testConfig(t.TempDir())
	manager := NewManager(func(id, source string, generation uint64, config Config, hub *Hub) Producer {
		return newReadyFake()
	}, func() Config { return config })
	t.Cleanup(manager.Close)
	streamBuffer := []byte("request-stream")
	streamID := unsafe.String(&streamBuffer[0], len(streamBuffer))
	session, err := manager.Acquire(context.Background(), streamID, []string{"https://example.test/live"})
	if err != nil {
		t.Fatal(err)
	}
	clientBuffer := []byte("request-viewer")
	clientID := unsafe.String(&clientBuffer[0], len(clientBuffer))
	registeredID, allowed := session.RegisterClient(ClientMetadata{ID: clientID, Protocol: "hls"}, nil)
	if !allowed {
		t.Fatal("client registration failed")
	}
	copy(streamBuffer, []byte("mutated-stream"))
	copy(clientBuffer, []byte("mutated-viewer"))
	snapshot := session.Snapshot()
	if snapshot.ID != "request-stream" || registeredID != "request-viewer" || snapshot.ClientDetails[0].ID != "request-viewer" {
		t.Fatalf("request-owned identifiers were retained: %#v", snapshot)
	}
}
