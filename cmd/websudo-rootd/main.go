package main

import (
	"log"

	"websudo/internal/config"
	"websudo/internal/rootd"
)

func main() {
	cfg := config.Default()
	if err := rootd.ListenAndServe(cfg.RootSocketPath, cfg.RootAllowedUID); err != nil {
		log.Fatal(err)
	}
}
