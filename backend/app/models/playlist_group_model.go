package models

// Channel struct to describe Playlist Group object.
type PlaylistGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name" validate:"required,lte=255"`
}

type PlaylistGroupItem struct {
	PlaylistId int64 `db:"playlist_id" json:"playlist_id" validate:"required"`
	GroupId    int64 `db:"group_id" json:"group_id" validate:"required"`
}

type PlaylistGroupChannel struct {
	PlaylistId int64 `db:"playlist_id" json:"playlist_id" validate:"required"`
	GroupId    int64 `db:"group_id" json:"group_id" validate:"required"`
	ChannelId  int64 `db:"channel_id" json:"channel_id" validate:"required"`
}
