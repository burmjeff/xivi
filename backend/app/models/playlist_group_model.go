package models

// Channel struct to describe Playlist Group object.
type PlaylistGroup struct {
	ID         int64  `db:"id" json:"id"`
	Name       string `db:"name" json:"name" validate:"required,lte=255"`
	PlaylistId int64  `db:"playlist_id" json:"playlist_id"`
	Enabled    bool   `db:"enabled" json:"enabled"`
}
