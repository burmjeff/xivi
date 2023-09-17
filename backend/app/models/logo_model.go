package models

// Channel struct to describe Template Channel object.
type Logo struct {
	ID  int64  `db:"id" json:"id"`
	Img string `db:"img" json:"img"`
}
