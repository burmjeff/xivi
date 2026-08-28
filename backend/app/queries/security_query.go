package queries

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
)

type SecurityQueries struct{ BaseQueries }

var ErrLastEnabledAdmin = errors.New("the final enabled administrator cannot be changed")

func NewSecurityQueries(db *sqlx.DB) *SecurityQueries {
	return &SecurityQueries{BaseQueries: NewBaseQueries(db)}
}

const userSelect = `
	SELECT u.id, u.username, u.password_hash, u.role, u.must_change_password, u.initial_password,
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

func (q *SecurityQueries) CreateUser(ctx context.Context, username, passwordHash, role string, mustChange bool, lineupIDs []int64) (int64, error) {
	var id int64
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		result, err := tx.ExecContext(ctx, `INSERT INTO app_user
			(username, password_hash, role, must_change_password, created_at, updated_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, username, passwordHash, role, mustChange)
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
	err := q.GetContext(ctx, session, `SELECT s.*, u.username, u.role,
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
		return nil
	})
}

func (q *SecurityQueries) ListMediaKeys(ctx context.Context, userID int64) ([]models.MediaAccessKey, error) {
	keys := []models.MediaAccessKey{}
	if err := q.SelectContext(ctx, &keys, `SELECT k.*, u.username FROM media_access_key k
		JOIN app_user u ON u.id = k.user_id WHERE k.user_id = ? ORDER BY k.created_at DESC, k.id DESC`, userID); err != nil {
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

func (q *SecurityQueries) Audit(ctx context.Context, event models.SecurityAuditEvent) error {
	_, err := q.ExecContext(ctx, `INSERT INTO security_audit_event
		(actor_user_id, actor_username, target_user_id, target_username, action, outcome, resource_type, resource_id, client_ip, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, event.ActorUserID, event.ActorUsername,
		event.TargetUserID, event.TargetUsername, event.Action, event.Outcome, event.ResourceType,
		event.ResourceID, event.ClientIP, event.Detail)
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
