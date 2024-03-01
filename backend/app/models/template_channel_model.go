package models

// Channel struct to describe Template Channel object.
type TemplateChannel struct {
	ID     int64   `db:"id" json:"id"`
	Name   string  `db:"name" json:"name" validate:"required,lte=255"`
	TvgID  *string `db:"tvgid" json:"tvgid"`
	LogoId int64   `db:"logoid" json:"logoid"`
	Uuid   string  `db:"uuid" json:"uuid" validate:"required"`
}

type TemplateChannelItem struct {
	ID                int64 `db:"id" json:"id"`
	ChannelId         int64 `db:"channel_id" json:"channel_id" validate:"required"`
	PlaylistChannelId int64 `db:"playlist_channel_id" json:"playlist_channel_id" validate:"required"`
	Order             int64 `db:"orderr" json:"orderr"`
}

type TemplateChannelLogo struct {
	TemplateChannel
	Logo string `db:"logo" json:"logo"`
}
