package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/maehler/goblin"
)

type SensorService struct {
	db *DB
}

func NewSensorService(db *DB) *SensorService {
	return &SensorService{db}
}

func (s *SensorService) SensorById(ctx context.Context, id string) (*goblin.Sensor, error) {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	return sensorById(ctx, tx, id)
}

func (s *SensorService) CreateSensor(ctx context.Context, sensor *goblin.Sensor) error {
	_, err := s.db.db.Begin()
	if err != nil {
		return err
	}
	return nil
}

func (s *SensorService) DeleteSensor(ctx context.Context, id string) error {
	_, err := s.db.db.Begin()
	if err != nil {
		return err
	}
	return nil
}

func (s *SensorService) UpdateRoom(ctx context.Context, id string, roomId string) error {
	_, err := s.db.db.Begin()
	if err != nil {
		return err
	}
	return nil
}

func sensorById(ctx context.Context, tx *sql.Tx, id string) (*goblin.Sensor, error) {
	sensors, err := sensors(ctx, tx, goblin.SensorFilter{Id: &id})
	if err != nil {
		return nil, err
	}

	if len(sensors) == 0 {
		return nil, fmt.Errorf("user with id %s not found", id)
	}

	return sensors[0], nil
}

func sensors(ctx context.Context, tx *sql.Tx, filter goblin.SensorFilter) ([]*goblin.Sensor, error) {
	where, args := []string{}, []interface{}{}
	if v := filter.Id; v != nil {
		where = append(where, "id = ?")
		args = append(args, *v)
	}
	if v := filter.RoomId; v != nil {
		where = append(where, "room_id = ?")
		args = append(args, *v)
	}

	rows, err := tx.QueryContext(ctx, `SELECT
		id,
		sensor_type,
		room_id
	FROM sensors
	WHERE `+strings.Join(where, " AND ")+`
	ORDER BY ASC id;`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sensors := make([]*goblin.Sensor, 0)
	for rows.Next() {
		sensor := &goblin.Sensor{}
		err := rows.Scan(
			&sensor.Id,
			&sensor.SensorType,
			&sensor.RoomId,
		)
		if err != nil {
			return nil, err
		}
		sensors = append(sensors, sensor)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sensors, nil
}
