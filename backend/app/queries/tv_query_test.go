package queries

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
	"xivi/backend/app/models"
)

func pairedTV(t *testing.T) (*SecurityQueries, *models.TVDevice, time.Time) {
	t.Helper()
	q := securityTestQueries(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	id, err := q.CreateUser(ctx, "tv-viewer", "Viewer", "unused-test-hash", models.RoleViewer, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := &models.TVPairing{DeviceCodeHash: []byte("device-code"), UserCodeHash: []byte("user-code"), DeviceName: "Living room", KeyThumbprint: "test-key", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
	if err = q.CreateTVPairing(ctx, p); err != nil {
		t.Fatal(err)
	}
	_, _, err = q.ExchangeTVPairing(ctx, p.DeviceCodeHash, p.KeyThumbprint, []byte("unused"), []byte("unused"), "127.0.0.1", now)
	if !errors.Is(err, ErrTVPending) {
		t.Fatalf("pending: %v", err)
	}
	changed, err := q.DecideTVPairing(ctx, p.UserCodeHash, id, 1, true, now)
	if err != nil || !changed {
		t.Fatalf("approve: %v %v", changed, err)
	}
	_, _, err = q.ExchangeTVPairing(ctx, p.DeviceCodeHash, p.KeyThumbprint, []byte("refresh"), []byte("cipher"), "127.0.0.1", now.Add(time.Second))
	if !errors.Is(err, ErrTVSlowDown) {
		t.Fatalf("poll limit: %v", err)
	}
	d, cipher, err := q.ExchangeTVPairing(ctx, p.DeviceCodeHash, p.KeyThumbprint, []byte("refresh"), []byte("cipher"), "127.0.0.1", now.Add(5*time.Second))
	if err != nil || !bytes.Equal(cipher, []byte("cipher")) {
		t.Fatalf("exchange: %v", err)
	}
	return q, d, now
}

func TestTVPairingDeliveryIsRetrySafeAndKeyBound(t *testing.T) {
	q, first, now := pairedTV(t)
	ctx := context.Background()
	retry, cipher, err := q.ExchangeTVPairing(ctx, []byte("device-code"), "test-key", []byte("must-not-replace"), []byte("must-not-replace"), "127.0.0.1", now.Add(10*time.Second))
	if err != nil || retry.ID != first.ID || !bytes.Equal(cipher, []byte("cipher")) {
		t.Fatalf("lost-response retry changed enrollment: %v", err)
	}
	var count int
	if err = q.GetContext(ctx, &count, `SELECT COUNT(*) FROM tv_device`); err != nil || count != 1 {
		t.Fatalf("duplicate device: %d %v", count, err)
	}
	_, _, err = q.ExchangeTVPairing(ctx, []byte("device-code"), "other-key", nil, nil, "", now.Add(20*time.Second))
	if !errors.Is(err, ErrTVGrant) {
		t.Fatalf("wrong key accepted: %v", err)
	}
	_, _, err = q.ExchangeTVPairing(ctx, []byte("device-code"), "test-key", nil, nil, "", now.Add(11*time.Minute))
	if !errors.Is(err, ErrTVGrant) {
		t.Fatalf("expired pairing accepted: %v", err)
	}
}

func TestTVRefreshDoesNotExpireOrRotateAndAccessExpires(t *testing.T) {
	q, d, now := pairedTV(t)
	ctx := context.Background()
	future := now.AddDate(20, 0, 0)
	if err := q.IssueTVAccess(ctx, d.ID, []byte("first-access"), now); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetTVDeviceByAccess(ctx, []byte("first-access"), now.Add(14*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetTVDeviceByAccess(ctx, []byte("first-access"), now.Add(15*time.Minute)); err == nil {
		t.Fatal("expired access accepted")
	}
	if err := q.IssueTVAccess(ctx, d.ID, []byte("future-access"), future); err != nil {
		t.Fatalf("inactivity expired TV: %v", err)
	}
	saved, err := q.GetTVDeviceByRefresh(ctx, []byte("refresh"))
	if err != nil || saved.ID != d.ID {
		t.Fatalf("durable credential rotated: %v", err)
	}
	if err = q.RevokeTVDevice(ctx, d.ID, d.UserID); err != nil {
		t.Fatal(err)
	}
	if err = q.IssueTVAccess(ctx, d.ID, []byte("revoked-access"), future); !errors.Is(err, ErrTVGrant) {
		t.Fatalf("revoked device renewed: %v", err)
	}
}

func TestTVSecurityChangesInvalidatePairing(t *testing.T) {
	for _, change := range []string{"auth_version=auth_version+1", "disabled_at=CURRENT_TIMESTAMP", "must_change_password=TRUE"} {
		t.Run(change, func(t *testing.T) {
			q, d, now := pairedTV(t)
			ctx := context.Background()
			if _, err := q.ExecContext(ctx, `UPDATE app_user SET `+change+` WHERE id=?`, d.UserID); err != nil {
				t.Fatal(err)
			}
			active, err := q.TVDeviceActive(ctx, d.ID)
			if err != nil || active {
				t.Fatalf("security change left TV active: %v", err)
			}
			if err = q.IssueTVAccess(ctx, d.ID, []byte("invalidated"), now); !errors.Is(err, ErrTVGrant) {
				t.Fatalf("invalidated device renewed: %v", err)
			}
		})
	}
}

func TestTVProofReplayEvidenceAndAllDeviceRevocation(t *testing.T) {
	q, d, now := pairedTV(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		fresh, err := q.ConsumeTVProof(ctx, "test-key", "unique-request", now)
		if err != nil || fresh != (i == 0) {
			t.Fatalf("proof replay %d: %v %v", i, fresh, err)
		}
	}
	if err := q.RevokeUserMobileSessions(ctx, d.UserID); err != nil {
		t.Fatal(err)
	}
	active, err := q.TVDeviceActive(ctx, d.ID)
	if err != nil || active {
		t.Fatalf("all-device revocation missed TV: %v", err)
	}
}

func TestViewerPreferencesRevisionConflictDoesNotOverwrite(t *testing.T) {
	q, d, _ := pairedTV(t)
	ctx := context.Background()
	p := models.EmptyViewerPreferences()
	p.Favorites = []models.FavoriteList{{ID: "sports", Name: "Sports", Channels: []string{"1:1"}}}
	saved, err := q.SaveViewerPreferences(ctx, d.UserID, p)
	if err != nil || saved.Revision != 1 {
		t.Fatalf("first save: %v", err)
	}
	p.Hidden = []string{"1:2"}
	if _, err = q.SaveViewerPreferences(ctx, d.UserID, p); !errors.Is(err, ErrPreferenceConflict) {
		t.Fatalf("stale write accepted: %v", err)
	}
	current, err := q.GetViewerPreferences(ctx, d.UserID)
	if err != nil || current.Revision != 1 || len(current.Hidden) != 0 || len(current.Favorites) != 1 {
		t.Fatalf("conflict overwrote document: %+v %v", current, err)
	}
}
