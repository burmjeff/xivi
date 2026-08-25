package streaming

import "testing"

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
