package queries

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"time"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
)

type SecurityQueries struct{ BaseQueries }

var (
	ErrLastEnabledAdmin    = errors.New("the final enabled administrator cannot be changed")
	ErrUsernameInUse       = errors.New("the username is already in use")
	ErrMobileRefreshReplay = errors.New("the mobile refresh token was already used")
	authThrottleWrites     atomic.Uint64
)

func NewSecurityQueries(db *sqlx.DB) *SecurityQueries {
	return &SecurityQueries{BaseQueries: NewBaseQueries(db)}
}

const userSelect = `
	SELECT u.id, u.username, u.display_name, u.password_hash, u.role, u.must_change_password, u.initial_password,
	       u.auth_version, u.disabled_at, u.created_at, u.updated_at,
	       EXISTS(SELECT 1 FROM user_mfa m WHERE m.user_id = u.id) AS mfa_enabled
	FROM app_user u`

func (q *SecurityQueries) UserCount(ctx context.Context) (int64, error) {
	var count int64
	err := q.GetContext(ctx, &count, `SELECT COUNT(*) FROM app_user`)
	return count, err
}

// CreateInitialAdminIfEmpty atomically creates the first administrator. The
// surrounding database connection uses an immediate SQLite transaction, so
// concurrent startup attempts cannot both observe an empty user table and
// create competing bootstrap accounts.
func (q *SecurityQueries) CreateInitialAdminIfEmpty(ctx context.Context, username, passwordHash string) (bool, int64, error) {
	var created bool
	var id int64
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var count int64
		if err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM app_user`); err != nil {
			return err
		}
		if count != 0 {
			return nil
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO app_user
			(username, password_hash, role, must_change_password, initial_password, created_at, updated_at)
			VALUES (?, ?, 'admin', TRUE, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, username, passwordHash)
		if err != nil {
			return err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, id, err
}

// InitialAdminPasswordChangeRequired identifies only the untouched automatic
// first-run credential. Later password resets also require a password change,
// but they never reactivate this marker or advertise the known default.
func (q *SecurityQueries) InitialAdminPasswordChangeRequired(ctx context.Context, username string) (bool, error) {
	var required bool
	err := q.GetContext(ctx, &required, `SELECT EXISTS(
		SELECT 1 FROM app_user u
		WHERE u.username = ? COLLATE NOCASE
		  AND u.role = 'admin'
		  AND u.disabled_at IS NULL
		  AND u.must_change_password = TRUE
		  AND u.initial_password = TRUE
	)`, username)
	return required, err
}

func (q *SecurityQueries) EnabledAdminCount(ctx context.Context) (int64, error) {
	var count int64
	err := q.GetContext(ctx, &count, `SELECT COUNT(*) FROM app_user WHERE role = 'admin' AND disabled_at IS NULL`)
	return count, err
}

func (q *SecurityQueries) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	user := &models.User{}
	if err := q.GetContext(ctx, user, userSelect+` WHERE u.username = ? COLLATE NOCASE`, username); err != nil {
		return nil, err
	}
	user.LineupIDs, _ = q.GetUserLineupIDs(ctx, user.ID)
	return user, nil
}

func (q *SecurityQueries) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	user := &models.User{}
	if err := q.GetContext(ctx, user, userSelect+` WHERE u.id = ?`, id); err != nil {
		return nil, err
	}
	user.LineupIDs, _ = q.GetUserLineupIDs(ctx, id)
	return user, nil
}

func (q *SecurityQueries) ListUsers(ctx context.Context) ([]models.User, error) {
	users := []models.User{}
	if err := q.SelectContext(ctx, &users, userSelect+` ORDER BY LOWER(u.username), u.id`); err != nil {
		return nil, err
	}
	for i := range users {
		users[i].LineupIDs, _ = q.GetUserLineupIDs(ctx, users[i].ID)
	}
	return users, nil
}

func (q *SecurityQueries) CreateUser(ctx context.Context, username, displayName, passwordHash, role string, mustChange bool, lineupIDs []int64) (int64, error) {
	var id int64
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		result, err := tx.ExecContext(ctx, `INSERT INTO app_user
			(username, display_name, password_hash, role, must_change_password, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, username, displayName, passwordHash, role, mustChange)
		if err != nil {
			return err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return err
		}
		return replaceUserLineupsTx(ctx, tx, id, lineupIDs)
	})
	return id, err
}

// UpdateUserIdentity changes presentation identity without coupling device
// credentials to mutable names. A username change increments auth_version so
// every existing browser session becomes invalid; display-name-only changes do
// not unnecessarily interrupt sessions.
func (q *SecurityQueries) UpdateUserIdentity(ctx context.Context, id int64, username, displayName string) error {
	result, err := q.ExecContext(ctx, `UPDATE app_user
		SET username = ?, display_name = ?,
		    auth_version = auth_version + CASE WHEN username <> ? COLLATE NOCASE THEN 1 ELSE 0 END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, username, displayName, username, id)
	if err != nil {
		var sqliteError sqlite3.Error
		if errors.As(err, &sqliteError) && sqliteError.ExtendedCode == sqlite3.ErrConstraintUnique {
			return ErrUsernameInUse
		}
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (q *SecurityQueries) UpdateUser(ctx context.Context, id int64, role string, disabled bool, lineupIDs []int64) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var current struct {
			Role       string     `db:"role"`
			DisabledAt *time.Time `db:"disabled_at"`
		}
		if err := tx.GetContext(ctx, &current, `SELECT role, disabled_at FROM app_user WHERE id = ?`, id); err != nil {
			return err
		}
		if current.Role == models.RoleAdmin && current.DisabledAt == nil && (role != models.RoleAdmin || disabled) {
			var count int64
			if err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM app_user WHERE role = 'admin' AND disabled_at IS NULL`); err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastEnabledAdmin
			}
		}
		var disabledAt any
		if disabled {
			disabledAt = time.Now().UTC()
		}
		result, err := tx.ExecContext(ctx, `UPDATE app_user
			SET role = ?, disabled_at = ?, auth_version = auth_version + 1, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`, role, disabledAt, id)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return sql.ErrNoRows
		}
		if role == models.RoleAdmin {
			lineupIDs = nil
		}
		return replaceUserLineupsTx(ctx, tx, id, lineupIDs)
	})
}

func replaceUserLineupsTx(ctx context.Context, tx *sqlx.Tx, userID int64, lineupIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_lineup WHERE user_id = ?`, userID); err != nil {
		return err
	}
	seen := map[int64]struct{}{}
	for _, lineupID := range lineupIDs {
		if lineupID < 1 {
			continue
		}
		if _, ok := seen[lineupID]; ok {
			continue
		}
		seen[lineupID] = struct{}{}
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_lineup(user_id, lineup_id) VALUES (?, ?)`, userID, lineupID); err != nil {
			return err
		}
	}
	return nil
}

func (q *SecurityQueries) ReplaceUserLineups(ctx context.Context, userID int64, lineupIDs []int64) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		return replaceUserLineupsTx(ctx, tx, userID, lineupIDs)
	})
}

func (q *SecurityQueries) SetUserPassword(ctx context.Context, userID int64, hash string, mustChange bool) error {
	result, err := q.ExecContext(ctx, `UPDATE app_user SET password_hash = ?, must_change_password = ?,
		initial_password = FALSE, auth_version = auth_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, hash, mustChange, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (q *SecurityQueries) GetUserLineupIDs(ctx context.Context, userID int64) ([]int64, error) {
	ids := []int64{}
	err := q.SelectContext(ctx, &ids, `SELECT lineup_id FROM user_lineup WHERE user_id = ? ORDER BY lineup_id`, userID)
	return ids, err
}

func (q *SecurityQueries) GetLineupGrants(ctx context.Context) ([]models.LineupGrant, error) {
	rows := []models.LineupGrant{}
	err := q.SelectContext(ctx, &rows, `SELECT id, name FROM template ORDER BY LOWER(name), id`)
	return rows, err
}

func (q *SecurityQueries) GetLineupViewerIDs(ctx context.Context, lineupID int64) ([]int64, error) {
	ids := []int64{}
	err := q.SelectContext(ctx, &ids, `SELECT ul.user_id FROM user_lineup ul
		JOIN app_user u ON u.id = ul.user_id
		WHERE ul.lineup_id = ? AND u.role = 'viewer'`, lineupID)
	return ids, err
}

func (q *SecurityQueries) GetLineupMediaKeyIDs(ctx context.Context, lineupID int64) ([]int64, error) {
	ids := []int64{}
	err := q.SelectContext(ctx, &ids, `SELECT media_key_id FROM media_key_lineup WHERE lineup_id = ?`, lineupID)
	return ids, err
}

func (q *SecurityQueries) UserCanAccessLineup(ctx context.Context, userID int64, role string, lineupID int64) (bool, error) {
	if role == models.RoleAdmin {
		var exists bool
		err := q.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM template WHERE id = ?)`, lineupID)
		return exists, err
	}
	var allowed bool
	err := q.GetContext(ctx, &allowed, `SELECT EXISTS(
		SELECT 1 FROM user_lineup WHERE user_id = ? AND lineup_id = ?)`, userID, lineupID)
	return allowed, err
}

func (q *SecurityQueries) UserCanAccessChannel(ctx context.Context, userID int64, role, uuid string) (bool, error) {
	var allowed bool
	if role == models.RoleAdmin {
		err := q.GetContext(ctx, &allowed, `SELECT EXISTS(
			SELECT 1 FROM app_user u, templatechannel tc
			WHERE u.id = ? AND u.role = 'admin' AND u.disabled_at IS NULL AND tc.uuid = ?)`, userID, uuid)
		return allowed, err
	}
	err := q.GetContext(ctx, &allowed, `SELECT EXISTS(
		SELECT 1 FROM app_user u
		JOIN user_lineup ul ON ul.user_id = u.id
		JOIN template_group_item tgi ON tgi.template_id = ul.lineup_id
		JOIN template_group_channel tgc ON tgc.group_id = tgi.group_id
		JOIN templatechannel tc ON tc.id = tgc.channel_id
		WHERE u.id = ? AND u.role = 'viewer' AND u.disabled_at IS NULL AND tc.uuid = ?)`, userID, uuid)
	return allowed, err
}

func (q *SecurityQueries) CreateSession(ctx context.Context, session *models.AuthSession) error {
	_, err := q.ExecContext(ctx, `INSERT INTO auth_session
		(token_hash, user_id, auth_version, transport_scope, mfa_verified, reauthenticated_at,
		 created_at, last_seen_at, idle_expires_at, absolute_expires_at, client_ip, user_agent_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.TokenHash, session.UserID, session.AuthVersion, session.TransportScope,
		session.MFAVerified, session.ReauthenticatedAt, session.CreatedAt, session.LastSeenAt,
		session.IdleExpiresAt, session.AbsoluteExpiresAt, session.ClientIP, session.UserAgentHash)
	return err
}

func (q *SecurityQueries) GetSession(ctx context.Context, tokenHash []byte) (*models.AuthSession, error) {
	session := &models.AuthSession{}
	err := q.GetContext(ctx, session, `SELECT s.*, u.username, u.display_name, u.role,
		u.must_change_password, u.disabled_at AS user_disabled_at,
		EXISTS(SELECT 1 FROM user_mfa m WHERE m.user_id = u.id) AS mfa_enabled
		FROM auth_session s JOIN app_user u ON u.id = s.user_id
		WHERE s.token_hash = ?`, tokenHash)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// SessionCanAccessChannel revalidates both the browser session and its current
// resource grant in one query. Long-lived MPEG-TS responses call this
// periodically so logout, session revocation, expiry, role changes, and lineup
// grant changes take effect without waiting for the downstream socket to close.
func (q *SecurityQueries) SessionCanAccessChannel(ctx context.Context, sessionID int64, uuid string) (bool, error) {
	var allowed bool
	err := q.GetContext(ctx, &allowed, `SELECT EXISTS(
		SELECT 1 FROM auth_session s
		JOIN app_user u ON u.id = s.user_id
		WHERE s.id = ? AND s.revoked_at IS NULL
		  AND s.idle_expires_at > CURRENT_TIMESTAMP
		  AND s.absolute_expires_at > CURRENT_TIMESTAMP
		  AND s.auth_version = u.auth_version
		  AND u.disabled_at IS NULL
		  AND (NOT EXISTS(SELECT 1 FROM user_mfa m WHERE m.user_id = u.id) OR s.mfa_verified = 1)
		  AND (
			u.role = 'admin' AND EXISTS(SELECT 1 FROM templatechannel tc WHERE tc.uuid = ?)
			OR u.role = 'viewer' AND EXISTS(
				SELECT 1 FROM user_lineup ul
				JOIN template_group_item tgi ON tgi.template_id = ul.lineup_id
				JOIN template_group_channel tgc ON tgc.group_id = tgi.group_id
				JOIN templatechannel tc ON tc.id = tgc.channel_id
				WHERE ul.user_id = u.id AND tc.uuid = ?
			)
		  )
	)`, sessionID, uuid, uuid)
	return allowed, err
}

func (q *SecurityQueries) TouchSession(ctx context.Context, id int64, lastSeen, idleExpiry time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE auth_session SET last_seen_at = ?, idle_expires_at = ?
		WHERE id = ? AND revoked_at IS NULL`, lastSeen, idleExpiry, id)
	return err
}

func (q *SecurityQueries) MarkSessionReauthenticated(ctx context.Context, id int64, at time.Time, mfaVerified bool) error {
	_, err := q.ExecContext(ctx, `UPDATE auth_session SET reauthenticated_at = ?, mfa_verified = ?
		WHERE id = ? AND revoked_at IS NULL`, at, mfaVerified, id)
	return err
}

func (q *SecurityQueries) RevokeSessionByHash(ctx context.Context, hash []byte) error {
	_, err := q.ExecContext(ctx, `UPDATE auth_session SET revoked_at = CURRENT_TIMESTAMP WHERE token_hash = ? AND revoked_at IS NULL`, hash)
	return err
}

func (q *SecurityQueries) RevokeUserSessions(ctx context.Context, userID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE auth_session SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = ? AND revoked_at IS NULL`, userID)
	return err
}

func (q *SecurityQueries) ListUserSessions(ctx context.Context, userID int64) ([]models.AuthSessionMetadata, error) {
	items := []models.AuthSessionMetadata{}
	err := q.SelectContext(ctx, &items, `SELECT id, transport_scope, created_at, last_seen_at,
		idle_expires_at, absolute_expires_at, revoked_at, client_ip
		FROM auth_session WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID)
	for index := range items {
		items[index].ClientType = "browser"
	}
	return items, err
}

func (q *SecurityQueries) RevokeUserSessionByID(ctx context.Context, sessionID, userID int64) (bool, error) {
	result, err := q.ExecContext(ctx, `UPDATE auth_session SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, sessionID, userID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

const mobileSessionSelect = `SELECT s.*, u.username, u.display_name, u.role,
	u.must_change_password, u.disabled_at AS user_disabled_at,
	EXISTS(SELECT 1 FROM user_mfa m WHERE m.user_id = u.id) AS mfa_enabled
	FROM mobile_session s JOIN app_user u ON u.id = s.user_id`

func (q *SecurityQueries) CreateMobileSession(ctx context.Context, session *models.MobileSession) error {
	result, err := q.ExecContext(ctx, `INSERT INTO mobile_session
		(user_id, auth_version, device_name, access_token_hash, refresh_token_hash,
		 mfa_verified, reauthenticated_at, created_at, last_seen_at, access_expires_at,
		 refresh_expires_at, client_ip, user_agent_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, session.UserID,
		session.AuthVersion, session.DeviceName, session.AccessTokenHash, session.RefreshTokenHash,
		session.MFAVerified, session.ReauthenticatedAt, session.CreatedAt, session.LastSeenAt,
		session.AccessExpiresAt, session.RefreshExpiresAt, session.ClientIP, session.UserAgentHash)
	if err == nil {
		session.ID, err = result.LastInsertId()
	}
	return err
}

func (q *SecurityQueries) GetMobileSessionByAccessHash(ctx context.Context, hash []byte) (*models.MobileSession, error) {
	session := &models.MobileSession{}
	err := q.GetContext(ctx, session, mobileSessionSelect+` WHERE s.access_token_hash = ?`, hash)
	return session, err
}

func (q *SecurityQueries) GetMobileSessionByRefreshHash(ctx context.Context, hash []byte) (*models.MobileSession, error) {
	session := &models.MobileSession{}
	err := q.GetContext(ctx, session, mobileSessionSelect+` WHERE s.refresh_token_hash = ?`, hash)
	return session, err
}

func (q *SecurityQueries) GetMobileSessionByRefreshHistory(ctx context.Context, hash []byte) (*models.MobileSession, error) {
	session := &models.MobileSession{}
	err := q.GetContext(ctx, session, mobileSessionSelect+`
		JOIN mobile_refresh_history h ON h.mobile_session_id = s.id
		WHERE h.token_hash = ?`, hash)
	return session, err
}

// RotateMobileSession performs a compare-and-swap on the current refresh hash.
// If a concurrent or later caller presents a recorded hash, the transaction
// commits revocation before returning ErrMobileRefreshReplay.
func (q *SecurityQueries) RotateMobileSession(ctx context.Context, session *models.MobileSession, oldRefreshHash,
	newAccessHash, newRefreshHash []byte, accessExpiresAt, now time.Time) error {
	replay := false
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE mobile_session
			SET access_token_hash = ?, refresh_token_hash = ?, access_expires_at = ?, last_seen_at = ?
			WHERE id = ? AND refresh_token_hash = ? AND revoked_at IS NULL AND refresh_expires_at > ?`,
			newAccessHash, newRefreshHash, accessExpiresAt, now, session.ID, oldRefreshHash, now)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 1 {
			_, err = tx.ExecContext(ctx, `INSERT INTO mobile_refresh_history
				(mobile_session_id, token_hash, used_at, expires_at) VALUES (?, ?, ?, ?)`,
				session.ID, oldRefreshHash, now, session.RefreshExpiresAt)
			return err
		}
		var known bool
		if err := tx.GetContext(ctx, &known, `SELECT EXISTS(SELECT 1 FROM mobile_refresh_history
			WHERE mobile_session_id = ? AND token_hash = ?)`, session.ID, oldRefreshHash); err != nil {
			return err
		}
		if known {
			replay = true
			_, err = tx.ExecContext(ctx, `UPDATE mobile_session SET revoked_at = ?
				WHERE id = ? AND revoked_at IS NULL`, now, session.ID)
			return err
		}
		return sql.ErrNoRows
	})
	if err != nil {
		return err
	}
	if replay {
		return ErrMobileRefreshReplay
	}
	session.AccessTokenHash = newAccessHash
	session.RefreshTokenHash = newRefreshHash
	session.AccessExpiresAt = accessExpiresAt
	session.LastSeenAt = now
	return nil
}

func (q *SecurityQueries) TouchMobileSession(ctx context.Context, id int64, at time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE mobile_session SET last_seen_at = ?
		WHERE id = ? AND revoked_at IS NULL`, at, id)
	return err
}

func (q *SecurityQueries) RevokeMobileSession(ctx context.Context, id, userID int64) (bool, error) {
	result, err := q.ExecContext(ctx, `UPDATE mobile_session SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, id, userID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (q *SecurityQueries) RevokeUserMobileSessions(ctx context.Context, userID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE mobile_session SET revoked_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND revoked_at IS NULL`, userID)
	return err
}

func (q *SecurityQueries) ListUserMobileSessions(ctx context.Context, userID int64) ([]models.MobileSessionMetadata, error) {
	items := []models.MobileSessionMetadata{}
	err := q.SelectContext(ctx, &items, `SELECT id, device_name, created_at, last_seen_at,
		refresh_expires_at, revoked_at, client_ip FROM mobile_session
		WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID)
	for index := range items {
		items[index].ClientType = "mobile"
	}
	return items, err
}

func (q *SecurityQueries) MobileSessionCanAccessChannel(ctx context.Context, sessionID int64, uuid string) (bool, error) {
	var allowed bool
	err := q.GetContext(ctx, &allowed, `SELECT EXISTS(
		SELECT 1 FROM mobile_session s
		JOIN app_user u ON u.id = s.user_id
		WHERE s.id = ? AND s.revoked_at IS NULL
		  AND s.access_expires_at > CURRENT_TIMESTAMP
		  AND s.refresh_expires_at > CURRENT_TIMESTAMP
		  AND s.auth_version = u.auth_version
		  AND u.disabled_at IS NULL
		  AND (NOT EXISTS(SELECT 1 FROM user_mfa m WHERE m.user_id = u.id) OR s.mfa_verified = 1)
		  AND (
			u.role = 'admin' AND EXISTS(SELECT 1 FROM templatechannel tc WHERE tc.uuid = ?)
			OR u.role = 'viewer' AND EXISTS(
				SELECT 1 FROM user_lineup ul
				JOIN template_group_item tgi ON tgi.template_id = ul.lineup_id
				JOIN template_group_channel tgc ON tgc.group_id = tgi.group_id
				JOIN templatechannel tc ON tc.id = tgc.channel_id
				WHERE ul.user_id = u.id AND tc.uuid = ?
			)
		  )
	)`, sessionID, uuid, uuid)
	return allowed, err
}

func (q *SecurityQueries) CreateLoginChallenge(ctx context.Context, challenge *models.LoginChallenge) error {
	_, err := q.ExecContext(ctx, `INSERT INTO auth_login_challenge
		(token_hash, user_id, auth_version, transport_scope, created_at, expires_at,
		 client_ip, user_agent_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, challenge.TokenHash, challenge.UserID,
		challenge.AuthVersion, challenge.TransportScope, challenge.CreatedAt,
		challenge.ExpiresAt, challenge.ClientIP, challenge.UserAgentHash)
	return err
}

func (q *SecurityQueries) GetLoginChallenge(ctx context.Context, tokenHash []byte) (*models.LoginChallenge, error) {
	challenge := &models.LoginChallenge{}
	err := q.GetContext(ctx, challenge, `SELECT * FROM auth_login_challenge WHERE token_hash = ?`, tokenHash)
	if err != nil {
		return nil, err
	}
	return challenge, nil
}

func (q *SecurityQueries) RecordLoginChallengeFailure(ctx context.Context, id int64) error {
	_, err := q.ExecContext(ctx, `UPDATE auth_login_challenge SET failure_count = failure_count + 1
		WHERE id = ? AND consumed_at IS NULL`, id)
	return err
}

func (q *SecurityQueries) ConsumeLoginChallenge(ctx context.Context, id int64, now time.Time, maxFailures int) (bool, error) {
	result, err := q.ExecContext(ctx, `UPDATE auth_login_challenge SET consumed_at = ?
		WHERE id = ? AND consumed_at IS NULL AND expires_at > ? AND failure_count < ?`, now, id, now, maxFailures)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (q *SecurityQueries) CreateTrustedBrowser(ctx context.Context, browser *models.TrustedBrowser, maximum int) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO trusted_browser
			(token_hash, user_id, auth_version, transport_scope, created_at, last_used_at,
			 expires_at, created_ip, last_used_ip, user_agent, user_agent_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, browser.TokenHash, browser.UserID,
			browser.AuthVersion, browser.TransportScope, browser.CreatedAt, browser.LastUsedAt,
			browser.ExpiresAt, browser.CreatedIP, browser.LastUsedIP, browser.UserAgent,
			browser.UserAgentHash); err != nil {
			return err
		}
		if maximum > 0 {
			_, err := tx.ExecContext(ctx, `UPDATE trusted_browser SET revoked_at = ?
				WHERE user_id = ? AND revoked_at IS NULL AND id NOT IN (
					SELECT id FROM trusted_browser WHERE user_id = ? AND revoked_at IS NULL
					ORDER BY last_used_at DESC, id DESC LIMIT ?
				)`, browser.CreatedAt, browser.UserID, browser.UserID, maximum)
			return err
		}
		return nil
	})
}

func (q *SecurityQueries) GetValidTrustedBrowser(ctx context.Context, tokenHash []byte, userID, authVersion int64, scope string, now time.Time) (*models.TrustedBrowser, error) {
	browser := &models.TrustedBrowser{}
	err := q.GetContext(ctx, browser, `SELECT * FROM trusted_browser
		WHERE token_hash = ? AND user_id = ? AND auth_version = ? AND transport_scope = ?
		  AND revoked_at IS NULL AND expires_at > ?`, tokenHash, userID, authVersion, scope, now)
	if err != nil {
		return nil, err
	}
	return browser, nil
}

func (q *SecurityQueries) TouchTrustedBrowser(ctx context.Context, id int64, at time.Time, ip string) error {
	_, err := q.ExecContext(ctx, `UPDATE trusted_browser SET last_used_at = ?, last_used_ip = ?
		WHERE id = ? AND revoked_at IS NULL AND expires_at > ?`, at, ip, id, at)
	return err
}

func (q *SecurityQueries) ListTrustedBrowsers(ctx context.Context, userID int64) ([]models.TrustedBrowser, error) {
	items := []models.TrustedBrowser{}
	err := q.SelectContext(ctx, &items, `SELECT * FROM trusted_browser WHERE user_id = ?
		ORDER BY CASE WHEN revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP THEN 0 ELSE 1 END,
		last_used_at DESC, id DESC LIMIT 100`, userID)
	return items, err
}

func (q *SecurityQueries) RevokeTrustedBrowser(ctx context.Context, id, userID int64) (bool, error) {
	result, err := q.ExecContext(ctx, `UPDATE trusted_browser SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, id, userID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (q *SecurityQueries) RevokeUserTrustedBrowsers(ctx context.Context, userID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE trusted_browser SET revoked_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND revoked_at IS NULL`, userID)
	return err
}

func (q *SecurityQueries) DeleteUser(ctx context.Context, userID int64) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var target struct {
			Role       string     `db:"role"`
			DisabledAt *time.Time `db:"disabled_at"`
		}
		if err := tx.GetContext(ctx, &target, `SELECT role, disabled_at FROM app_user WHERE id = ?`, userID); err != nil {
			return err
		}
		if target.Role == models.RoleAdmin && target.DisabledAt == nil {
			var count int64
			if err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM app_user WHERE role = 'admin' AND disabled_at IS NULL`); err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastEnabledAdmin
			}
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM app_user WHERE id = ?`, userID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
}

func (q *SecurityQueries) SaveMFA(ctx context.Context, userID int64, encrypted []byte) error {
	_, err := q.ExecContext(ctx, `INSERT INTO user_mfa(user_id, encrypted_secret, last_counter, enabled_at, updated_at)
		VALUES (?, ?, -1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET encrypted_secret = excluded.encrypted_secret,
		last_counter = -1, enabled_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP`, userID, encrypted)
	return err
}

func (q *SecurityQueries) GetMFA(ctx context.Context, userID int64) ([]byte, int64, error) {
	var row struct {
		Secret  []byte `db:"encrypted_secret"`
		Counter int64  `db:"last_counter"`
	}
	err := q.GetContext(ctx, &row, `SELECT encrypted_secret, last_counter FROM user_mfa WHERE user_id = ?`, userID)
	return row.Secret, row.Counter, err
}

func (q *SecurityQueries) UseMFACounter(ctx context.Context, userID, counter int64) (bool, error) {
	result, err := q.ExecContext(ctx, `UPDATE user_mfa SET last_counter = ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND last_counter < ?`, counter, userID, counter)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (q *SecurityQueries) DisableMFA(ctx context.Context, userID int64) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_mfa_recovery_code WHERE user_id = ?`, userID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM user_mfa WHERE user_id = ?`, userID)
		return err
	})
}

func (q *SecurityQueries) ReplaceRecoveryCodes(ctx context.Context, userID int64, hashes [][]byte) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_mfa_recovery_code WHERE user_id = ?`, userID); err != nil {
			return err
		}
		for _, hash := range hashes {
			if _, err := tx.ExecContext(ctx, `INSERT INTO user_mfa_recovery_code(user_id, code_hash) VALUES (?, ?)`, userID, hash); err != nil {
				return err
			}
		}
		return nil
	})
}

func (q *SecurityQueries) UseRecoveryCode(ctx context.Context, userID int64, hash []byte) (bool, error) {
	result, err := q.ExecContext(ctx, `UPDATE user_mfa_recovery_code SET used_at = CURRENT_TIMESTAMP
		WHERE id = (SELECT id FROM user_mfa_recovery_code WHERE user_id = ? AND code_hash = ? AND used_at IS NULL LIMIT 1)`, userID, hash)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (q *SecurityQueries) CreateMediaKey(ctx context.Context, key *models.MediaAccessKey, lineupIDs []int64) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		result, err := tx.ExecContext(ctx, `INSERT INTO media_access_key
			(user_id, name, token_prefix, token_hash, token_cipher, network_scope, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, key.UserID, key.Name, key.TokenPrefix, key.TokenHash,
			key.TokenCipher, key.NetworkScope, key.CreatedAt, key.ExpiresAt)
		if err != nil {
			return err
		}
		key.ID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		for _, lineupID := range lineupIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO media_key_lineup(media_key_id, lineup_id) VALUES (?, ?)`, key.ID, lineupID); err != nil {
				return err
			}
		}
		for index := range key.OutputAliases {
			alias := &key.OutputAliases[index]
			alias.MediaKeyID = key.ID
			result, err := tx.ExecContext(ctx, `INSERT INTO media_output_alias
				(media_key_id, lineup_id, code_hash, code_cipher, created_at)
				VALUES (?, ?, ?, ?, ?)`, alias.MediaKeyID, alias.LineupID, alias.CodeHash, alias.CodeCipher, alias.CreatedAt)
			if err != nil {
				return err
			}
			alias.ID, err = result.LastInsertId()
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (q *SecurityQueries) ListMediaKeys(ctx context.Context, userID int64) ([]models.MediaAccessKey, error) {
	keys := []models.MediaAccessKey{}
	if err := q.SelectContext(ctx, &keys, `SELECT k.*, u.username FROM media_access_key k
		JOIN app_user u ON u.id = k.user_id WHERE k.user_id = ?
		ORDER BY CASE WHEN k.revoked_at IS NULL THEN 0 ELSE 1 END,
			k.created_at DESC, k.id DESC`, userID); err != nil {
		return nil, err
	}
	for i := range keys {
		keys[i].LineupIDs, _ = q.GetMediaKeyLineupIDs(ctx, keys[i].ID)
	}
	return keys, nil
}

func (q *SecurityQueries) GetMediaKeyByPrefix(ctx context.Context, prefix string) (*models.MediaAccessKey, error) {
	key := &models.MediaAccessKey{}
	if err := q.GetContext(ctx, key, `SELECT k.*, u.username FROM media_access_key k
		JOIN app_user u ON u.id = k.user_id WHERE k.token_prefix = ? AND u.disabled_at IS NULL`, prefix); err != nil {
		return nil, err
	}
	key.LineupIDs, _ = q.GetMediaKeyLineupIDs(ctx, key.ID)
	return key, nil
}

func (q *SecurityQueries) GetMediaKeyByID(ctx context.Context, id int64) (*models.MediaAccessKey, error) {
	key := &models.MediaAccessKey{}
	if err := q.GetContext(ctx, key, `SELECT k.*, u.username FROM media_access_key k
		JOIN app_user u ON u.id = k.user_id WHERE k.id = ? AND u.disabled_at IS NULL`, id); err != nil {
		return nil, err
	}
	key.LineupIDs, _ = q.GetMediaKeyLineupIDs(ctx, key.ID)
	return key, nil
}

func (q *SecurityQueries) ListMediaOutputAliases(ctx context.Context, keyID int64) ([]models.MediaOutputAlias, error) {
	aliases := []models.MediaOutputAlias{}
	err := q.SelectContext(ctx, &aliases, `SELECT * FROM media_output_alias
		WHERE media_key_id = ? ORDER BY lineup_id`, keyID)
	return aliases, err
}

func (q *SecurityQueries) CreateMediaOutputAlias(ctx context.Context, alias *models.MediaOutputAlias) error {
	result, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO media_output_alias
		(media_key_id, lineup_id, code_hash, code_cipher, created_at)
		VALUES (?, ?, ?, ?, ?)`, alias.MediaKeyID, alias.LineupID, alias.CodeHash, alias.CodeCipher, alias.CreatedAt)
	if err != nil {
		return err
	}
	alias.ID, err = result.LastInsertId()
	return err
}

func (q *SecurityQueries) GetMediaOutputAliasByHash(ctx context.Context, hash []byte) (*models.MediaOutputAlias, error) {
	alias := &models.MediaOutputAlias{}
	if err := q.GetContext(ctx, alias, `SELECT * FROM media_output_alias WHERE code_hash = ?`, hash); err != nil {
		return nil, err
	}
	return alias, nil
}

func (q *SecurityQueries) GetMediaKeyLineupIDs(ctx context.Context, keyID int64) ([]int64, error) {
	ids := []int64{}
	err := q.SelectContext(ctx, &ids, `SELECT lineup_id FROM media_key_lineup WHERE media_key_id = ? ORDER BY lineup_id`, keyID)
	return ids, err
}

func (q *SecurityQueries) TouchMediaKey(ctx context.Context, id int64, ip string) error {
	_, err := q.ExecContext(ctx, `UPDATE media_access_key SET last_used_at = CURRENT_TIMESTAMP, last_used_ip = ? WHERE id = ?`, ip, id)
	return err
}

func (q *SecurityQueries) RevokeMediaKey(ctx context.Context, id, userID int64) (bool, error) {
	result, err := q.ExecContext(ctx, `UPDATE media_access_key SET revoked_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, id, userID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (q *SecurityQueries) RevokeUserMediaKeys(ctx context.Context, userID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE media_access_key SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = ? AND revoked_at IS NULL`, userID)
	return err
}

func (q *SecurityQueries) MediaKeyAllowsLineup(ctx context.Context, keyID, lineupID int64) (bool, error) {
	var allowed bool
	err := q.GetContext(ctx, &allowed, `SELECT EXISTS(
		SELECT 1 FROM media_key_lineup mkl
		JOIN media_access_key mak ON mak.id = mkl.media_key_id
		JOIN app_user u ON u.id = mak.user_id
		LEFT JOIN user_lineup ul ON ul.user_id = u.id AND ul.lineup_id = mkl.lineup_id
		WHERE mkl.media_key_id = ? AND mkl.lineup_id = ?
		  AND mak.revoked_at IS NULL AND (mak.expires_at IS NULL OR mak.expires_at > CURRENT_TIMESTAMP)
		  AND u.disabled_at IS NULL AND (u.role = 'admin' OR ul.lineup_id IS NOT NULL))`, keyID, lineupID)
	return allowed, err
}

func (q *SecurityQueries) MediaKeyAllowsChannel(ctx context.Context, keyID int64, uuid string) (bool, error) {
	var allowed bool
	err := q.GetContext(ctx, &allowed, `SELECT EXISTS(
		SELECT 1 FROM media_key_lineup mkl
		JOIN media_access_key mak ON mak.id = mkl.media_key_id
		JOIN app_user u ON u.id = mak.user_id
		LEFT JOIN user_lineup ul ON ul.user_id = u.id AND ul.lineup_id = mkl.lineup_id
		JOIN template_group_item tgi ON tgi.template_id = mkl.lineup_id
		JOIN template_group_channel tgc ON tgc.group_id = tgi.group_id
		JOIN templatechannel tc ON tc.id = tgc.channel_id
		WHERE mkl.media_key_id = ? AND tc.uuid = ?
		  AND mak.revoked_at IS NULL AND (mak.expires_at IS NULL OR mak.expires_at > CURRENT_TIMESTAMP)
		  AND u.disabled_at IS NULL AND (u.role = 'admin' OR ul.lineup_id IS NOT NULL))`, keyID, uuid)
	return allowed, err
}

// LineupContainsLogo prevents an authorized lineup id from being used as a
// generic capability for unrelated assets in the logo library.
func (q *SecurityQueries) LineupContainsLogo(ctx context.Context, lineupID int64, asset string) (bool, error) {
	var allowed bool
	err := q.GetContext(ctx, &allowed, `SELECT EXISTS(
		SELECT 1 FROM template_group_item tgi
		JOIN template_group_channel tgc ON tgc.group_id = tgi.group_id
		JOIN templatechannel tc ON tc.id = tgc.channel_id
		JOIN logo l ON l.id = COALESCE(tc.logoid, 0)
		WHERE tgi.template_id = ? AND l.name || '.png' = ?)`, lineupID, asset)
	return allowed, err
}

func (q *SecurityQueries) VirtualTunerAllowsChannel(ctx context.Context, lineupID, version int64, uuid string) (bool, error) {
	var allowed bool
	err := q.GetContext(ctx, &allowed, `SELECT EXISTS(
		SELECT 1 FROM template t
		JOIN template_group_item tgi ON tgi.template_id = t.id
		JOIN template_group_channel tgc ON tgc.group_id = tgi.group_id
		JOIN templatechannel tc ON tc.id = tgc.channel_id
		WHERE t.id = ? AND t.virtual_tuner_enabled = 1
		  AND t.virtual_tuner_token_version = ? AND tc.uuid = ?)`, lineupID, version, uuid)
	return allowed, err
}

func (q *SecurityQueries) VirtualTunerCredentialValid(ctx context.Context, lineupID, version int64) (bool, error) {
	var allowed bool
	err := q.GetContext(ctx, &allowed, `SELECT EXISTS(SELECT 1 FROM template
		WHERE id = ? AND virtual_tuner_enabled = 1 AND virtual_tuner_token_version = ?)`, lineupID, version)
	return allowed, err
}

const authThrottleSelect = `SELECT b.*, COALESCE(u.username, '') AS username,
	COALESCE(u.display_name, '') AS display_name
	FROM auth_throttle_bucket b LEFT JOIN app_user u ON u.id = b.user_id`

func authCooldown(subjectType string, failures int) time.Duration {
	switch subjectType {
	case "account", "pair":
		if failures < 5 {
			return 0
		}
		shift := min(failures-5, 5)
		return min(30*time.Second*time.Duration(1<<shift), 15*time.Minute)
	case "address":
		if failures >= 30 {
			return 15 * time.Minute
		}
	case "network":
		if failures >= 100 {
			return 15 * time.Minute
		}
	}
	return 0
}

// CheckAuthThrottle evaluates durable, independent account, address, network,
// and pair buckets. The bot challenge starts before the hard cooldown so a
// legitimate user gets one more protected opportunity to authenticate.
func (q *SecurityQueries) CheckAuthThrottle(ctx context.Context, hashes [][]byte, now time.Time) (models.AuthThrottleDecision, error) {
	decision := models.AuthThrottleDecision{}
	if len(hashes) == 0 {
		return decision, nil
	}
	query, args, err := sqlx.In(authThrottleSelect+` WHERE b.bucket_hash IN (?)`, hashes)
	if err != nil {
		return decision, err
	}
	rows := []models.AuthThrottleBucket{}
	if err := q.SelectContext(ctx, &rows, q.Rebind(query), args...); err != nil {
		return decision, err
	}
	for _, row := range rows {
		if row.BlockedUntil != nil && row.BlockedUntil.After(now) {
			decision.Blocked = true
			if retry := row.BlockedUntil.Sub(now); retry > decision.RetryAfter {
				decision.RetryAfter = retry
			}
		}
		if now.Sub(row.LastFailedAt) <= 15*time.Minute {
			switch row.SubjectType {
			case "account", "pair":
				decision.ChallengeRequired = decision.ChallengeRequired || row.ConsecutiveFailures >= 3
			case "address":
				decision.ChallengeRequired = decision.ChallengeRequired || row.ConsecutiveFailures >= 10
			case "network":
				decision.ChallengeRequired = decision.ChallengeRequired || row.ConsecutiveFailures >= 25
			}
		}
	}
	return decision, nil
}

func (q *SecurityQueries) RecordAuthFailure(ctx context.Context, inputs []models.AuthThrottleInput, factor string, now time.Time) (models.AuthThrottleDecision, error) {
	decision := models.AuthThrottleDecision{}
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		for _, input := range inputs {
			var existing models.AuthThrottleBucket
			err := tx.GetContext(ctx, &existing, `SELECT * FROM auth_throttle_bucket WHERE bucket_hash = ?`, input.BucketHash)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			failures := 1
			windowStarted := now
			createdAt := now
			passwordFailures, mfaFailures, pendingMFA := 0, 0, 0
			denied := 0
			var oldBlocked *time.Time
			if err == nil {
				createdAt = existing.CreatedAt
				windowStarted = existing.WindowStartedAt
				failures = existing.ConsecutiveFailures + 1
				if now.Sub(existing.LastFailedAt) > 15*time.Minute {
					failures = 1
					windowStarted = now
				}
				passwordFailures = existing.PasswordFailures
				mfaFailures = existing.MFAFailures
				pendingMFA = existing.PendingMFANotice
				denied = existing.DeniedRequests
				oldBlocked = existing.BlockedUntil
			}
			if factor == "mfa" {
				mfaFailures++
				if input.SubjectType == "account" {
					pendingMFA++
				}
			} else {
				passwordFailures++
			}
			cooldown := authCooldown(input.SubjectType, failures)
			if input.SubjectType == "account" {
				decision.FailureCount = failures
			}
			var blockedUntil *time.Time
			if cooldown > 0 {
				value := now.Add(cooldown)
				blockedUntil = &value
				decision.Blocked = true
				if cooldown > decision.RetryAfter {
					decision.RetryAfter = cooldown
				}
				if oldBlocked == nil || !oldBlocked.After(now) {
					decision.NewlyBlocked = true
				}
			} else if oldBlocked != nil && oldBlocked.After(now) {
				blockedUntil = oldBlocked
				decision.Blocked = true
				if retry := oldBlocked.Sub(now); retry > decision.RetryAfter {
					decision.RetryAfter = retry
				}
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO auth_throttle_bucket
				(bucket_hash, subject_type, user_id, client_ip, network_prefix, consecutive_failures,
				 password_failures, mfa_failures, pending_mfa_notice, denied_requests, window_started_at,
				 last_failed_at, blocked_until, last_factor, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(bucket_hash) DO UPDATE SET
				 user_id = COALESCE(excluded.user_id, auth_throttle_bucket.user_id),
				 client_ip = excluded.client_ip, network_prefix = excluded.network_prefix,
				 consecutive_failures = excluded.consecutive_failures,
				 password_failures = excluded.password_failures, mfa_failures = excluded.mfa_failures,
				 pending_mfa_notice = excluded.pending_mfa_notice, denied_requests = excluded.denied_requests,
				 window_started_at = excluded.window_started_at, last_failed_at = excluded.last_failed_at,
				 blocked_until = excluded.blocked_until, last_factor = excluded.last_factor,
				 updated_at = excluded.updated_at`, input.BucketHash, input.SubjectType, input.UserID,
				input.ClientIP, input.NetworkPrefix, failures, passwordFailures, mfaFailures, pendingMFA,
				denied, windowStarted, now, blockedUntil, factor, createdAt, now)
			if err != nil {
				return err
			}
		}
		if authThrottleWrites.Add(1)%256 == 0 {
			// Attacker-selected usernames and addresses must not create unbounded
			// durable state between maintenance passes. Run the comparatively
			// expensive ceiling check only periodically, and retain active
			// cooldowns plus known account buckets preferentially.
			_, err := tx.ExecContext(ctx, `DELETE FROM auth_throttle_bucket WHERE id IN (
				SELECT id FROM auth_throttle_bucket
				WHERE blocked_until IS NULL OR blocked_until <= ?
				ORDER BY CASE
					WHEN user_id IS NULL THEN 0
					WHEN subject_type <> 'account' THEN 1
					ELSE 2 END,
					updated_at ASC, id ASC
				LIMIT (SELECT MAX(COUNT(*) - 100000, 0) FROM auth_throttle_bucket))`, now)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return decision, err
}

func (q *SecurityQueries) RecordAuthThrottleDenial(ctx context.Context, hashes [][]byte, now time.Time) error {
	if len(hashes) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`UPDATE auth_throttle_bucket
		SET denied_requests = denied_requests + 1, updated_at = ?
		WHERE id = (SELECT id FROM auth_throttle_bucket
			WHERE bucket_hash IN (?) AND blocked_until > ?
			ORDER BY CASE subject_type WHEN 'account' THEN 1 WHEN 'pair' THEN 2 WHEN 'address' THEN 3 ELSE 4 END
			LIMIT 1)`, now, hashes, now)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, q.Rebind(query), args...)
	return err
}

func (q *SecurityQueries) ResetAuthThrottle(ctx context.Context, hashes [][]byte, now time.Time) error {
	if len(hashes) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`UPDATE auth_throttle_bucket SET consecutive_failures = 0,
		pending_mfa_notice = 0, blocked_until = NULL, window_started_at = ?, updated_at = ?
		WHERE bucket_hash IN (?)`, now, now, hashes)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, q.Rebind(query), args...)
	return err
}

func (q *SecurityQueries) AccountMFANotice(ctx context.Context, accountHash []byte) (*models.AuthenticationNotice, error) {
	var row models.AuthThrottleBucket
	if err := q.GetContext(ctx, &row, `SELECT * FROM auth_throttle_bucket WHERE bucket_hash = ?`, accountHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if row.PendingMFANotice == 0 {
		return nil, nil
	}
	return &models.AuthenticationNotice{Kind: "failed_mfa", Count: row.PendingMFANotice,
		LastAt: row.LastFailedAt, LastIP: row.ClientIP,
		Message: "A verification code was rejected before this sign-in succeeded."}, nil
}

func (q *SecurityQueries) ListAuthThrottleBuckets(ctx context.Context, now, since time.Time) ([]models.AuthThrottleBucket, error) {
	items := []models.AuthThrottleBucket{}
	err := q.SelectContext(ctx, &items, authThrottleSelect+`
		WHERE b.last_failed_at >= ? OR b.blocked_until > ?
		ORDER BY (b.blocked_until > ?) DESC, b.last_failed_at DESC, b.id DESC LIMIT 500`, since, now, now)
	return items, err
}

func (q *SecurityQueries) ClearAuthThrottleBucket(ctx context.Context, id int64) (bool, error) {
	cleared := false
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		var item models.AuthThrottleBucket
		if err := tx.GetContext(ctx, &item, `SELECT * FROM auth_throttle_bucket WHERE id = ?`, id); err != nil {
			return err
		}
		var result sql.Result
		var err error
		switch {
		case item.UserID != nil && (item.SubjectType == "account" || item.SubjectType == "pair"):
			// Account and pair cooldowns start together. Clearing either one as
			// an administrator must actually restore that account's ability to
			// authenticate, while independent abusive address/network signals stay.
			result, err = tx.ExecContext(ctx, `DELETE FROM auth_throttle_bucket
				WHERE user_id = ? AND subject_type IN ('account', 'pair')`, *item.UserID)
		case item.SubjectType == "address":
			result, err = tx.ExecContext(ctx, `DELETE FROM auth_throttle_bucket
				WHERE id = ? OR (subject_type = 'pair' AND client_ip = ?)`, id, item.ClientIP)
		case item.SubjectType == "network":
			result, err = tx.ExecContext(ctx, `DELETE FROM auth_throttle_bucket
				WHERE id = ? OR (subject_type IN ('address', 'pair') AND network_prefix = ?)`, id, item.NetworkPrefix)
		default:
			result, err = tx.ExecContext(ctx, `DELETE FROM auth_throttle_bucket WHERE id = ?`, id)
		}
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		cleared = rows > 0
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return cleared, err
}

func (q *SecurityQueries) CreateBotChallenge(ctx context.Context, challenge *models.BotChallenge) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM auth_bot_challenge
			WHERE expires_at < ? OR used_at IS NOT NULL`, challenge.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO auth_bot_challenge
			(token_hash, account_hash, address_hash, difficulty, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?)`, challenge.TokenHash, challenge.AccountHash, challenge.AddressHash,
			challenge.Difficulty, challenge.CreatedAt, challenge.ExpiresAt); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM auth_bot_challenge WHERE id IN (
			SELECT id FROM auth_bot_challenge ORDER BY created_at DESC, id DESC LIMIT -1 OFFSET 50000)`)
		return err
	})
}

func (q *SecurityQueries) GetBotChallenge(ctx context.Context, tokenHash []byte) (*models.BotChallenge, error) {
	challenge := &models.BotChallenge{}
	if err := q.GetContext(ctx, challenge, `SELECT * FROM auth_bot_challenge WHERE token_hash = ?`, tokenHash); err != nil {
		return nil, err
	}
	return challenge, nil
}

func (q *SecurityQueries) ConsumeBotChallenge(ctx context.Context, id int64, now time.Time) (bool, error) {
	result, err := q.ExecContext(ctx, `UPDATE auth_bot_challenge SET used_at = ?
		WHERE id = ? AND used_at IS NULL AND expires_at > ?`, now, id, now)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (q *SecurityQueries) Audit(ctx context.Context, event models.SecurityAuditEvent) error {
	_, err := q.ExecContext(ctx, `INSERT INTO security_audit_event
		(actor_user_id, actor_username, actor_display_name, target_user_id, target_username,
		 target_display_name, action, outcome, resource_type, resource_id, client_ip, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.ActorUserID, event.ActorUsername,
		event.ActorDisplayName, event.TargetUserID, event.TargetUsername, event.TargetDisplayName,
		event.Action, event.Outcome, event.ResourceType, event.ResourceID, event.ClientIP, event.Detail)
	return err
}

func (q *SecurityQueries) ListSecurityAuditEvents(ctx context.Context, beforeID int64, limit int) ([]models.SecurityAuditEvent, int64, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	var total int64
	if err := q.GetContext(ctx, &total, `SELECT COUNT(*) FROM security_audit_event`); err != nil {
		return nil, 0, err
	}
	items := []models.SecurityAuditEvent{}
	if beforeID > 0 {
		if err := q.SelectContext(ctx, &items, `SELECT * FROM security_audit_event
			WHERE id < ? ORDER BY id DESC LIMIT ?`, beforeID, limit+1); err != nil {
			return nil, 0, err
		}
	} else if err := q.SelectContext(ctx, &items, `SELECT * FROM security_audit_event
		ORDER BY id DESC LIMIT ?`, limit+1); err != nil {
		return nil, 0, err
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

func (q *SecurityQueries) PruneSecurityData(ctx context.Context, sessionBefore, auditBefore time.Time, maxAudit int) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM auth_session WHERE absolute_expires_at < ? OR revoked_at < ?`, sessionBefore, sessionBefore); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM mobile_refresh_history WHERE expires_at < ?`, sessionBefore); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM mobile_session
			WHERE refresh_expires_at < ? OR revoked_at < ?`, sessionBefore, sessionBefore); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM auth_login_challenge
			WHERE expires_at < ? OR consumed_at IS NOT NULL`, sessionBefore); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM auth_bot_challenge
			WHERE expires_at < ? OR used_at IS NOT NULL`, sessionBefore); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM auth_throttle_bucket
			WHERE updated_at < ? AND (blocked_until IS NULL OR blocked_until < ?)`, auditBefore, sessionBefore); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM trusted_browser
			WHERE expires_at < ? OR revoked_at < ?`, sessionBefore, sessionBefore); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM security_audit_event WHERE created_at < ?`, auditBefore); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_mfa_recovery_code WHERE used_at IS NOT NULL AND used_at < ?`, auditBefore); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_access_key
			WHERE (revoked_at IS NOT NULL AND revoked_at < ?)
			   OR (expires_at IS NOT NULL AND expires_at < ?)`, auditBefore, auditBefore); err != nil {
			return err
		}
		if maxAudit > 0 {
			_, err := tx.ExecContext(ctx, `DELETE FROM security_audit_event WHERE id IN (
				SELECT id FROM security_audit_event ORDER BY created_at DESC, id DESC LIMIT -1 OFFSET ?)`, maxAudit)
			return err
		}
		return nil
	})
}
