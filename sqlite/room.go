package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/maehler/goblin"
)

type RoomService struct {
	db *DB
}

func NewRoomService(db *DB) *RoomService {
	return &RoomService{db}
}

func (s *RoomService) RoomById(ctx context.Context, id string) (room *goblin.Room, err error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			err = tx.Rollback()
		}
	}()

	return roomById(ctx, tx, id)
}

func (s *RoomService) CreateRoom(ctx context.Context, room *goblin.Room) (err error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			slog.Debug("rolling back room creation")
			err = tx.Rollback()
		}
	}()

	if err := createRoom(ctx, tx, room); err != nil {
		return err
	}

	return tx.Commit()
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
	where, args := []string{}, []any{}
	if v := filter.Id; v != nil {
		where = append(where, "id = ?")
		args = append(args, *v)
	}
	if v := filter.Name; v != nil {
		where = append(where, "name = ?")
		args = append(args, *v)
	}

	rows, err := tx.QueryContext(ctx, "SELECT id, name FROM rooms WHERE "+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("failed to close rows", "error", err)
		}
	}()

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

func createRoom(ctx context.Context, tx *sql.Tx, room *goblin.Room) error {
	slog.Debug("upserting room", "name", room.Name, "id", room.Id)
	stmt := `INSERT OR REPLACE INTO rooms (id, name) VALUES (?, ?)`
	_, err := tx.ExecContext(ctx, stmt, room.Id, room.Name, room.Id)
	return err
}
