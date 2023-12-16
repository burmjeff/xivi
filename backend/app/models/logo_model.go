package models

// Channel struct to describe Template Channel object.
type Logo struct {
	ID    int64  `db:"id" json:"id"`
	Name  string `db:"name" json:"name"`
	Image string `json:"image"`
}
