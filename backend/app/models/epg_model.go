package models

import (
	"encoding/xml"
	"time"
)

// Channel struct to describe Playlist object.
type Epg struct {
	ID        int64     `db:"id" json:"id"`
	Name      string    `db:"name" json:"name" validate:"required,lte=255"`
	URL       string    `db:"url" json:"url" validate:"required,lte=255"`
	URLCipher []byte    `db:"url_cipher" json:"-"`
	Order     int64     `db:"orderr" json:"orderr"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type EpgAddParam struct {
	Name string `json:"name" validate:"required,lte=255"`
	URL  string `json:"url" validate:"required,lte=255"`
}

type EpgItem struct {
	XMLName        xml.Name       `xml:"tv"`
	GeneratorInfo  string         `xml:"generator-info-name,attr"`
	SourceInfoName string         `xml:"source-info-name,attr"`
	Channels       []EpgChannel   `xml:"channel"`
	Programmes     []EpgProgramme `xml:"programme"`
}
