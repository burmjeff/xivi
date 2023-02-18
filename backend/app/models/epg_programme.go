package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// Channel struct to describe Template Channel object.
type EpgProgramme struct {
	ID            int64         `db:"id" json:"id"`
	Start         time.Time     `db:"start" json:"start" validate:"required"`
	Stop          time.Time     `db:"stop" json:"stop" validate:"required"`
	StartString   string        `xml:"start,attr"`
	StopString    string        `xml:"stop,attr"`
	Channel       string        `db:"channel" json:"channel" xml:"channel,attr" validate:"required,lte=20"`
	Title         Title         `db:"title" json:"title" xml:"title"`
	Subtitle      string        `db:"subtitle" json:"subtitle" xml:"sub-title" validate:"lte=255"`
	Desc          string        `db:"desc" json:"desc" xml:"desc" validate:"lte=1000"`
	Categories    StringArray   `db:"categories" json:"categories" xml:"category"`
	Icon          Icon          `db:"icon" json:"icon" xml:"icon"`
	Directors     StringArray   `db:"directors" json:"directors" xml:"credits>director"`
	Presenters    StringArray   `db:"presenters" json:"presenters" xml:"credits>presenter"`
	Producers     StringArray   `db:"producers" json:"producers" xml:"credits>producer"`
	Actors        StringArray   `db:"actors" json:"actors" xml:"credits>actor"`
	EpisodeNumber EpisodeNumber `db:"episodenumber" json:"episodenumber" xml:"episode-num"`
	Rating        Rating        `db:"rating" json:"rating" xml:"rating"`
	Video         Video         `db:"video" json:"video" xml:"video"`
	Date          string        `db:"date" json:"date" xml:"date" validate:"lte=4"`
}

type Title struct {
	Lang  string `db:"lang" json:"lang" xml:"lang,attr"`
	Value string `db:"value" json:"value" xml:",chardata"`
}
type Icon struct {
	Src string `db:"src" json:"src" xml:"src,attr"`
}
type EpisodeNumber struct {
	System string `db:"system" json:"system" xml:"system,attr"`
	Value  string `db:"value" json:"value" xml:",chardata"`
}
type Rating struct {
	System string `db:"system" json:"system" xml:"system,attr"`
	Value  string `db:"value" json:"value" xml:"value"`
}
type Video struct {
	Quality string `db:"quality" json:"quality" xml:"quality"`
}

type StringArray []string

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = StringArray{}
		return nil
	}
	strValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("unexpected type for StringArray: %T", value)
	}
	*a = strings.Split(strValue, ", ")
	return nil
}

func (a StringArray) Value() (driver.Value, error) {
	return strings.Join(a, ","), nil
}
