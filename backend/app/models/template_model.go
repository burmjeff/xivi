package models

// Channel struct to describe Template object.
type Template struct {
	ID   int64  `db:"id" json:"id"`
	Name string `db:"name" json:"name" validate:"required,lte=255"`
}

type TemplateItem struct {
	ID         int64 `db:"id" json:"id"`
	TemplateId int64 `db:"template_id" json:"template_id" validate:"required"`
	GroupId    int64 `db:"group_id" json:"group_id" validate:"required"`
	Order      int64 `db:"orderr" json:"orderr"`
}

type TemplateCreateParam struct {
	Name string `json:"name" validate:"required,lte=255"`
}
