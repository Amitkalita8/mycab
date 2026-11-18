package main

import (
	"log"
	"net/http"
	"os"
)

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	// DATABASE_URL (libsql://...?authToken=...) selects a hosted Turso database;
	// otherwise a local SQLite file at DB_PATH is used.
	target := getenv("DATABASE_URL", getenv("DB_PATH", "mycab.db"))
	db, err := openDB(target)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	srv := &Server{db: db, staticDir: os.Getenv("STATIC_DIR"), allowOrigin: getenv("ALLOWED_ORIGIN", "*")}
	addr := ":" + getenv("PORT", "8080")
	log.Printf("mycab API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.routes()))
}
