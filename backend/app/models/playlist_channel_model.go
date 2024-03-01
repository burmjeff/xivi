package models

import (
	"time"
)

// Channel struct to describe Playlist Channel object.
type PlaylistChannel struct {
	ID        int64     `db:"id" json:"id"`
	TvgID     *string   `db:"tvg_id" json:"tvg_id"`
	TvgName   string    `db:"tvg_name" json:"tvg_name" validate:"required,lte=255"`
	Logo      *string   `db:"tvg_logo" json:"tvg_logo"`
	Title     string    `db:"title" json:"title" validate:"lte=255"`
	Enabled   bool      `db:"enabled" json:"enabled"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type ChannelUrl struct {
	ID                int64     `db:"id" json:"id"`
	Url               string    `db:"url" json:"url" validate:"required,lte=255"`
	PlaylistID        int64     `db:"playlist_id" json:"playlist_id" validate:"required"`
	PlaylistChannelId int64     `db:"playlist_channel_id" json:"playlist_channel_id" validate:"required"`
	Order             int64     `db:"orderr" json:"orderr"`
	CreatedAt         time.Time `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time `db:"updated_at" json:"updated_at"`
}
