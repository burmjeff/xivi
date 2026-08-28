package models

import (
	"database/sql/driver"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
	"xivi/backend/platform/settings"
)

// Channel struct to describe Template Channel object.
type EpgProgramme struct {
	ID            int64         `db:"id" json:"id,omitempty" xml:"-"`
	Start         *Time         `db:"start" json:"stop,omitempty" xml:"start,attr,omitempty" validate:"required"`
	Stop          *Time         `db:"stop" json:"start,omitempty" xml:"stop,attr,omitempty" validate:"required"`
	Channel       string        `db:"channel" json:"channel,omitempty" xml:"channel,attr,omitempty" validate:"required,lte=50"`
	Title         Title         `db:"title" json:"title,omitempty" xml:"title,omitempty" validate:"required"`
	Subtitle      string        `db:"subtitle" json:"subtitle,omitempty" xml:"sub-title,omitempty" validate:"lte=255"`
	Desc          string        `db:"desc" json:"desc,omitempty" xml:"desc,omitempty" validate:"lte=1000"`
	Categories    StringArray   `db:"categories" json:"categories,omitempty" xml:"category,omitempty" validate:"lte=255"`
	Icon          Icon          `db:"icon" json:"icon,omitempty" xml:"icon,omitempty" validate:"lte=255"`
	Directors     StringArray   `db:"directors" json:"directors,omitempty" xml:"credits>director,omitempty" validate:"lte=255"`
	Presenters    StringArray   `db:"presenters" json:"presenters,omitempty" xml:"credits>presenter,omitempty" validate:"lte=255"`
	Producers     StringArray   `db:"producers" json:"producers,omitempty" xml:"credits>producer,omitempty" validate:"lte=255"`
	Actors        StringArray   `db:"actors" json:"actors,omitempty" xml:"credits>actor,omitempty" validate:"lte=255"`
	EpisodeNumber EpisodeNumber `db:"episodenumber" json:"episodenumber,omitempty" xml:"episode-num,omitempty" validate:"lte=255"`
	Rating        Rating        `db:"rating" json:"rating,omitempty" xml:"rating,omitempty" validate:"lte=255"`
	Video         Video         `db:"video" json:"video,omitempty" xml:"video,omitempty" validate:"lte=255"`
	Date          string        `db:"date" json:"date,omitempty" xml:"date,omitempty" validate:"lte=4"`
}

type Title struct {
	Lang  string `db:"title.lang" json:"lang,omitempty" xml:"lang,attr,omitempty"`
	Value string `db:"title.value" json:"value" xml:",chardata"`
}
type Icon struct {
	Src string `db:"icon.src" json:"src,omitempty" xml:"src,attr,omitempty"`
}
type EpisodeNumber struct {
	System string `db:"episodenumber.system" json:"system,omitempty" xml:"system,attr,omitempty"`
	Value  string `db:"episodenumber.value" json:"value,omitempty" xml:",chardata"`
}
type Rating struct {
	System string `db:"rating.system" json:"system,omitempty" xml:"system,attr,omitempty"`
	Value  string `db:"rating.value" json:"value,omitempty" xml:"value,omitempty"`
}
type Video struct {
	Quality string `db:"video.quality" json:"quality,omitempty" xml:"quality,omitempty"`
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

// Time that holds the time which is parsed from XML
type Time struct {
	time.Time
}

func (t *Time) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	return xml.Attr{
		Name:  name,
		Value: t.Format("20060102150405 -0700"),
	}, nil
}

func (t *Time) UnmarshalXMLAttr(attr xml.Attr) error {
	if strings.HasPrefix(attr.Value, "-") {
		return nil
	}

	t1, err := time.Parse("20060102150405 -0700", attr.Value)
	if err != nil {
		return err
	}

	*t = Time{t1}
	return nil
}

func (t Time) Value() (driver.Value, error) {
	var localTime *time.Location
	localTime, err := time.LoadLocation(settings.Current().Application.TZ)
	if err != nil {
		localTime = time.UTC
	}
	return t.In(localTime), nil
}

func (t *Time) Scan(value interface{}) error {
	if value == nil {
		t.Time = time.Time{}
		return nil
	}
	v, ok := value.(time.Time)
	if !ok {
		return fmt.Errorf("failed to scan Time: value %v is not of type time.Time", value)
	}

	// Convert the time to the application's configured timezone when reading from DB
	localTime, err := time.LoadLocation(settings.Current().Application.TZ)
	if err != nil {
		localTime = time.UTC
	}

	// Store the time in the configured timezone
	t.Time = v.In(localTime)
	return nil
}

// String returns a custom string representation of the time
// This ensures the time is in the application's configured timezone
// and formats it with the correct timezone offset
func (t Time) String() string {
	// Get the application's configured timezone
	localTime, err := time.LoadLocation(settings.Current().Application.TZ)
	if err != nil {
		localTime = time.UTC
	}

	// Ensure the time is in the configured timezone
	localizedTime := t.In(localTime)

	// Format the time with the timezone offset
	return localizedTime.Format("2006-01-02 15:04:05 -0700")
}
