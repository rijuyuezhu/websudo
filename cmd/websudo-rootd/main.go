package main

import (
	"log"

	"websudo/internal/config"
	"websudo/internal/rootd"
)

func main() {
	if err := rootd.ListenAndServe(config.Default().RootSocketPath); err != nil {
		log.Fatal(err)
	}
}
