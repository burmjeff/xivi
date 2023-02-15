package models

// Channel struct to describe Template Group object.
type TemplateGroup struct {
	ID   int64  `db:"id" json:"id"`
	Name string `db:"name" json:"name" validate:"required,lte=255"`
}

type TemplateGroupItem struct {
	ID        int64 `db:"id" json:"id"`
	GroupId   int64 `db:"group_id" json:"group_id" validate:"required"`
	ChannelId int64 `db:"channel_id" json:"channel_id" validate:"required"`
	Order     int64 `db:"orderr" json:"orderr"`
}

type TemplateGroupCreateParam struct {
	Name string `json:"name" validate:"required,lte=255"`
}
