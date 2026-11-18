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
	db, err := openDB(getenv("DB_PATH", "mycab.db"))
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	addr := ":" + getenv("PORT", "8080")
	srv := &Server{db: db}
	log.Printf("mycab API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, srv.routes()))
}
