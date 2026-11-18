package main

import (
	"database/sql"
	"strings"

	_ "github.com/tursodatabase/libsql-client-go/libsql" // hosted Turso database (pure Go)
	_ "modernc.org/sqlite"                               // local SQLite file (pure Go, no cgo / gcc needed)
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	name          TEXT NOT NULL,
	phone         TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS drivers (
	id                  INTEGER PRIMARY KEY AUTOINCREMENT,
	name                TEXT NOT NULL,
	phone               TEXT NOT NULL UNIQUE,
	password_hash       TEXT NOT NULL,
	vehicle_model       TEXT NOT NULL,
	vehicle_plate       TEXT NOT NULL UNIQUE,
	available           INTEGER NOT NULL DEFAULT 0,
	lat                 REAL,
	lng                 REAL,
	location_updated_at DATETIME,
	rating_sum          INTEGER NOT NULL DEFAULT 0,
	rating_count        INTEGER NOT NULL DEFAULT 0,
	created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	role       TEXT NOT NULL,
	account_id INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS rides (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id        INTEGER NOT NULL REFERENCES users(id),
	driver_id      INTEGER REFERENCES drivers(id),
	pickup         TEXT NOT NULL,
	dropoff        TEXT NOT NULL,
	pickup_lat     REAL NOT NULL,
	pickup_lng     REAL NOT NULL,
	dropoff_lat    REAL NOT NULL,
	dropoff_lng    REAL NOT NULL,
	distance_km    REAL NOT NULL,
	fare           REAL NOT NULL,
	status         TEXT NOT NULL DEFAULT 'requested',
	payment_method TEXT NOT NULL DEFAULT 'cash',
	payment_status TEXT NOT NULL DEFAULT 'pending',
	rating         INTEGER,
	review         TEXT,
	created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_rides_user   ON rides(user_id);
CREATE INDEX IF NOT EXISTS idx_rides_driver ON rides(driver_id);
CREATE INDEX IF NOT EXISTS idx_rides_status ON rides(status);
`

// openDB opens the database. If target is a libsql:// (Turso) URL it connects to that hosted
// database, otherwise target is treated as a local SQLite file path.
func openDB(target string) (*sql.DB, error) {
	var db *sql.DB
	var err error
	if strings.HasPrefix(target, "libsql://") || strings.HasPrefix(target, "https://") {
		db, err = sql.Open("libsql", target)
	} else {
		// busy_timeout + WAL avoid "database is locked" under concurrent requests.
		db, err = sql.Open("sqlite", "file:"+target+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	}
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single writer; also makes each request's transaction serial
	// Run statements one at a time: remote drivers don't all accept multi-statement Exec.
	for _, stmt := range strings.Split(schema, ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}
