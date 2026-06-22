package main

import (
	"log"
	"net/http"

	"websudo/internal/approverd"
	"websudo/internal/config"
)

func main() {
	cfg := config.Default()
	srv := approverd.NewServer(approverd.Dependencies{Config: cfg})
	log.Fatal(http.ListenAndServe(cfg.WebAddr, srv.Routes()))
}
