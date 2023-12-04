package models

// Channel struct to describe Template Group object.
type TemplateGroup struct {
	ID            int64  `db:"id" json:"id"`
	Name          string `db:"name" json:"name" validate:"required,lte=255"`
	Dynamic       bool   `db:"dynamic" json:"dynamic"`
	PlaylistGroup int64  `db:"playlistgroup" json:"playlistgroup"`
}

type TemplateGroupItem struct {
	TemplateId int64 `db:"template_id" json:"template_id" validate:"required"`
	GroupId    int64 `db:"group_id" json:"group_id" validate:"required"`
	Order      int64 `db:"orderr" json:"orderr"`
}

type TemplateGroupChannel struct {
	GroupId   int64 `db:"group_id" json:"group_id" validate:"required"`
	ChannelId int64 `db:"channel_id" json:"channel_id" validate:"required"`
	Order     int64 `db:"orderr" json:"orderr"`
}

type TemplateGroupCreateParam struct {
	Name          string `db:"name" json:"name" validate:"required,lte=255"`
	Dynamic       bool   `db:"dynamic" json:"dynamic" validate:"required"`
	PlaylistGroup int64  `db:"playlistgroup" json:"playlistgroup"`
}
