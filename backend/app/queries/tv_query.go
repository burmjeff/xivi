package queries

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
	"xivi/backend/app/models"

	"github.com/jmoiron/sqlx"
)

var (
	ErrTVPending          = errors.New("authorization_pending")
	ErrTVSlowDown         = errors.New("slow_down")
	ErrTVGrant            = errors.New("invalid_grant")
	ErrPreferenceConflict = errors.New("preference_conflict")
)

func (q *SecurityQueries) ConsumeTVProof(ctx context.Context, thumb, jti string, now time.Time) (bool, error) {
	// Keep replay evidence for the whole accepted iat window, across restarts.
	_, err := q.ExecContext(ctx, `DELETE FROM tv_dpop_replay WHERE expires_at < ?`, now)
	if err != nil {
		return false, err
	}
	r, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO tv_dpop_replay(key_thumbprint,jti,expires_at) VALUES(?,?,?)`, thumb, jti, now.Add(7*time.Minute))
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}

func (q *SecurityQueries) CreateTVPairing(ctx context.Context, p *models.TVPairing) error {
	_, err := q.ExecContext(ctx, `DELETE FROM tv_pairing WHERE expires_at < ?`, p.CreatedAt)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO tv_pairing(device_code_hash,user_code_hash,device_name,key_thumbprint,created_at,expires_at) VALUES(?,?,?,?,?,?)`, p.DeviceCodeHash, p.UserCodeHash, p.DeviceName, p.KeyThumbprint, p.CreatedAt, p.ExpiresAt)
	return err
}

func (q *SecurityQueries) GetTVPairingByUserCode(ctx context.Context, hash []byte, now time.Time) (*models.TVPairing, error) {
	p := &models.TVPairing{}
	err := q.GetContext(ctx, p, `SELECT * FROM tv_pairing WHERE user_code_hash=? AND expires_at>? AND decision='pending'`, hash, now)
	return p, err
}

func (q *SecurityQueries) DecideTVPairing(ctx context.Context, hash []byte, userID, version int64, approve bool, now time.Time) (bool, error) {
	decision := "denied"
	if approve {
		decision = "approved"
	}
	r, err := q.ExecContext(ctx, `UPDATE tv_pairing SET decision=?,user_id=?,auth_version=? WHERE user_code_hash=? AND decision='pending' AND expires_at>?`, decision, userID, version, hash, now)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}

// ExchangeTVPairing commits enrollment and encrypted retry delivery together. A
// lost HTTP response cannot create duplicate devices or destroy the credential.
func (q *SecurityQueries) ExchangeTVPairing(ctx context.Context, codeHash []byte, thumb string, refreshHash, refreshCipher []byte, ip string, now time.Time) (*models.TVDevice, []byte, error) {
	var device models.TVDevice
	var cipher []byte
	var outcome error
	err := q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		p := models.TVPairing{}
		if err := tx.GetContext(ctx, &p, `SELECT * FROM tv_pairing WHERE device_code_hash=? AND key_thumbprint=? AND expires_at>?`, codeHash, thumb, now); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrTVGrant
			}
			return err
		}
		if p.LastPolledAt != nil && now.Sub(*p.LastPolledAt) < 5*time.Second {
			return ErrTVSlowDown
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tv_pairing SET last_polled_at=? WHERE id=?`, now, p.ID); err != nil {
			return err
		}
		if p.Decision == "pending" {
			outcome = ErrTVPending
			return nil
		}
		if p.Decision != "approved" || p.UserID == nil || p.AuthVersion == nil {
			return ErrTVGrant
		}
		var valid bool
		if err := tx.GetContext(ctx, &valid, `SELECT EXISTS(SELECT 1 FROM app_user WHERE id=? AND auth_version=? AND disabled_at IS NULL AND must_change_password=FALSE)`, *p.UserID, *p.AuthVersion); err != nil {
			return err
		}
		if !valid {
			return ErrTVGrant
		}
		if p.DeviceID == nil {
			var count int
			if err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM tv_device WHERE user_id=? AND revoked_at IS NULL`, *p.UserID); err != nil {
				return err
			}
			if count >= 50 {
				return ErrTVGrant
			}
			r, err := tx.ExecContext(ctx, `INSERT INTO tv_device(user_id,auth_version,device_name,key_thumbprint,refresh_token_hash,created_at,last_seen_at,client_ip) VALUES(?,?,?,?,?,?,?,?)`, *p.UserID, *p.AuthVersion, p.DeviceName, thumb, refreshHash, now, now, ip)
			if err != nil {
				return err
			}
			id, err := r.LastInsertId()
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE tv_pairing SET device_id=?,refresh_token_cipher=? WHERE id=?`, id, refreshCipher, p.ID); err != nil {
				return err
			}
			p.DeviceID = &id
			p.RefreshTokenCipher = refreshCipher
		}
		if err := tx.GetContext(ctx, &device, `SELECT d.*, 'tv' AS session_type FROM tv_device d WHERE id=? AND revoked_at IS NULL`, *p.DeviceID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrTVGrant
			}
			return err
		}
		cipher = p.RefreshTokenCipher
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &device, cipher, outcome
}

func (q *SecurityQueries) GetTVDeviceByRefresh(ctx context.Context, hash []byte) (*models.TVDevice, error) {
	d := &models.TVDevice{}
	err := q.GetContext(ctx, d, `SELECT d.*, 'tv' AS session_type FROM tv_device d WHERE refresh_token_hash=?`, hash)
	return d, err
}
func (q *SecurityQueries) GetTVDeviceByAccess(ctx context.Context, hash []byte, now time.Time) (*models.TVDevice, error) {
	d := &models.TVDevice{}
	err := q.GetContext(ctx, d, `SELECT d.*, 'tv' AS session_type FROM tv_device d JOIN tv_access_token a ON a.device_id=d.id WHERE a.token_hash=? AND a.expires_at>?`, hash, now)
	return d, err
}
func (q *SecurityQueries) IssueTVAccess(ctx context.Context, deviceID int64, hash []byte, now time.Time) error {
	return q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM tv_pairing WHERE expires_at<=?`, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tv_access_token WHERE expires_at<=?`, now); err != nil {
			return err
		}
		r, err := tx.ExecContext(ctx, `INSERT INTO tv_access_token(token_hash,device_id,expires_at) SELECT ?,d.id,? FROM tv_device d JOIN app_user u ON u.id=d.user_id WHERE d.id=? AND d.revoked_at IS NULL AND u.disabled_at IS NULL AND u.must_change_password=FALSE AND d.auth_version=u.auth_version`, hash, now.Add(15*time.Minute), deviceID)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrTVGrant
		}
		_, err = tx.ExecContext(ctx, `UPDATE tv_device SET last_seen_at=? WHERE id=?`, now, deviceID)
		return err
	})
}
func (q *SecurityQueries) ListTVDevices(ctx context.Context, userID int64) ([]models.TVDevice, error) {
	items := []models.TVDevice{}
	err := q.SelectContext(ctx, &items, `SELECT d.*, 'tv' AS session_type FROM tv_device d JOIN app_user u ON u.id=d.user_id WHERE d.user_id=? AND d.revoked_at IS NULL AND d.auth_version=u.auth_version ORDER BY d.last_seen_at DESC`, userID)
	return items, err
}
func (q *SecurityQueries) RevokeTVDevice(ctx context.Context, id, userID int64) error {
	r, err := q.ExecContext(ctx, `UPDATE tv_device SET revoked_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
func (q *SecurityQueries) TVDeviceCanAccessChannel(ctx context.Context, id int64, streamID string) (bool, error) {
	var device models.TVDevice
	err := q.GetContext(ctx, &device, `SELECT d.*, 'tv' AS session_type FROM tv_device d JOIN app_user u ON u.id=d.user_id WHERE d.id=? AND d.revoked_at IS NULL AND u.disabled_at IS NULL AND u.must_change_password=FALSE AND d.auth_version=u.auth_version`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	user, err := q.GetUserByID(ctx, device.UserID)
	if err != nil {
		return false, err
	}
	return q.UserCanAccessChannel(ctx, user.ID, user.Role, streamID)
}
func (q *SecurityQueries) RevokeUserTVDevices(ctx context.Context, userID int64) error {
	_, err := q.ExecContext(ctx, `UPDATE tv_device SET revoked_at=CURRENT_TIMESTAMP WHERE user_id=? AND revoked_at IS NULL`, userID)
	return err
}
func (q *SecurityQueries) TVDeviceActive(ctx context.Context, id int64) (bool, error) {
	var active bool
	err := q.GetContext(ctx, &active, `SELECT EXISTS(SELECT 1 FROM tv_device d JOIN app_user u ON u.id=d.user_id WHERE d.id=? AND d.revoked_at IS NULL AND u.disabled_at IS NULL AND u.must_change_password=FALSE AND d.auth_version=u.auth_version)`, id)
	return active, err
}

func (q *SecurityQueries) GetViewerPreferences(ctx context.Context, userID int64) (models.ViewerPreferences, error) {
	p := models.EmptyViewerPreferences()
	var raw string
	err := q.GetContext(ctx, &raw, `SELECT document FROM viewer_preferences WHERE user_id=?`, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(raw), &p)
	return p, err
}
func (q *SecurityQueries) SaveViewerPreferences(ctx context.Context, userID int64, p models.ViewerPreferences) (models.ViewerPreferences, error) {
	previous := p.Revision
	p.Revision++
	raw, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	err = q.WithTransactionContext(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO viewer_preferences(user_id,revision,document,updated_at) VALUES(?,0,'{}',CURRENT_TIMESTAMP)`, userID); err != nil {
			return err
		}
		r, err := tx.ExecContext(ctx, `UPDATE viewer_preferences SET revision=?,document=?,updated_at=CURRENT_TIMESTAMP WHERE user_id=? AND revision=?`, p.Revision, string(raw), userID, previous)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrPreferenceConflict
		}
		return nil
	})
	return p, err
}
