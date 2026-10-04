package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"websudo/internal/askpass"
	"websudo/internal/config"
	"websudo/internal/processauth"
)

func main() {
	prompt := "Password:"
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	baseURL := "http://" + cfg.WebAddr
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ApprovalTimeout)
	defer cancel()

	approverdExecutable, err := processauth.SiblingExecutable("websudo-approverd")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	client := askpass.New(config.AskpassSocketPath(), approverdExecutable)
	req, err := client.Create(ctx, prompt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%s/askpass/%s\n", baseURL, url.PathEscape(req.ID))

	password, err := client.WaitForPassword(ctx, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := fmt.Fprintln(os.Stdout, password); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
