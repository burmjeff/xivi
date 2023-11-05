package models

// Channel struct to describe Template Channel object.
type Logo struct {
	ID  int64  `db:"id" json:"id"`
	Img string `db:"img" json:"img"`
}

type LogoUpload struct {
	Type  string `json:"type"`
	Image string `json:"image"`
}

type LogoCreateParam struct {
	Img string `json:"img" validate:"required,lte=255"`
}
