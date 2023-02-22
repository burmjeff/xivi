package models

import (
	"database/sql/driver"
	"fmt"
	utils "xivi/backend/pkg/dbutils"
)

// Channel struct to describe Template object.
type TemplateChannelVector struct {
	ID        int64  `db:"id" json:"id"`
	Name      string `db:"name" json:"name" validate:"required,lte=255"`
	ChannelId int64  `db:"channel_id" json:"channel_id" validate:"required"`
}

type ChannelVector struct {
	ID     int64       `db:"id" json:"id"`
	Name   string      `db:"name" json:"name" validate:"required,lte=255"`
	Vector VectorFloat `db:"vector" json:"vector"`
}

type VectorFloat []float64

func (a *VectorFloat) Scan(value interface{}) error {
	if value == nil {
		*a = VectorFloat{}
		return nil
	}
	byteValue, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("unexpected type for VectorFloat: %T", value)
	}
	*a = utils.BytesToVector(byteValue)
	return nil
}

func (a VectorFloat) Value() (driver.Value, error) {
	return utils.VectorToBytes(a), nil
}
