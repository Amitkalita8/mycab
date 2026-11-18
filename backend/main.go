package main

import (
	"log"
	"net/http"
	"os"
	"strings"
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
	dbURL := strings.Trim(os.Getenv("DATABASE_URL"), " \t\r\n\"'") // tolerate pasted quotes/spaces
	target := getenv("DB_PATH", "mycab.db")
	switch {
	case dbURL != "":
		if !strings.HasPrefix(dbURL, "libsql://") && !strings.HasPrefix(dbURL, "https://") {
			// never print the value: it contains the auth token
			log.Fatal("DATABASE_URL must start with libsql:// (format: libsql://<db>.turso.io?authToken=<token>)")
		}
		target = dbURL
	case os.Getenv("RENDER") != "":
		log.Fatal("DATABASE_URL is not set: add it under Environment in the Render dashboard")
	}

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
