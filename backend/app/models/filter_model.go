package models

// Channel struct to describe Template Channel object.
type ChannelFilter struct {
	ID      int64  `db:"id" json:"id" xml:"-"`
	OldName string `db:"oldname" json:"oldname" validate:"required,lte=255"`
	NewName string `db:"newname" json:"newname" validate:"lte=255"`
}
