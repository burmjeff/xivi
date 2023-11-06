package models

// Channel struct to describe Template Channel object.
type Logo struct {
	ID   int64  `db:"id" json:"id"`
	Uuid string `db:"uuid" json:"uuid"`
}

type LogoPath struct {
	ID    int64  `json:"id"`
	Image string `json:"image"`
}

type LogoCreateParam struct {
	Uuid string `json:"uuid" validate:"required,lte=255"`
}
