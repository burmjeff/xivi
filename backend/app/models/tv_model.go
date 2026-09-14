package models

import "time"

type TVDevice struct {
	ID               int64      `db:"id" json:"id"`
	UserID           int64      `db:"user_id" json:"user_id"`
	AuthVersion      int64      `db:"auth_version" json:"-"`
	DeviceName       string     `db:"device_name" json:"device_name"`
	KeyThumbprint    string     `db:"key_thumbprint" json:"-"`
	RefreshTokenHash []byte     `db:"refresh_token_hash" json:"-"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	LastSeenAt       time.Time  `db:"last_seen_at" json:"last_seen_at"`
	RevokedAt        *time.Time `db:"revoked_at" json:"revoked_at"`
	ClientIP         string     `db:"client_ip" json:"client_ip"`
	SessionType      string     `db:"session_type" json:"client_type"`
}

type TVPairing struct {
	ID                 int64      `db:"id"`
	DeviceCodeHash     []byte     `db:"device_code_hash"`
	UserCodeHash       []byte     `db:"user_code_hash"`
	DeviceName         string     `db:"device_name"`
	KeyThumbprint      string     `db:"key_thumbprint"`
	CreatedAt          time.Time  `db:"created_at"`
	ExpiresAt          time.Time  `db:"expires_at"`
	Decision           string     `db:"decision"`
	UserID             *int64     `db:"user_id"`
	AuthVersion        *int64     `db:"auth_version"`
	DeviceID           *int64     `db:"device_id"`
	RefreshTokenCipher []byte     `db:"refresh_token_cipher"`
	LastPolledAt       *time.Time `db:"last_polled_at"`
}

type FavoriteList struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Channels []string `json:"channels"`
}

type ViewerPreferences struct {
	Revision  int64          `json:"revision"`
	Favorites []FavoriteList `json:"favorites"`
	Hidden    []string       `json:"hidden"`
	Order     []string       `json:"order"`
}

func EmptyViewerPreferences() ViewerPreferences {
	return ViewerPreferences{Favorites: []FavoriteList{}, Hidden: []string{}, Order: []string{}}
}
