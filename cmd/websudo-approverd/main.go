package main

import (
	"log"
	"net/http"

	"websudo/internal/approverd"
	"websudo/internal/config"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	srv := approverd.NewServer(approverd.Dependencies{Config: cfg})
	listener, err := approverd.ListenAskpassIPC(config.AskpassSocketPath())
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()

	errCh := make(chan error, 2)
	go func() {
		errCh <- srv.ServeAskpassIPC(listener)
	}()
	go func() {
		errCh <- http.ListenAndServe(cfg.WebAddr, srv.Routes())
	}()
	return <-errCh
}
