package goblin

import (
	"context"
)

type Sensor struct {
	Id         string
	SensorType string
	RoomId     string
}

type SensorService interface {
	SensorById(context.Context, string) (*Sensor, error)
	CreateSensor(context.Context, *Sensor) error
	DeleteSensor(context.Context, string) error
	UpdateRoom(context.Context, string, string) error
}

type SensorFilter struct {
	Id     *string
	RoomId *string
}
