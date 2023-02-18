package models

// Channel struct to describe Template Channel object.
type EpgChannel struct {
	ID          int64  `db:"id" json:"id"`
	ChannelId   string `db:"channelid" json:"channelid" xml:"id,attr" validate:"required,lte=255"`
	DisplayName string `db:"displayname" json:"displayname" xml:"display-name" validate:"lte=255"`
	Icon        string `db:"icon" json:"-" xml:"-"`
	IconSrc     struct {
		Src string `json:"icon" xml:"src,attr"`
	} `xml:"icon"`
}

type EpgChannelItem struct {
	ID             int64 `db:"id" json:"id"`
	EpgChannelId   int64 `db:"epg_channel_id" json:"epg_channel_id" validate:"required"`
	EpgProgrammeId int64 `db:"epg_programme_id" json:"epg_programme_id" validate:"required"`
}
