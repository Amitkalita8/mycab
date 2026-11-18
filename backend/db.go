package main

import (
	"database/sql"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo / gcc needed)
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	phone      TEXT NOT NULL UNIQUE,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS drivers (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	name          TEXT NOT NULL,
	phone         TEXT NOT NULL UNIQUE,
	vehicle_model TEXT NOT NULL,
	vehicle_plate TEXT NOT NULL UNIQUE,
	available     INTEGER NOT NULL DEFAULT 1,
	created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS rides (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id      INTEGER NOT NULL REFERENCES users(id),
	driver_id    INTEGER REFERENCES drivers(id),
	pickup       TEXT NOT NULL,
	dropoff      TEXT NOT NULL,
	pickup_lat   REAL NOT NULL,
	pickup_lng   REAL NOT NULL,
	dropoff_lat  REAL NOT NULL,
	dropoff_lng  REAL NOT NULL,
	distance_km  REAL NOT NULL,
	fare         REAL NOT NULL,
	status       TEXT NOT NULL DEFAULT 'requested',
	created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_rides_user   ON rides(user_id);
CREATE INDEX IF NOT EXISTS idx_rides_driver ON rides(driver_id);
CREATE INDEX IF NOT EXISTS idx_rides_status ON rides(status);
`

func openDB(path string) (*sql.DB, error) {
	// busy_timeout + WAL avoid "database is locked" under concurrent requests.
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: single writer; keeps things simple and safe
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
