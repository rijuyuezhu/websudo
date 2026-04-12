package main

import (
	"log"
	"net/http"

	"websudo/internal/approverd"
	"websudo/internal/config"
	"websudo/internal/store"
)

func main() {
	cfg := config.Default()
	sqliteStore, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer sqliteStore.Close()

	srv := approverd.NewServer(approverd.Dependencies{
		Config: cfg,
		Store:  approverd.NewSQLiteStore(sqliteStore),
	})
	log.Fatal(http.ListenAndServe(cfg.WebAddr, srv.Routes()))
}
