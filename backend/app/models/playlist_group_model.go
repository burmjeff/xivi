package models

// Channel struct to describe Playlist Group object.
type PlaylistGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name" validate:"required,lte=255"`
}
