package cli

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	clientpkg "websudo/internal/client"
	"websudo/internal/config"
	"websudo/internal/model"
)

type fakeApprovalClient struct {
	created model.Request
	result  clientpkg.Request
	err     error
}

type fakeExecutor struct {
	command model.Command
	result  model.Result
	err     error
}

func (f *fakeApprovalClient) CreateAndWait(ctx context.Context, req model.Request) (clientpkg.Request, error) {
	f.created = req
	return f.result, f.err
}

func (f *fakeExecutor) Execute(ctx context.Context, command model.Command) (model.Result, error) {
	f.command = command
	return f.result, f.err
}

func testDependencies() Dependencies {
	return Dependencies{
		Config:   config.Config{TTYTimeoutSeconds: 0, TimestampDir: filepath.Join("/tmp", "unused")},
		Now:      func() time.Time { return time.Date(2026, 4, 12, 6, 0, 0, 0, time.UTC) },
		TTYName:  func() (string, error) { return "/dev/pts/7", nil },
		LookPath: func(file string) (string, error) { return "/usr/bin/" + file, nil },
		CurrentUser: func() (*user.User, error) {
			return &user.User{Uid: "1000", Gid: "1000", Username: "alice"}, nil
		},
		Hostname: func() (string, error) { return "host", nil },
	}
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

	dep := testDependencies()
	dep.ApprovalClient = client
	dep.LookPath = func(file string) (string, error) { return file, nil }

	exitCode, stdout, stderr, err := Run(context.Background(), dep, []string{"/usr/bin/printf", "hello"}, "/tmp")
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
	dep := testDependencies()
	dep.ApprovalClient = &fakeApprovalClient{}

	_, _, _, err := Run(context.Background(), dep, nil, "/tmp")
	if err == nil {
		t.Fatal("Run() error = nil, want missing command error")
	}
}

func TestRunMapsSignalToShellExitStatus(t *testing.T) {
	client := &fakeApprovalClient{
		result: clientpkg.Request{
			ID:        "req-signal",
			CreatedAt: time.Date(2026, 4, 12, 6, 0, 0, 0, time.UTC),
			Command:   model.Command{ResolvedPath: "/usr/bin/sleep", Argv: []string{"/usr/bin/sleep", "30"}, Cwd: "/tmp"},
			Status:    model.StatusFailed,
			Result:    &clientpkg.Result{ExitCode: -1, Signal: 9, Stderr: "killed"},
		},
	}

	dep := testDependencies()
	dep.ApprovalClient = client
	dep.LookPath = func(file string) (string, error) { return file, nil }

	exitCode, stdout, stderr, err := Run(context.Background(), dep, []string{"/usr/bin/sleep", "30"}, "/tmp")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 137 {
		t.Fatalf("exitCode = %d, want %d", exitCode, 137)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if stderr != "killed" {
		t.Fatalf("stderr = %q, want %q", stderr, "killed")
	}
}

func TestRunValidateUsesCacheWithoutApproval(t *testing.T) {
	timestampDir := t.TempDir()
	now := time.Date(2026, 4, 12, 6, 0, 0, 0, time.UTC)
	cachePath, ok := ttyTimestampCache{dir: timestampDir, timeout: 300 * time.Second, ttyName: func() (string, error) { return "/dev/pts/7", nil }}.path()
	if !ok {
		t.Fatal("expected cache path")
	}
	if err := os.WriteFile(cachePath, []byte(now.Add(-time.Minute).Format(time.RFC3339Nano)+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeApprovalClient{}
	dep := testDependencies()
	dep.ApprovalClient = client
	dep.Config = config.Config{TTYTimeoutSeconds: 300, TimestampDir: timestampDir}
	dep.Now = func() time.Time { return now }

	exitCode, stdout, stderr, err := Run(context.Background(), dep, []string{"-v"}, "/tmp")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 0 || stdout != "" || stderr != "" {
		t.Fatalf("result = (%d, %q, %q), want success with no output", exitCode, stdout, stderr)
	}
	if client.created.ID() != "" {
		t.Fatal("expected cached validation to skip approval request")
	}
}

func TestRunUsesDirectExecutorWhenTTYCacheIsFresh(t *testing.T) {
	timestampDir := t.TempDir()
	now := time.Date(2026, 4, 12, 6, 0, 0, 0, time.UTC)
	cachePath, ok := ttyTimestampCache{dir: timestampDir, timeout: 300 * time.Second, ttyName: func() (string, error) { return "/dev/pts/7", nil }}.path()
	if !ok {
		t.Fatal("expected cache path")
	}
	if err := os.WriteFile(cachePath, []byte(now.Add(-time.Minute).Format(time.RFC3339Nano)+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeApprovalClient{}
	executor := &fakeExecutor{result: model.Result{ExitCode: 0, Stdout: "cached"}}
	dep := testDependencies()
	dep.ApprovalClient = client
	dep.Executor = executor
	dep.Config = config.Config{TTYTimeoutSeconds: 300, TimestampDir: timestampDir}
	dep.Now = func() time.Time { return now }

	exitCode, stdout, stderr, err := Run(context.Background(), dep, []string{"printf", "hello"}, "/tmp")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 0 || stdout != "cached" || stderr != "" {
		t.Fatalf("result = (%d, %q, %q), want cached executor result", exitCode, stdout, stderr)
	}
	if client.created.ID() != "" {
		t.Fatal("expected cached command execution to skip approval request")
	}
	if executor.command.ResolvedPath != "/usr/bin/printf" {
		t.Fatalf("resolved path = %q, want %q", executor.command.ResolvedPath, "/usr/bin/printf")
	}
}
