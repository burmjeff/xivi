package models

// Channel struct to describe Playlist object.
type Epg struct {
	ID   int64  `db:"id" json:"id"`
	Name string `db:"name" json:"name" validate:"required,lte=255"`
	URL  string `db:"url" json:"url" validate:"required,lte=255"`
}

type EpgCreateParam struct {
	Name string `json:"name" validate:"required,lte=255"`
	URL  string `json:"url" validate:"required,lte=255"`
}
