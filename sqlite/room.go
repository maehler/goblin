package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/maehler/goblin"
)

type RoomService struct {
	db *DB
}

func NewRoomService(db *DB) *RoomService {
	return &RoomService{db}
}

func (s *RoomService) RoomById(ctx context.Context, id string) (*goblin.Room, error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	return roomById(ctx, tx, id)
}

func (s *RoomService) CreateRoom(ctx context.Context, room *goblin.Room) error {
	panic("not implemented")
}

func (s *RoomService) DeleteRoom(ctx context.Context, id string) error {
	panic("not implemented")
}

func roomById(ctx context.Context, tx *sql.Tx, id string) (*goblin.Room, error) {
	rooms, err := rooms(ctx, tx, goblin.RoomFilter{Id: &id})
	if err != nil {
		return nil, err
	}

	if len(rooms) == 0 {
		return nil, fmt.Errorf("room with id %s not found", id)
	}

	return rooms[0], nil
}

func rooms(ctx context.Context, tx *sql.Tx, filter goblin.RoomFilter) ([]*goblin.Room, error) {
	where, args := []string{}, []interface{}{}
	if v := filter.Id; v != nil {
		where = append(where, "id = ?")
		args = append(args, *v)
	}
	if v := filter.Name; v != nil {
		where = append(where, "name = ?")
		args = append(args, *v)
	}

	rows, err := tx.QueryContext(ctx, "SELECT id, name FROM room WHERE "+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rooms := make([]*goblin.Room, 0)
	for rows.Next() {
		room := &goblin.Room{}
		err := rows.Scan(
			&room.Id,
			&room.Name,
		)
		if err != nil {
			return nil, err
		}
		rooms = append(rooms, room)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return rooms, nil
}
