package goblin

import "context"

type Room struct {
	Id   string
	Name string
}

type RoomService interface {
	RoomById(context.Context, string) (*Room, error)
	CreateRoom(context.Context, *Room) error
	DeleteRoom(context.Context, string) error
}

type RoomFilter struct {
	Id   *string
	Name *string
}
