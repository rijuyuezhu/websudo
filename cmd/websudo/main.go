package main

import (
	"context"
	"fmt"
	"os"

	"websudo/internal/cli"
	"websudo/internal/client"
	"websudo/internal/config"
)

func main() {
	cfg := config.Default()

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	approvalClient := client.New("http://"+cfg.WebAddr, nil)
	exitCode, stdout, stderr, err := cli.Run(context.Background(), cli.Dependencies{ApprovalClient: approvalClient, Config: cfg}, os.Args[1:], cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if stdout != "" {
		fmt.Fprint(os.Stdout, stdout)
	}
	if stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}
	os.Exit(exitCode)
}
