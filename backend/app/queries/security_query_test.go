package queries

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func securityTestQueries(t *testing.T) *SecurityQueries {
	t.Helper()
	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_txlock=immediate")
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE template (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	db.MustExec(`CREATE TABLE app_user (
		id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE,
		display_name TEXT NOT NULL DEFAULT '',
		password_hash TEXT NOT NULL, role TEXT NOT NULL, must_change_password BOOLEAN NOT NULL,
		initial_password BOOLEAN NOT NULL DEFAULT FALSE,
		auth_version INTEGER NOT NULL DEFAULT 1, disabled_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	db.MustExec(`CREATE TABLE user_lineup (
		user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
		lineup_id INTEGER NOT NULL REFERENCES template(id) ON DELETE CASCADE,
		PRIMARY KEY(user_id, lineup_id))`)
	db.MustExec(`CREATE TABLE user_mfa (user_id INTEGER PRIMARY KEY REFERENCES app_user(id))`)
	db.MustExec(`CREATE TABLE user_mfa_recovery_code (
		id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES app_user(id),
		used_at TIMESTAMP NULL)`)
	db.MustExec(`CREATE TABLE auth_session (
		id INTEGER PRIMARY KEY, absolute_expires_at TIMESTAMP NOT NULL,
		revoked_at TIMESTAMP NULL)`)
	db.MustExec(`CREATE TABLE security_audit_event (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		actor_user_id INTEGER NULL REFERENCES app_user(id) ON DELETE SET NULL,
		actor_username TEXT NOT NULL DEFAULT '',
		actor_display_name TEXT NOT NULL DEFAULT '',
		target_user_id INTEGER NULL REFERENCES app_user(id) ON DELETE SET NULL,
		target_username TEXT NOT NULL DEFAULT '',
		target_display_name TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL, outcome TEXT NOT NULL, resource_type TEXT NOT NULL DEFAULT '',
		resource_id TEXT NOT NULL DEFAULT '', client_ip TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	db.MustExec(`CREATE TABLE media_access_key (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
		name TEXT NOT NULL, token_prefix TEXT NOT NULL UNIQUE, token_hash BLOB NOT NULL UNIQUE,
		token_cipher BLOB NULL, network_scope TEXT NOT NULL, created_at TIMESTAMP NOT NULL,
		expires_at TIMESTAMP NULL, last_used_at TIMESTAMP NULL, last_used_ip TEXT NOT NULL DEFAULT '',
		revoked_at TIMESTAMP NULL)`)
	db.MustExec(`CREATE TABLE media_key_lineup (
		media_key_id INTEGER NOT NULL REFERENCES media_access_key(id) ON DELETE CASCADE,
		lineup_id INTEGER NOT NULL REFERENCES template(id) ON DELETE CASCADE,
		PRIMARY KEY(media_key_id, lineup_id))`)
	return NewSecurityQueries(db)
}

func TestSecurityAuditPreservesActorAndTargetUsernames(t *testing.T) {
	ctx := context.Background()
	q := securityTestQueries(t)
	actorID, err := q.CreateUser(ctx, "alice-admin", "Alice Admin", "hash", "admin", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	targetID, err := q.CreateUser(ctx, "bob-viewer", "Bob Viewer", "hash", "viewer", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Audit(ctx, models.SecurityAuditEvent{
		ActorUserID: &actorID, ActorUsername: "alice-admin", ActorDisplayName: "Alice Admin",
		TargetUserID: &targetID, TargetUsername: "bob-viewer", TargetDisplayName: "Bob Viewer",
		Action: "user_update", Outcome: "success", ResourceType: "user",
		ResourceID: "2",
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateUserIdentity(ctx, actorID, "renamed-admin", "Renamed Admin"); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateUserIdentity(ctx, targetID, "renamed-viewer", "Renamed Viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM app_user WHERE id = ?`, targetID); err != nil {
		t.Fatal(err)
	}
	events, total, err := q.ListSecurityAuditEvents(ctx, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(events) != 1 {
		t.Fatalf("unexpected audit result total=%d events=%d", total, len(events))
	}
	if events[0].ActorUsername != "alice-admin" || events[0].TargetUsername != "bob-viewer" {
		t.Fatalf("audit usernames were not preserved: %+v", events[0])
	}
	if events[0].ActorDisplayName != "Alice Admin" || events[0].TargetDisplayName != "Bob Viewer" {
		t.Fatalf("audit display names were not preserved: %+v", events[0])
	}
	if events[0].TargetUserID != nil {
		t.Fatalf("deleted target foreign key was not cleared: %+v", events[0].TargetUserID)
	}
}

func TestMediaKeyRetainsEncryptedCredentialForReusableLinks(t *testing.T) {
	ctx := context.Background()
	q := securityTestQueries(t)
	userID, err := q.CreateUser(ctx, "device-owner", "", "hash", "viewer", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	q.MustExec(`INSERT INTO template(id, name) VALUES (7, 'Home')`)
	key := &models.MediaAccessKey{
		UserID: userID, Name: "Living room", TokenPrefix: "xmk_example",
		TokenHash: []byte("hash"), TokenCipher: []byte("encrypted-credential"),
		NetworkScope: "lan", CreatedAt: time.Now().UTC(),
	}
	if err := q.CreateMediaKey(ctx, key, []int64{7}); err != nil {
		t.Fatal(err)
	}
	keys, err := q.ListMediaKeys(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || !bytes.Equal(keys[0].TokenCipher, key.TokenCipher) {
		t.Fatalf("encrypted device credential was not retained: %+v", keys)
	}
	if len(keys[0].LineupIDs) != 1 || keys[0].LineupIDs[0] != 7 {
		t.Fatalf("device lineup access was not retained: %+v", keys[0].LineupIDs)
	}
}

func TestListMediaKeysGroupsActiveKeysBeforeRevokedKeys(t *testing.T) {
	ctx := context.Background()
	q := securityTestQueries(t)
	userID, err := q.CreateUser(ctx, "ordered-device-owner", "", "hash", "viewer", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-time.Hour)
	items := []struct {
		name      string
		createdAt time.Time
		revoke    bool
	}{
		{name: "active-older", createdAt: base},
		{name: "revoked-newest", createdAt: base.Add(2 * time.Minute), revoke: true},
		{name: "active-newer", createdAt: base.Add(time.Minute)},
	}
	for _, item := range items {
		key := &models.MediaAccessKey{
			UserID: userID, Name: item.name, TokenPrefix: "prefix-" + item.name,
			TokenHash: []byte("hash-" + item.name), NetworkScope: "lan", CreatedAt: item.createdAt,
		}
		if err := q.CreateMediaKey(ctx, key, nil); err != nil {
			t.Fatal(err)
		}
		if item.revoke {
			if revoked, err := q.RevokeMediaKey(ctx, key.ID, userID); err != nil || !revoked {
				t.Fatalf("revoke media key: revoked=%v err=%v", revoked, err)
			}
		}
	}

	keys, err := q.ListMediaKeys(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"active-newer", "active-older", "revoked-newest"}
	if len(keys) != len(want) {
		t.Fatalf("unexpected key count: %d", len(keys))
	}
	for index, name := range want {
		if keys[index].Name != name {
			t.Fatalf("device access order[%d]=%q, want %q", index, keys[index].Name, name)
		}
	}
}

func TestSecurityRetentionDeletesOldDeviceKeysAndLineupGrants(t *testing.T) {
	ctx := context.Background()
	q := securityTestQueries(t)
	userID, err := q.CreateUser(ctx, "retention-owner", "", "hash", "viewer", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	q.MustExec(`INSERT INTO template(id, name) VALUES (7, 'Home')`)
	now := time.Now().UTC().Truncate(time.Second)
	cutoff := now.AddDate(0, 0, -90)
	for _, item := range []struct {
		id        int
		name      string
		revokedAt *time.Time
		expiresAt *time.Time
	}{
		{id: 1, name: "old-revoked", revokedAt: pointerTime(cutoff.Add(-time.Hour))},
		{id: 2, name: "recent-revoked", revokedAt: pointerTime(cutoff.Add(time.Hour))},
		{id: 3, name: "old-expired", expiresAt: pointerTime(cutoff.Add(-time.Hour))},
		{id: 4, name: "active"},
	} {
		q.MustExec(`INSERT INTO media_access_key
			(id, user_id, name, token_prefix, token_hash, token_cipher, network_scope,
			 created_at, expires_at, revoked_at)
			VALUES (?, ?, ?, ?, ?, ?, 'lan', ?, ?, ?)`, item.id, userID, item.name,
			"prefix-"+item.name, []byte("hash-"+item.name), []byte("cipher-"+item.name),
			now.AddDate(-1, 0, 0), item.expiresAt, item.revokedAt)
		q.MustExec(`INSERT INTO media_key_lineup(media_key_id, lineup_id) VALUES (?, 7)`, item.id)
	}
	if err := q.PruneSecurityData(ctx, now, cutoff, 1000); err != nil {
		t.Fatal(err)
	}
	var remaining []int
	if err := q.SelectContext(ctx, &remaining, `SELECT id FROM media_access_key ORDER BY id`); err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 || remaining[0] != 2 || remaining[1] != 4 {
		t.Fatalf("unexpected retained device keys: %v", remaining)
	}
	var removedGrants int
	if err := q.GetContext(ctx, &removedGrants, `SELECT COUNT(*) FROM media_key_lineup WHERE media_key_id IN (1, 3)`); err != nil || removedGrants != 0 {
		t.Fatalf("deleted device-key grants remained: count=%d err=%v", removedGrants, err)
	}
}

func pointerTime(value time.Time) *time.Time { return &value }

func streamAuthorizationTestQueries(t *testing.T) *SecurityQueries {
	t.Helper()
	db := sqlx.MustOpen("sqlite3", ":memory:?_foreign_keys=on&_txlock=immediate")
	t.Cleanup(func() { _ = db.Close() })
	db.MustExec(`CREATE TABLE app_user (
		id INTEGER PRIMARY KEY, role TEXT NOT NULL, auth_version INTEGER NOT NULL,
		disabled_at TIMESTAMP NULL)`)
	db.MustExec(`CREATE TABLE user_mfa (user_id INTEGER PRIMARY KEY)`)
	db.MustExec(`CREATE TABLE auth_session (
		id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, auth_version INTEGER NOT NULL,
		mfa_verified BOOLEAN NOT NULL, revoked_at TIMESTAMP NULL,
		idle_expires_at TIMESTAMP NOT NULL, absolute_expires_at TIMESTAMP NOT NULL)`)
	db.MustExec(`CREATE TABLE template (id INTEGER PRIMARY KEY)`)
	db.MustExec(`CREATE TABLE user_lineup (user_id INTEGER NOT NULL, lineup_id INTEGER NOT NULL)`)
	db.MustExec(`CREATE TABLE templategroup (id INTEGER PRIMARY KEY)`)
	db.MustExec(`CREATE TABLE template_group_item (template_id INTEGER NOT NULL, group_id INTEGER NOT NULL)`)
	db.MustExec(`CREATE TABLE templatechannel (id INTEGER PRIMARY KEY, uuid TEXT NOT NULL, logoid INTEGER NULL)`)
	db.MustExec(`CREATE TABLE template_group_channel (group_id INTEGER NOT NULL, channel_id INTEGER NOT NULL)`)
	db.MustExec(`CREATE TABLE logo (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`)
	db.MustExec(`CREATE TABLE media_access_key (
		id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, revoked_at TIMESTAMP NULL,
		expires_at TIMESTAMP NULL)`)
	db.MustExec(`CREATE TABLE media_key_lineup (media_key_id INTEGER NOT NULL, lineup_id INTEGER NOT NULL)`)
	return NewSecurityQueries(db)
}

func TestFinalEnabledAdministratorIsTransactionalInvariant(t *testing.T) {
	ctx := context.Background()
	q := securityTestQueries(t)
	first, err := q.CreateUser(ctx, "first-admin", "", "hash", "admin", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateUser(ctx, first, "viewer", false, nil); !errors.Is(err, ErrLastEnabledAdmin) {
		t.Fatalf("final admin demotion returned %v", err)
	}
	if err := q.DeleteUser(ctx, first); !errors.Is(err, ErrLastEnabledAdmin) {
		t.Fatalf("final admin deletion returned %v", err)
	}
	second, err := q.CreateUser(ctx, "second-admin", "", "hash", "admin", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateUser(ctx, first, "viewer", false, nil); err != nil {
		t.Fatalf("demotion with replacement admin failed: %v", err)
	}
	if err := q.DeleteUser(ctx, second); !errors.Is(err, ErrLastEnabledAdmin) {
		t.Fatalf("remaining admin deletion returned %v", err)
	}
}

func TestUserIdentityUsesImmutableOwnershipAndRotatesOnlyForUsername(t *testing.T) {
	ctx := context.Background()
	q := securityTestQueries(t)
	userID, err := q.CreateUser(ctx, "profile-owner", "Original Name", "hash", "viewer", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := &models.MediaAccessKey{
		UserID: userID, Name: "Television", TokenPrefix: "profile-key",
		TokenHash: []byte("profile-key-hash"), NetworkScope: "lan", CreatedAt: time.Now().UTC(),
	}
	if err := q.CreateMediaKey(ctx, key, nil); err != nil {
		t.Fatal(err)
	}

	if err := q.UpdateUserIdentity(ctx, userID, "profile-owner", "Updated Name"); err != nil {
		t.Fatal(err)
	}
	user, err := q.GetUserByID(ctx, userID)
	if err != nil || user.DisplayName != "Updated Name" || user.AuthVersion != 1 {
		t.Fatalf("display-name update unexpectedly rotated authentication: user=%+v err=%v", user, err)
	}

	if err := q.UpdateUserIdentity(ctx, userID, "renamed-owner", "Updated Name"); err != nil {
		t.Fatal(err)
	}
	user, err = q.GetUserByID(ctx, userID)
	if err != nil || user.Username != "renamed-owner" || user.AuthVersion != 2 {
		t.Fatalf("username update did not rotate authentication: user=%+v err=%v", user, err)
	}
	keys, err := q.ListMediaKeys(ctx, userID)
	if err != nil || len(keys) != 1 || keys[0].ID != key.ID || keys[0].RevokedAt != nil {
		t.Fatalf("identity update changed immutable device-key ownership: keys=%+v err=%v", keys, err)
	}

	otherID, err := q.CreateUser(ctx, "other-owner", "", "hash", "viewer", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateUserIdentity(ctx, otherID, "renamed-owner", ""); !errors.Is(err, ErrUsernameInUse) {
		t.Fatalf("duplicate username update returned %v", err)
	}
}

func TestInitialAdministratorIsCreatedOnceAndRequiresPasswordChange(t *testing.T) {
	ctx := context.Background()
	q := securityTestQueries(t)

	created, id, err := q.CreateInitialAdminIfEmpty(ctx, "xivi", "temporary-hash")
	if err != nil || !created || id < 1 {
		t.Fatalf("first initialization returned created=%v id=%d err=%v", created, id, err)
	}
	user, err := q.GetUserByUsername(ctx, "xivi")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != "admin" || !user.MustChangePassword || !user.InitialPassword || user.PasswordHash != "temporary-hash" {
		t.Fatalf("unexpected initial administrator: %+v", user)
	}
	required, err := q.InitialAdminPasswordChangeRequired(ctx, "xivi")
	if err != nil || !required {
		t.Fatalf("initial password-change state returned %v, %v", required, err)
	}

	created, secondID, err := q.CreateInitialAdminIfEmpty(ctx, "xivi", "different-hash")
	if err != nil || created || secondID != 0 {
		t.Fatalf("repeat initialization returned created=%v id=%d err=%v", created, secondID, err)
	}
	count, err := q.UserCount(ctx)
	if err != nil || count != 1 {
		t.Fatalf("repeat initialization left %d users, err=%v", count, err)
	}
	if err := q.SetUserPassword(ctx, id, "changed-hash", false); err != nil {
		t.Fatal(err)
	}
	required, err = q.InitialAdminPasswordChangeRequired(ctx, "xivi")
	if err != nil || required {
		t.Fatalf("completed password change returned required=%v err=%v", required, err)
	}
	user, err = q.GetUserByUsername(ctx, "xivi")
	if err != nil || user.InitialPassword {
		t.Fatalf("initial password marker was not permanently cleared: user=%+v err=%v", user, err)
	}
}

func TestSessionChannelAuthorizationTracksRevocationAndCurrentGrant(t *testing.T) {
	ctx := context.Background()
	q := streamAuthorizationTestQueries(t)
	now := time.Now().UTC()
	q.MustExec(`INSERT INTO app_user(id, role, auth_version) VALUES (1, 'viewer', 1)`)
	q.MustExec(`INSERT INTO template(id) VALUES (10)`)
	q.MustExec(`INSERT INTO templategroup(id) VALUES (20)`)
	q.MustExec(`INSERT INTO template_group_item(template_id, group_id) VALUES (10, 20)`)
	q.MustExec(`INSERT INTO templatechannel(id, uuid) VALUES (30, 'channel-one')`)
	q.MustExec(`INSERT INTO template_group_channel(group_id, channel_id) VALUES (20, 30)`)
	q.MustExec(`INSERT INTO template(id) VALUES (11)`)
	q.MustExec(`INSERT INTO templategroup(id) VALUES (21)`)
	q.MustExec(`INSERT INTO template_group_item(template_id, group_id) VALUES (11, 21)`)
	q.MustExec(`INSERT INTO templatechannel(id, uuid) VALUES (31, 'channel-two')`)
	q.MustExec(`INSERT INTO template_group_channel(group_id, channel_id) VALUES (21, 31)`)
	q.MustExec(`INSERT INTO user_lineup(user_id, lineup_id) VALUES (1, 10)`)
	q.MustExec(`INSERT INTO auth_session(id, user_id, auth_version, mfa_verified, idle_expires_at, absolute_expires_at)
		VALUES (40, 1, 1, 1, ?, ?)`, now.Add(time.Hour), now.Add(2*time.Hour))

	allowed, err := q.SessionCanAccessChannel(ctx, 40, "channel-one")
	if err != nil || !allowed {
		t.Fatalf("active granted session returned %v, %v", allowed, err)
	}
	allowed, err = q.SessionCanAccessChannel(ctx, 40, "channel-two")
	if err != nil || allowed {
		t.Fatalf("another viewer lineup returned %v, %v", allowed, err)
	}
	q.MustExec(`DELETE FROM user_lineup WHERE user_id = 1`)
	allowed, err = q.SessionCanAccessChannel(ctx, 40, "channel-one")
	if err != nil || allowed {
		t.Fatalf("removed lineup grant returned %v, %v", allowed, err)
	}
	q.MustExec(`INSERT INTO user_lineup(user_id, lineup_id) VALUES (1, 10)`)
	q.MustExec(`UPDATE auth_session SET revoked_at = CURRENT_TIMESTAMP WHERE id = 40`)
	allowed, err = q.SessionCanAccessChannel(ctx, 40, "channel-one")
	if err != nil || allowed {
		t.Fatalf("revoked session returned %v, %v", allowed, err)
	}
}

func TestMediaKeyAuthorizationNeverOutlivesCurrentViewerGrant(t *testing.T) {
	ctx := context.Background()
	q := streamAuthorizationTestQueries(t)
	q.MustExec(`INSERT INTO app_user(id, role, auth_version) VALUES (1, 'viewer', 1)`)
	q.MustExec(`INSERT INTO template(id) VALUES (10), (11)`)
	q.MustExec(`INSERT INTO templategroup(id) VALUES (20), (21)`)
	q.MustExec(`INSERT INTO template_group_item(template_id, group_id) VALUES (10, 20), (11, 21)`)
	q.MustExec(`INSERT INTO templatechannel(id, uuid) VALUES (30, 'channel-one'), (31, 'channel-two')`)
	q.MustExec(`INSERT INTO template_group_channel(group_id, channel_id) VALUES (20, 30), (21, 31)`)
	q.MustExec(`INSERT INTO user_lineup(user_id, lineup_id) VALUES (1, 10)`)
	q.MustExec(`INSERT INTO media_access_key(id, user_id) VALUES (50, 1)`)
	// The stale second row simulates a previously valid key whose account grant
	// was later narrowed. Authorization must still consult the current grant.
	q.MustExec(`INSERT INTO media_key_lineup(media_key_id, lineup_id) VALUES (50, 10), (50, 11)`)

	allowed, err := q.MediaKeyAllowsChannel(ctx, 50, "channel-one")
	if err != nil || !allowed {
		t.Fatalf("currently granted media channel returned %v, %v", allowed, err)
	}
	allowed, err = q.MediaKeyAllowsChannel(ctx, 50, "channel-two")
	if err != nil || allowed {
		t.Fatalf("stale media-key lineup grant returned %v, %v", allowed, err)
	}
	q.MustExec(`DELETE FROM user_lineup WHERE user_id = 1`)
	allowed, err = q.MediaKeyAllowsChannel(ctx, 50, "channel-one")
	if err != nil || allowed {
		t.Fatalf("removed viewer grant returned %v, %v", allowed, err)
	}
}

func TestLineupLogoAuthorizationDoesNotLeakOtherAssets(t *testing.T) {
	ctx := context.Background()
	q := streamAuthorizationTestQueries(t)
	q.MustExec(`INSERT INTO template(id) VALUES (1), (2)`)
	q.MustExec(`INSERT INTO templategroup(id) VALUES (11), (22)`)
	q.MustExec(`INSERT INTO template_group_item(template_id, group_id) VALUES (1, 11), (2, 22)`)
	q.MustExec(`INSERT INTO logo(id, name) VALUES (101, 'allowed'), (202, 'other')`)
	q.MustExec(`INSERT INTO templatechannel(id, uuid, logoid) VALUES (111, 'one', 101), (222, 'two', 202)`)
	q.MustExec(`INSERT INTO template_group_channel(group_id, channel_id) VALUES (11, 111), (22, 222)`)

	allowed, err := q.LineupContainsLogo(ctx, 1, "allowed.png")
	if err != nil || !allowed {
		t.Fatalf("lineup logo returned %v, %v", allowed, err)
	}
	allowed, err = q.LineupContainsLogo(ctx, 1, "other.png")
	if err != nil || allowed {
		t.Fatalf("unrelated logo returned %v, %v", allowed, err)
	}
}
