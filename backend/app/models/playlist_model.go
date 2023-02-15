package models

import "time"

// Channel struct to describe Playlist object.
type Playlist struct {
	ID        int64     `db:"id" json:"id"`
	Name      string    `db:"name" json:"name" validate:"required,lte=255"`
	URL       string    `db:"url" json:"url" validate:"required,lte=255"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type PlaylistCreateParam struct {
	Name string `json:"name" validate:"required,lte=255"`
	URL  string `json:"url" validate:"required,lte=255"`
}
