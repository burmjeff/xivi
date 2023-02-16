package models

// Channel struct to describe Template Channel object.
type EpgChannel struct {
	ID        int64  `db:"id" json:"id"`
	ChannelId string `db:"channelid" json:"channelid" validate:"required,lte=255"`
	Name      string `db:"name" json:"name" validate:"lte=255"`
	Icon      string `db:"icon" json:"icon" validate:"lte=255"`
}

type EpgChannelItem struct {
	ID             int64 `db:"id" json:"id"`
	EpgChannelId   int64 `db:"epg_channel_id" json:"epg_channel_id" validate:"required"`
	EpgProgrammeId int64 `db:"epg_programme_id" json:"epg_programme_id" validate:"required"`
}
