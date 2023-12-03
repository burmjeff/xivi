package models

type RegexFilter struct {
	ID    int64  `db:"id" json:"id"`
	Name  string `db:"name" json:"name" validate:"required,lte=255"`
	Regex string `db:"regex" json:"regex" validate:"lte=255"`
}

type GroupFilter struct {
	ID       int64  `db:"id" json:"id"`
	GroupId  string `db:"group_id" json:"group_id" validate:"required,lte=255"`
	FilterId string `db:"filter_id" json:"filter_id" validate:"required,lte=255"`
	Type     bool   `db:"type" json:"type" validate:"required"`
}
