package models

// Channel struct to describe Template object.
type Template struct {
	ID   int64  `db:"id" json:"id"`
	Name string `db:"name" json:"name" validate:"required,lte=255"`
}

type TemplateCreateParam struct {
	Name string `json:"name" validate:"required,lte=255"`
}
