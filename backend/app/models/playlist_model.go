package models

import "time"

// Channel struct to describe Playlist object.
type Playlist struct {
	ID                int64     `db:"id" json:"id"`
	Name              string    `db:"name" json:"name" validate:"required,lte=255"`
	URL               string    `db:"url" json:"url" validate:"required,lte=255"`
	URLCipher         []byte    `db:"url_cipher" json:"-"`
	ConnectionLimit   int       `db:"connection_limit" json:"connection_limit" validate:"gte=1,lte=100"`
	ActiveConnections int       `db:"-" json:"active_connections"`
	CreatedAt         time.Time `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time `db:"updated_at" json:"updated_at"`
}
