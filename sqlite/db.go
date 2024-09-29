package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	dsn     string
	db      *sql.DB
	context context.Context
	cancel  context.CancelFunc
}

//go:embed migrations/*.sql
var migrationFS embed.FS

func NewDatabase(dsn string) *DB {
	db := &DB{
		dsn: dsn,
	}

	db.context, db.cancel = context.WithCancel(context.Background())
	return db
}

func (db *DB) Open() error {
	log.Printf("Connecting to database %s", db.dsn)
	var err error
	if db.db, err = sql.Open("sqlite3", db.dsn); err != nil {
		return err
	}

	if _, err := db.db.Exec(`PRAGMA journal_mode = wal`); err != nil {
		return fmt.Errorf("enable wal: %s", err)
	}

	if _, err := db.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %s", err)
	}

	if err := db.migrate(); err != nil {
		return err
	}

	return nil
}

func (db DB) migrate() error {
	if _, err := db.db.Exec(`CREATE TABLE IF NOT EXISTS migrations (name TEXT PRIMARY KEY);`); err != nil {
		return fmt.Errorf("Failed to create migrations table: %s", err.Error())
	}

	fnames, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(fnames)

	for _, fname := range fnames {
		if err := db.migrateFile(fname); err != nil {
			return err
		}
	}

	return nil
}

func (db DB) migrateFile(fname string) error {
	log.Printf("Running migration for %s", fname)
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}

	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM migrations WHERE name = ?`, fname).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return nil
	}

	if buf, err := fs.ReadFile(migrationFS, fname); err != nil {
		return err
	} else if _, err := tx.Exec(string(buf)); err != nil {
		return err
	}

	if _, err := tx.Exec(`INSERT INTO migrations (name) VALUES (?)`, fname); err != nil {
		return err
	}

	return tx.Commit()
}
