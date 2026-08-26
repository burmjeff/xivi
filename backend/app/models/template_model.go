package models

// Channel struct to describe Template object.
type Template struct {
	ID                    int64  `db:"id" json:"id"`
	Name                  string `db:"name" json:"name" validate:"required,lte=255"`
	VirtualTunerEnabled   bool   `db:"virtual_tuner_enabled" json:"virtual_tuner_enabled"`
	FillMissingGuideSlots bool   `db:"fill_missing_guide_slots" json:"fill_missing_guide_slots"`
}

type TemplateCreateParam struct {
	Name string `json:"name" validate:"required,lte=255"`
}
