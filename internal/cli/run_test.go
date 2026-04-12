package cli

import (
	"context"
	"testing"
	"time"

	clientpkg "websudo/internal/client"
	"websudo/internal/model"
)

type fakeApprovalClient struct {
	created model.Request
	result  clientpkg.Request
	err     error
}

func (f *fakeApprovalClient) CreateAndWait(ctx context.Context, req model.Request) (clientpkg.Request, error) {
	f.created = req
	return f.result, f.err
}

func TestRunFreezesResolvedCommandAndReturnsExitCode(t *testing.T) {
	client := &fakeApprovalClient{
		result: clientpkg.Request{
			ID:        "req-result",
			CreatedAt: time.Date(2026, 4, 12, 6, 0, 0, 0, time.UTC),
			Command:   model.Command{ResolvedPath: "/usr/bin/printf", Argv: []string{"/usr/bin/printf", "hello"}, Cwd: "/tmp"},
			Status:    model.StatusFailed,
			Result:    &clientpkg.Result{ExitCode: 7, Stdout: "ok", Stderr: "bad"},
		},
	}

	exitCode, stdout, stderr, err := Run(context.Background(), client, []string{"/usr/bin/printf", "hello"}, "/tmp")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := client.created.Command().ResolvedPath; got != "/usr/bin/printf" {
		t.Fatalf("resolved path = %q, want %q", got, "/usr/bin/printf")
	}
	if got := client.created.Command().Argv; len(got) != 2 || got[0] != "/usr/bin/printf" || got[1] != "hello" {
		t.Fatalf("argv = %#v, want frozen argv", got)
	}
	if exitCode != 7 || stdout != "ok" || stderr != "bad" {
		t.Fatalf("result = (%d, %q, %q), want (7, %q, %q)", exitCode, stdout, stderr, "ok", "bad")
	}
	if client.created.Status() != model.StatusPending {
		t.Fatalf("created status = %q, want %q", client.created.Status(), model.StatusPending)
	}
	if client.created.CreatedAt().IsZero() {
		t.Fatal("created request timestamp was not set")
	}
	if client.created.ID() == "" {
		t.Fatal("created request id was not set")
	}
}

func TestRunReturnsErrorWhenCommandMissing(t *testing.T) {
	_, _, _, err := Run(context.Background(), &fakeApprovalClient{}, nil, "/tmp")
	if err == nil {
		t.Fatal("Run() error = nil, want missing command error")
	}
}
