package models

import "time"

// Channel struct to describe Template Channel object.
type EpgProgramme struct {
	ID            int64     `db:"id" json:"id"`
	Start         time.Time `db:"start" json:"start" validate:"required"`
	Stop          time.Time `db:"stop" json:"stop" validate:"required"`
	Channel       string    `db:"channel" json:"channel" validate:"required,lte=20"`
	Title         string    `db:"title" json:"title" validate:"lte=255"`
	Subtitle      string    `db:"subtitle" json:"subtitle" validate:"lte=255"`
	Desc          string    `db:"desc" json:"desc" validate:"lte=500"`
	Credits       string    `db:"credits" json:"credits"`
	Date          string    `db:"date" json:"date" validate:"lte=4"`
	Category_1    string    `db:"category_1" json:"category_1" validate:"lte=20"`
	Category_2    string    `db:"category_2" json:"category_2" validate:"lte=20"`
	Category_3    string    `db:"category_3" json:"category_3" validate:"lte=20"`
	Category_4    string    `db:"category_4" json:"category_4" validate:"lte=20"`
	Icon          string    `db:"icon" json:"icon" validate:"lte=255"`
	Episodesystem string    `db:"episodesystem" json:"episodesystem" validate:"lte=20"`
	Episodenum    string    `db:"episodenum" json:"episodenum" validate:"lte=20"`
	Ratingsystem  string    `db:"ratingsystem" json:"ratingsystem" validate:"lte=20"`
	Ratingvalue   string    `db:"ratingvalue" json:"ratingvalue" validate:"lte=20"`
	Lang          string    `db:"lang" json:"lang" validate:"lte=20"`
}
