package main

import (
	"context"
	"fmt"
	"os"

	"websudo/internal/config"
	"websudo/internal/sudoexec"
)

func main() {
	cfg := config.Default()
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	exitCode, err := sudoexec.Run(context.Background(), sudoexec.Dependencies{
		Config: cfg,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, os.Args[1:], cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(exitCode)
}
