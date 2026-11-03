package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	Conn *sql.DB
}

func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}

	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	if _, err := conn.Exec(`
		PRAGMA foreign_keys = ON;
		PRAGMA journal_mode = WAL;
		PRAGMA busy_timeout = 5000;
		PRAGMA synchronous = NORMAL;
	`); err != nil {
		conn.Close()
		return nil, err
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, err
	}

	db := &DB{Conn: conn}

	if err := db.Migrate(); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.Conn.Close()
}

func (db *DB) Migrate() error {
	_, err := db.Conn.Exec(schema)

	if err != nil {
		return fmt.Errorf("database migration: %w", err)
	}

	return nil
}
