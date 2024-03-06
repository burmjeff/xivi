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
	GroupId   int64     `db:"group_id" json:"group_id" validate:"required"`
	Enabled   bool      `db:"enabled" json:"enabled"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type ChannelUrl struct {
	ID        int64     `db:"id" json:"id"`
	Url       string    `db:"url" json:"url" validate:"required,lte=255"`
	ChannelId int64     `db:"channel_id" json:"channel_id" validate:"required"`
	Order     int64     `db:"orderr" json:"orderr"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
