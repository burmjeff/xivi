package streaming

import (
	"errors"
	"testing"
)

func TestConnectionCoordinatorEnforcesAndReleasesPerSourceLimit(t *testing.T) {
	coordinator := newConnectionCoordinator()
	source := Source{PoolID: 7, PoolName: "provider", ConnectionLimit: 1}
	lease, err := coordinator.acquire(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.acquire(source); err == nil {
		t.Fatal("second connection exceeded the source limit")
	}
	lease.Release()
	lease.Release()
	if _, err := coordinator.acquire(source); err != nil {
		t.Fatalf("released capacity was not reusable: %v", err)
	}
}

func TestConnectionCoordinatorKeepsPlaylistPoolsIndependent(t *testing.T) {
	coordinator := newConnectionCoordinator()
	if _, err := coordinator.acquire(Source{PoolID: 1, ConnectionLimit: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.acquire(Source{PoolID: 2, ConnectionLimit: 1}); err != nil {
		t.Fatalf("one playlist consumed another playlist's connection budget: %v", err)
	}
}

func TestConnectionCoordinatorPrewarmPreservesActiveCapacity(t *testing.T) {
	coordinator := newConnectionCoordinator()
	source := Source{PoolID: 7, PoolName: "provider", ConnectionLimit: 3}
	first, err := coordinator.acquirePrewarm(source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := coordinator.acquirePrewarm(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.acquirePrewarm(source); !errors.Is(err, ErrSourceConnectionLimit) {
		t.Fatalf("third prewarm did not preserve the active slot: %v", err)
	}
	active, err := coordinator.acquire(source)
	if err != nil {
		t.Fatalf("active playback could not use the reserved slot: %v", err)
	}
	first.Release()
	second.Release()
	active.Release()
}
