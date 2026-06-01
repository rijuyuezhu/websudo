# Sudo Askpass PoC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the default `websudo` execution path with a sudo `-A` wrapper and browser-backed askpass helper while leaving legacy rootd code passing its existing tests.

**Architecture:** Add a new `internal/sudoexec` package for invoking `/usr/bin/sudo -A`, a new `internal/askpass` package for the askpass HTTP client, and in-memory askpass support inside `internal/approverd`. `cmd/websudo` becomes a thin sudo wrapper; `cmd/websudo-askpass` becomes the helper invoked by sudo when sudo/PAM needs a password.

**Tech Stack:** Go standard library, existing Go HTTP server/templates, existing `config` package, existing `go test ./...` verification.

---

## File Structure

- Modify `internal/config/config.go`: add sudo and askpass configuration fields and environment overrides.
- Modify `internal/config/config_test.go`: verify default and environment override behavior for the new fields.
- Create `internal/sudoexec/run.go`: parse `websudo` arguments and execute sudo with `-A`, `SUDO_ASKPASS`, stdin/stdout/stderr wiring, and exit-code mapping.
- Create `internal/sudoexec/run_test.go`: test sudo argv/env construction, `-v` mapping, missing command errors, and exit code propagation using fake sudo scripts.
- Modify `cmd/websudo/main.go`: call `sudoexec.Run` instead of the legacy approval/rootd path.
- Create `internal/approverd/askpass_store.go`: in-memory askpass request store with random IDs, status transitions, expiration, one-time password consumption, and no persistence.
- Create `internal/approverd/askpass_store_test.go`: unit-test create/get/list/complete/consume/deny/expire behavior.
- Create `internal/approverd/askpass_handlers.go`: HTTP handlers for creating, viewing, completing, denying, and consuming askpass prompts.
- Modify `internal/approverd/server.go`: add `AskpassStore` to dependencies/server state and route askpass endpoints.
- Modify `internal/approverd/server_test.go`: add handler tests for askpass responses and keep legacy tests passing.
- Modify `internal/approverd/templates/index.html`: show pending askpass prompts.
- Create `internal/approverd/templates/askpass.html`: minimal password prompt page.
- Create `internal/askpass/client.go`: client used by `websudo-askpass` to create an askpass request and poll one-time password consumption.
- Create `internal/askpass/client_test.go`: test success, denied/expired responses, and service errors without exposing passwords through GET.
- Create `cmd/websudo-askpass/main.go`: askpass helper binary that prints only the password to stdout on success.
- Update `README.md`: document sudo askpass PoC usage and rootd deprecation for the default path.

---

### Task 1: Configuration And Sudo Wrapper

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Create: `internal/sudoexec/run.go`
- Create: `internal/sudoexec/run_test.go`
- Modify: `cmd/websudo/main.go`

- [ ] **Step 1: Write failing config tests**

Append these tests to `internal/config/config_test.go`:

```go
func TestDefaultsExposeSudoAskpassConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("WEBSUDO_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("WEBSUDO_SUDO_PATH", "")
	t.Setenv("WEBSUDO_ASKPASS_PATH", "")

	cfg := Default()

	if cfg.SudoPath != "/usr/bin/sudo" {
		t.Fatalf("sudo path = %q, want %q", cfg.SudoPath, "/usr/bin/sudo")
	}
	if cfg.AskpassPath != "" {
		t.Fatalf("askpass path = %q, want empty default for PATH lookup", cfg.AskpassPath)
	}
}

func TestDefaultsHonorSudoAskpassEnvironmentOverrides(t *testing.T) {
	t.Setenv("WEBSUDO_SUDO_PATH", "/custom/sudo")
	t.Setenv("WEBSUDO_ASKPASS_PATH", "/custom/websudo-askpass")

	cfg := Default()

	if cfg.SudoPath != "/custom/sudo" {
		t.Fatalf("sudo path = %q, want %q", cfg.SudoPath, "/custom/sudo")
	}
	if cfg.AskpassPath != "/custom/websudo-askpass" {
		t.Fatalf("askpass path = %q, want %q", cfg.AskpassPath, "/custom/websudo-askpass")
	}
}
```

- [ ] **Step 2: Run config tests and verify they fail**

Run: `go test ./internal/config -run 'TestDefaultsExposeSudoAskpassConfig|TestDefaultsHonorSudoAskpassEnvironmentOverrides' -v`

Expected: FAIL with compile errors like `cfg.SudoPath undefined` and `cfg.AskpassPath undefined`.

- [ ] **Step 3: Implement config fields and overrides**

In `internal/config/config.go`, update `Config` and `Default()` with these exact additions:

```go
type Config struct {
	WebAddr                string
	ApprovalTimeoutSeconds int
	TTYTimeoutSeconds      int
	TokenHashHex           string
	DatabasePath           string
	TimestampDir           string
	RootSocketPath         string
	RootAllowedUID         int
	SudoPath               string
	AskpassPath            string
}
```

In the default config literal, add:

```go
		SudoPath:               "/usr/bin/sudo",
		AskpassPath:            "",
```

Before `return cfg`, add:

```go
	if value, ok := envString(fileEnv, "WEBSUDO_SUDO_PATH"); ok {
		cfg.SudoPath = value
	}
	if value, ok := envString(fileEnv, "WEBSUDO_ASKPASS_PATH"); ok {
		cfg.AskpassPath = value
	}
```

- [ ] **Step 4: Run config tests and verify they pass**

Run: `go test ./internal/config -v`

Expected: PASS for all config tests.

- [ ] **Step 5: Write failing sudoexec tests**

Create `internal/sudoexec/run_test.go`:

```go
package sudoexec

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"websudo/internal/config"
)

func TestRunExecutesCommandThroughSudoAskpass(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	envPath := filepath.Join(t.TempDir(), "env.txt")
	fakeSudo := writeFakeSudo(t, `#!/bin/sh
printf '%s\n' "$*" > "$WEBSUDO_TEST_ARGS"
printf '%s\n' "$SUDO_ASKPASS" > "$WEBSUDO_TEST_ENV"
printf 'sudo-stdout'
printf 'sudo-stderr' >&2
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode, err := Run(context.Background(), Dependencies{
		Config: config.Config{SudoPath: fakeSudo, AskpassPath: "/tmp/websudo-askpass"},
		Environ: func() []string {
			return []string{"WEBSUDO_TEST_ARGS=" + argsPath, "WEBSUDO_TEST_ENV=" + envPath}
		},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("stdin"),
	}, []string{"/usr/bin/true", "--flag"}, t.TempDir())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if stdout.String() != "sudo-stdout" {
		t.Fatalf("stdout = %q, want sudo-stdout", stdout.String())
	}
	if stderr.String() != "sudo-stderr" {
		t.Fatalf("stderr = %q, want sudo-stderr", stderr.String())
	}
	if got := strings.TrimSpace(readFile(t, argsPath)); got != "-A -- /usr/bin/true --flag" {
		t.Fatalf("sudo args = %q, want %q", got, "-A -- /usr/bin/true --flag")
	}
	if got := strings.TrimSpace(readFile(t, envPath)); got != "/tmp/websudo-askpass" {
		t.Fatalf("SUDO_ASKPASS = %q, want %q", got, "/tmp/websudo-askpass")
	}
}

func TestRunValidateUsesSudoValidate(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	fakeSudo := writeFakeSudo(t, `#!/bin/sh
printf '%s\n' "$*" > "$WEBSUDO_TEST_ARGS"
`)

	exitCode, err := Run(context.Background(), Dependencies{
		Config: config.Config{SudoPath: fakeSudo, AskpassPath: "/tmp/websudo-askpass"},
		Environ: func() []string {
			return []string{"WEBSUDO_TEST_ARGS=" + argsPath}
		},
	}, []string{"-v"}, t.TempDir())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if got := strings.TrimSpace(readFile(t, argsPath)); got != "-A -v" {
		t.Fatalf("sudo args = %q, want %q", got, "-A -v")
	}
}

func TestRunReturnsErrorWhenCommandMissing(t *testing.T) {
	_, err := Run(context.Background(), Dependencies{Config: config.Config{SudoPath: "/usr/bin/sudo", AskpassPath: "/tmp/websudo-askpass"}}, nil, t.TempDir())
	if err == nil {
		t.Fatal("Run() error = nil, want missing command error")
	}
}

func TestRunPropagatesSudoExitCode(t *testing.T) {
	fakeSudo := writeFakeSudo(t, `#!/bin/sh
exit 7
`)

	exitCode, err := Run(context.Background(), Dependencies{
		Config: config.Config{SudoPath: fakeSudo, AskpassPath: "/tmp/websudo-askpass"},
	}, []string{"/usr/bin/false"}, t.TempDir())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if exitCode != 7 {
		t.Fatalf("exitCode = %d, want 7", exitCode)
	}
}

func writeFakeSudo(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sudo")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(data)
}
```

- [ ] **Step 6: Run sudoexec tests and verify they fail**

Run: `go test ./internal/sudoexec -v`

Expected: FAIL with `stat /.../internal/sudoexec: directory not found` or `undefined: Run` after the package is created without implementation.

- [ ] **Step 7: Implement sudoexec**

Create `internal/sudoexec/run.go`:

```go
package sudoexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"websudo/internal/config"
)

type Dependencies struct {
	Config  config.Config
	Environ func() []string
	LookPath func(string) (string, error)
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

func Run(ctx context.Context, dep Dependencies, argv []string, cwd string) (int, error) {
	sudoArgs, err := sudoArgs(argv)
	if err != nil {
		return 0, err
	}
	cfg := fillConfig(dep.Config)
	lookPath := dep.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	askpassPath, err := resolveAskpassPath(cfg.AskpassPath, lookPath)
	if err != nil {
		return 0, err
	}
	environ := dep.Environ
	if environ == nil {
		environ = os.Environ
	}

	cmd := exec.CommandContext(ctx, cfg.SudoPath, sudoArgs...)
	cmd.Dir = cwd
	cmd.Env = withEnv(environ(), "SUDO_ASKPASS", askpassPath)
	cmd.Stdin = dep.Stdin
	cmd.Stdout = dep.Stdout
	cmd.Stderr = dep.Stderr
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return exitCodeFromError(err)
	}
	return 0, nil
}

func fillConfig(cfg config.Config) config.Config {
	defaults := config.Default()
	if strings.TrimSpace(cfg.SudoPath) == "" {
		cfg.SudoPath = defaults.SudoPath
	}
	return cfg
}

func sudoArgs(argv []string) ([]string, error) {
	if len(argv) == 0 {
		return nil, errors.New("no command provided")
	}
	if argv[0] == "-v" {
		if len(argv) != 1 {
			return nil, errors.New("-v does not accept a command")
		}
		return []string{"-A", "-v"}, nil
	}
	args := []string{"-A", "--"}
	args = append(args, argv...)
	return args, nil
}

func resolveAskpassPath(configured string, lookPath func(string) (string, error)) (string, error) {
	if strings.TrimSpace(configured) != "" {
		return configured, nil
	}
	path, err := lookPath("websudo-askpass")
	if err != nil {
		return "", fmt.Errorf("resolve websudo-askpass: %w", err)
	}
	return path, nil
}

func withEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			out = append(out, prefix+value)
			replaced = true
			continue
		}
		out = append(out, item)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

func exitCodeFromError(err error) (int, error) {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 1, err
	}
	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal()), nil
	}
	return exitErr.ExitCode(), nil
}
```

- [ ] **Step 8: Update `cmd/websudo` to use sudoexec**

Replace `cmd/websudo/main.go` with:

```go
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
```

- [ ] **Step 9: Run wrapper tests and full suite**

Run: `go test ./internal/config ./internal/sudoexec ./cmd/websudo -v`

Expected: PASS.

Run: `go test ./...`

Expected: PASS. Legacy tests still pass because `internal/cli` remains unchanged.

- [ ] **Step 10: Commit Task 1**

```bash
git add internal/config/config.go internal/config/config_test.go internal/sudoexec/run.go internal/sudoexec/run_test.go cmd/websudo/main.go
git commit -m "feat: wrap sudo with askpass"
```

---

### Task 2: In-Memory Askpass Store

**Files:**
- Create: `internal/approverd/askpass_store.go`
- Create: `internal/approverd/askpass_store_test.go`

- [ ] **Step 1: Write failing store tests**

Create `internal/approverd/askpass_store_test.go`:

```go
package approverd

import (
	"testing"
	"time"
)

func TestAskpassStoreCreateCompleteConsumeOnce(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string { return "askpass-1" })

	req := store.Create("[sudo] password for alice:")
	if req.ID != "askpass-1" {
		t.Fatalf("id = %q, want askpass-1", req.ID)
	}
	if req.Prompt != "[sudo] password for alice:" {
		t.Fatalf("prompt = %q", req.Prompt)
	}
	if req.Status != AskpassPending {
		t.Fatalf("status = %q, want %q", req.Status, AskpassPending)
	}

	completed, err := store.Complete("askpass-1", "secret")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if completed.Status != AskpassCompleted {
		t.Fatalf("status = %q, want %q", completed.Status, AskpassCompleted)
	}

	password, err := store.Consume("askpass-1")
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
	if _, err := store.Consume("askpass-1"); err == nil {
		t.Fatal("second Consume() error = nil, want missing request")
	}
}

func TestAskpassStoreDoesNotExposePasswordInGet(t *testing.T) {
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string { return "askpass-2" })
	store.Create("Password:")
	if _, err := store.Complete("askpass-2", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	req, err := store.Get("askpass-2")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if req.Status != AskpassCompleted {
		t.Fatalf("status = %q, want completed", req.Status)
	}
}

func TestAskpassStoreDenyAndExpire(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	ids := []string{"askpass-deny", "askpass-expire"}
	store := newAskpassStoreForTest(func() time.Time { return now }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})

	store.Create("deny")
	denied, err := store.Deny("askpass-deny")
	if err != nil {
		t.Fatalf("Deny() error = %v", err)
	}
	if denied.Status != AskpassDenied {
		t.Fatalf("status = %q, want denied", denied.Status)
	}
	if _, err := store.Consume("askpass-deny"); err == nil {
		t.Fatal("Consume(denied) error = nil, want terminal status error")
	}

	store.Create("expire")
	expired := store.ExpireBefore(now.Add(time.Second))
	if expired != 1 {
		t.Fatalf("expired = %d, want 1", expired)
	}
	req, err := store.Get("askpass-expire")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if req.Status != AskpassExpired {
		t.Fatalf("status = %q, want expired", req.Status)
	}
}

func TestAskpassStoreListsOnlyPending(t *testing.T) {
	ids := []string{"askpass-a", "askpass-b"}
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})
	store.Create("a")
	store.Create("b")
	if _, err := store.Complete("askpass-b", "secret"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	pending := store.ListPending()
	if len(pending) != 1 || pending[0].ID != "askpass-a" {
		t.Fatalf("pending = %#v, want only askpass-a", pending)
	}
}
```

- [ ] **Step 2: Run store tests and verify they fail**

Run: `go test ./internal/approverd -run AskpassStore -v`

Expected: FAIL with undefined symbols such as `newAskpassStoreForTest` and `AskpassPending`.

- [ ] **Step 3: Implement askpass store**

Create `internal/approverd/askpass_store.go`:

```go
package approverd

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

type AskpassStatus string

const (
	AskpassPending   AskpassStatus = "pending"
	AskpassCompleted AskpassStatus = "completed"
	AskpassDenied    AskpassStatus = "denied"
	AskpassExpired   AskpassStatus = "expired"
)

type AskpassRequest struct {
	ID        string        `json:"id"`
	Prompt    string        `json:"prompt"`
	CreatedAt time.Time     `json:"createdAt"`
	Status    AskpassStatus `json:"status"`
}

type askpassEntry struct {
	request  AskpassRequest
	password string
}

type AskpassStore struct {
	mu     sync.Mutex
	now    func() time.Time
	newID  func() string
	items  map[string]askpassEntry
	order  []string
}

func NewAskpassStore() *AskpassStore {
	return newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, randomAskpassID)
}

func newAskpassStoreForTest(now func() time.Time, newID func() string) *AskpassStore {
	return &AskpassStore{now: now, newID: newID, items: make(map[string]askpassEntry)}
}

func (s *AskpassStore) Create(prompt string) AskpassRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	req := AskpassRequest{ID: s.newID(), Prompt: prompt, CreatedAt: s.now().UTC(), Status: AskpassPending}
	s.items[req.ID] = askpassEntry{request: req}
	s.order = append([]string{req.ID}, s.order...)
	return req
}

func (s *AskpassStore) Get(id string) (AskpassRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[id]
	if !ok {
		return AskpassRequest{}, errors.New("askpass request not found")
	}
	return entry.request, nil
}

func (s *AskpassStore) ListPending() []AskpassRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	requests := make([]AskpassRequest, 0)
	for _, id := range s.order {
		entry := s.items[id]
		if entry.request.Status == AskpassPending {
			requests = append(requests, entry.request)
		}
	}
	sort.SliceStable(requests, func(i, j int) bool { return requests[i].CreatedAt.After(requests[j].CreatedAt) })
	return requests
}

func (s *AskpassStore) Complete(id, password string) (AskpassRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[id]
	if !ok {
		return AskpassRequest{}, errors.New("askpass request not found")
	}
	if entry.request.Status != AskpassPending {
		return AskpassRequest{}, fmt.Errorf("askpass request is %s", entry.request.Status)
	}
	entry.request.Status = AskpassCompleted
	entry.password = password
	s.items[id] = entry
	return entry.request, nil
}

func (s *AskpassStore) Deny(id string) (AskpassRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[id]
	if !ok {
		return AskpassRequest{}, errors.New("askpass request not found")
	}
	if entry.request.Status != AskpassPending {
		return AskpassRequest{}, fmt.Errorf("askpass request is %s", entry.request.Status)
	}
	entry.request.Status = AskpassDenied
	entry.password = ""
	s.items[id] = entry
	return entry.request, nil
}

func (s *AskpassStore) Consume(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[id]
	if !ok {
		return "", errors.New("askpass request not found")
	}
	if entry.request.Status != AskpassCompleted {
		return "", fmt.Errorf("askpass request is %s", entry.request.Status)
	}
	password := entry.password
	delete(s.items, id)
	for index, orderedID := range s.order {
		if orderedID == id {
			s.order = append(s.order[:index], s.order[index+1:]...)
			break
		}
	}
	return password, nil
}

func (s *AskpassStore) ExpireBefore(before time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	expired := 0
	for id, entry := range s.items {
		if entry.request.Status != AskpassPending || entry.request.CreatedAt.After(before) {
			continue
		}
		entry.request.Status = AskpassExpired
		entry.password = ""
		s.items[id] = entry
		expired++
	}
	return expired
}

func randomAskpassID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return "askpass-" + hex.EncodeToString(bytes[:])
}
```

- [ ] **Step 4: Run askpass store tests**

Run: `go test ./internal/approverd -run AskpassStore -v`

Expected: PASS.

- [ ] **Step 5: Run approverd package tests**

Run: `go test ./internal/approverd -v`

Expected: PASS.

- [ ] **Step 6: Commit Task 2**

```bash
git add internal/approverd/askpass_store.go internal/approverd/askpass_store_test.go
git commit -m "feat: add askpass memory store"
```

---

### Task 3: Approverd Askpass HTTP And Templates

**Files:**
- Modify: `internal/approverd/server.go`
- Create: `internal/approverd/askpass_handlers.go`
- Modify: `internal/approverd/server_test.go`
- Modify: `internal/approverd/templates/index.html`
- Create: `internal/approverd/templates/askpass.html`

- [ ] **Step 1: Add failing handler tests**

Append these tests before `testTemplates` in `internal/approverd/server_test.go`:

```go
func TestAskpassCreateGetCompleteAndConsume(t *testing.T) {
	store := newAskpassStoreForTest(func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }, func() string { return "askpass-http" })
	srv := NewServer(Dependencies{AskpassStore: store, Templates: testTemplates(t)})

	createReq := httptest.NewRequest(http.MethodPost, "/api/askpass", strings.NewReader(`{"prompt":"Password:"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createW.Code, http.StatusCreated)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/askpass/askpass-http", nil)
	getW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getW.Code, http.StatusOK)
	}
	if strings.Contains(getW.Body.String(), "secret") {
		t.Fatalf("GET leaked password: %q", getW.Body.String())
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-http/complete", strings.NewReader(`{"password":"secret"}`))
	completeReq.Header.Set("Content-Type", "application/json")
	completeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(completeW, completeReq)
	if completeW.Code != http.StatusAccepted {
		t.Fatalf("complete status = %d, want %d", completeW.Code, http.StatusAccepted)
	}
	if strings.Contains(completeW.Body.String(), "secret") {
		t.Fatalf("complete echoed password: %q", completeW.Body.String())
	}

	consumeReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-http/consume", nil)
	consumeW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(consumeW, consumeReq)
	if consumeW.Code != http.StatusOK {
		t.Fatalf("consume status = %d, want %d", consumeW.Code, http.StatusOK)
	}
	var payload map[string]string
	if err := json.Unmarshal(consumeW.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["password"] != "secret" {
		t.Fatalf("password = %q, want secret", payload["password"])
	}

	secondConsume := httptest.NewRecorder()
	srv.Routes().ServeHTTP(secondConsume, consumeReq.Clone(context.Background()))
	if secondConsume.Code != http.StatusNotFound {
		t.Fatalf("second consume status = %d, want %d", secondConsume.Code, http.StatusNotFound)
	}
}

func TestAskpassDenyAndPendingConsumeStatus(t *testing.T) {
	ids := []string{"askpass-pending", "askpass-deny"}
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	})
	store.Create("pending")
	store.Create("deny")
	srv := NewServer(Dependencies{AskpassStore: store, Templates: testTemplates(t)})

	pendingConsume := httptest.NewRecorder()
	srv.Routes().ServeHTTP(pendingConsume, httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-pending/consume", nil))
	if pendingConsume.Code != http.StatusConflict {
		t.Fatalf("pending consume status = %d, want %d", pendingConsume.Code, http.StatusConflict)
	}

	denyReq := httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-deny/deny", nil)
	denyReq.Header.Set("Content-Type", "application/json")
	denyW := httptest.NewRecorder()
	srv.Routes().ServeHTTP(denyW, denyReq)
	if denyW.Code != http.StatusAccepted {
		t.Fatalf("deny status = %d, want %d", denyW.Code, http.StatusAccepted)
	}

	deniedConsume := httptest.NewRecorder()
	srv.Routes().ServeHTTP(deniedConsume, httptest.NewRequest(http.MethodPost, "/api/askpass/askpass-deny/consume", nil))
	if deniedConsume.Code != http.StatusGone {
		t.Fatalf("denied consume status = %d, want %d", deniedConsume.Code, http.StatusGone)
	}
}

func TestAskpassPageRendersPasswordForm(t *testing.T) {
	store := newAskpassStoreForTest(func() time.Time { return time.Now().UTC() }, func() string { return "askpass-page" })
	store.Create("Password:")
	srv := NewServer(Dependencies{AskpassStore: store})

	w := httptest.NewRecorder()
	srv.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/askpass/askpass-page", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !strings.Contains(body, `action="/api/askpass/askpass-page/complete"`) {
		t.Fatalf("askpass page missing complete form: %s", body)
	}
	if !strings.Contains(body, `type="password"`) {
		t.Fatalf("askpass page missing password input: %s", body)
	}
}
```

Update `testTemplates` in `internal/approverd/server_test.go` to define `askpass.html`:

```go
func testTemplates(t *testing.T) *template.Template {
	t.Helper()

	return template.Must(template.New("index.html").Parse(`{{define "index.html"}}{{range .Pending}}{{.ID}} {{.Command.ResolvedPath}}{{end}}{{range .AskpassPending}}{{.ID}} {{.Prompt}}{{end}}{{end}}` +
		`{{define "request.html"}}{{.ID}} {{.Status}} {{.RequestedBy.Username}}{{end}}` +
		`{{define "askpass.html"}}{{.ID}} {{.Prompt}}<form method="post" action="/api/askpass/{{.ID}}/complete"><input type="password" name="password" /></form>{{end}}`))
}
```

- [ ] **Step 2: Run handler tests and verify they fail**

Run: `go test ./internal/approverd -run Askpass -v`

Expected: FAIL with missing `Dependencies.AskpassStore`, missing routes, or 404 responses.

- [ ] **Step 3: Wire askpass store into Server and routes**

In `internal/approverd/server.go`, add `AskpassStore *AskpassStore` to `Dependencies`, add `askpassStore *AskpassStore` to `Server`, initialize it in `NewServer`, and add the new routes.

Use these exact edits:

```go
type Dependencies struct {
	Config       config.Config
	Store        Store
	Templates    *template.Template
	Executor     Executor
	AskpassStore *AskpassStore
}

type Server struct {
	config       config.Config
	store        Store
	templates    *template.Template
	executor     Executor
	askpassStore *AskpassStore
}
```

In `NewServer`, before the return:

```go
	askpassStore := dep.AskpassStore
	if askpassStore == nil {
		askpassStore = NewAskpassStore()
	}
```

In the `Server` literal, add:

```go
		askpassStore: askpassStore,
```

In `Routes()`, add:

```go
	mux.HandleFunc("/askpass/", s.handleAskpassPage)
	mux.HandleFunc("/api/askpass", s.handleAskpassCreate)
	mux.HandleFunc("/api/askpass/", s.handleAskpassAction)
```

In `handleIndex`, before rendering, add:

```go
	if err := s.expireAskpassRequests(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
```

Add `AskpassPending` to the template data map:

```go
		"AskpassPending": s.askpassStore.ListPending(),
```

- [ ] **Step 4: Implement askpass handlers**

Create `internal/approverd/askpass_handlers.go`:

```go
package approverd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleAskpassCreate(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/askpass" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req := s.askpassStore.Create(body.Prompt)
	writeJSON(w, http.StatusCreated, req)
}

func (s *Server) handleAskpassPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := s.expireAskpassRequests(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, ok := requestIDFromPath(r.URL.Path, "/askpass/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	req, err := s.askpassStore.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "askpass.html", req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAskpassAction(w http.ResponseWriter, r *http.Request) {
	if err := s.expireAskpassRequests(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id, action, ok := askpassActionFromPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet && action == "" {
		req, err := s.askpassStore.Get(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, req)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	switch action {
	case "complete":
		password, err := askpassPassword(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := s.askpassStore.Complete(id, password); err != nil {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if isJSONRequest(r) {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	case "deny":
		if _, err := s.askpassStore.Deny(id); err != nil {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if isJSONRequest(r) {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	case "consume":
		password, err := s.askpassStore.Consume(id)
		if err != nil {
			status := http.StatusConflict
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			} else if strings.Contains(err.Error(), string(AskpassDenied)) || strings.Contains(err.Error(), string(AskpassExpired)) {
				status = http.StatusGone
			}
			w.WriteHeader(status)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"password": password})
	default:
		http.NotFound(w, r)
	}
}

func askpassActionFromPath(path string) (string, string, bool) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/askpass/"), "/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func askpassPassword(r *http.Request) (string, error) {
	if isJSONRequest(r) {
		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return "", err
		}
		if body.Password == "" {
			return "", errors.New("missing password")
		}
		return body.Password, nil
	}
	if err := r.ParseForm(); err != nil {
		return "", err
	}
	password := r.Form.Get("password")
	if password == "" {
		return "", errors.New("missing password")
	}
	return password, nil
}

func (s *Server) expireAskpassRequests() error {
	if s.config.ApprovalTimeoutSeconds <= 0 || s.askpassStore == nil {
		return nil
	}
	s.askpassStore.ExpireBefore(time.Now().Add(-time.Duration(s.config.ApprovalTimeoutSeconds) * time.Second).UTC())
	return nil
}
```

- [ ] **Step 5: Update templates**

Replace `internal/approverd/templates/index.html` with:

```html
<!doctype html>
<html lang="en">
<body>
  <h1>Pending Password Prompts</h1>
  {{range .AskpassPending}}
  <article>
    <h2><a href="/askpass/{{.ID}}">{{.ID}}</a></h2>
    <p>Status: {{.Status}}</p>
    <pre>{{.Prompt}}</pre>
  </article>
  {{else}}
  <p>No pending password prompts.</p>
  {{end}}

  <h1>Pending Requests</h1>
  {{range .Pending}}
  <article>
    <h2><a href="/requests/{{.ID}}">{{.ID}}</a></h2>
    <p>Status: {{.Status}}</p>
    <pre>{{.Command.ResolvedPath}}</pre>
  </article>
  {{else}}
  <p>No pending requests.</p>
  {{end}}

  <h1>Recent Requests</h1>
  {{range .Recent}}
  <article>
    <h2><a href="/requests/{{.ID}}">{{.ID}}</a></h2>
    <p>Status: {{.Status}}</p>
  </article>
  {{else}}
  <p>No recent requests.</p>
  {{end}}
</body>
</html>
```

Create `internal/approverd/templates/askpass.html`:

```html
<!doctype html>
<html lang="en">
<body>
  <p><a href="/">Back</a></p>
  <h1>Sudo Password Required</h1>
  <p>Status: {{.Status}}</p>
  <pre>{{.Prompt}}</pre>

  <form method="post" action="/api/askpass/{{.ID}}/complete">
    <label>Password <input type="password" name="password" autocomplete="current-password" /></label>
    <button type="submit">Submit Password</button>
  </form>

  <form method="post" action="/api/askpass/{{.ID}}/deny">
    <button type="submit">Cancel</button>
  </form>
</body>
</html>
```

- [ ] **Step 6: Run approverd tests**

Run: `go test ./internal/approverd -v`

Expected: PASS.

- [ ] **Step 7: Run full suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 8: Commit Task 3**

```bash
git add internal/approverd/server.go internal/approverd/askpass_handlers.go internal/approverd/server_test.go internal/approverd/templates/index.html internal/approverd/templates/askpass.html
git commit -m "feat: add askpass web endpoints"
```

---

### Task 4: Askpass Client And Helper Binary

**Files:**
- Create: `internal/askpass/client.go`
- Create: `internal/askpass/client_test.go`
- Create: `cmd/websudo-askpass/main.go`

- [ ] **Step 1: Write failing askpass client tests**

Create `internal/askpass/client_test.go`:

```go
package askpass

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientCreateAndWaitForPassword(t *testing.T) {
	consumeCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/askpass":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if body["prompt"] != "Password:" {
				t.Fatalf("prompt = %q, want Password:", body["prompt"])
			}
			writeTestJSON(w, http.StatusCreated, map[string]any{"id": "askpass-client", "prompt": "Password:", "status": "pending", "createdAt": "2026-06-01T12:00:00Z"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/askpass/askpass-client/consume":
			consumeCalls++
			if consumeCalls == 1 {
				w.WriteHeader(http.StatusConflict)
				return
			}
			writeTestJSON(w, http.StatusOK, map[string]string{"password": "secret"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL, server.Client())
	client.pollInterval = time.Millisecond
	req, err := client.Create(context.Background(), "Password:")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if req.ID != "askpass-client" {
		t.Fatalf("id = %q, want askpass-client", req.ID)
	}
	password, err := client.WaitForPassword(context.Background(), req.ID)
	if err != nil {
		t.Fatalf("WaitForPassword() error = %v", err)
	}
	if password != "secret" {
		t.Fatalf("password = %q, want secret", password)
	}
}

func TestClientWaitForPasswordReturnsErrorForDeniedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/askpass/askpass-denied/consume" {
			w.WriteHeader(http.StatusGone)
			return
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	client := New(server.URL, server.Client())
	client.pollInterval = time.Millisecond
	_, err := client.WaitForPassword(context.Background(), "askpass-denied")
	if err == nil {
		t.Fatal("WaitForPassword() error = nil, want terminal status error")
	}
}

func TestClientCreateReturnsErrorWhenServiceUnavailable(t *testing.T) {
	client := New("http://127.0.0.1:1", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := client.Create(ctx, "Password:")
	if err == nil {
		t.Fatal("Create() error = nil, want connection error")
	}
}

func writeTestJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
```

- [ ] **Step 2: Run askpass client tests and verify they fail**

Run: `go test ./internal/askpass -v`

Expected: FAIL with `stat /.../internal/askpass: directory not found` or undefined `New`.

- [ ] **Step 3: Implement askpass client**

Create `internal/askpass/client.go`:

```go
package askpass

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Request struct {
	ID        string    `json:"id"`
	Prompt    string    `json:"prompt"`
	CreatedAt time.Time `json:"createdAt"`
	Status    string    `json:"status"`
}

type Client struct {
	baseURL      string
	httpClient   *http.Client
	pollInterval time.Duration
}

func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient, pollInterval: 250 * time.Millisecond}
}

func (c *Client) Create(ctx context.Context, prompt string) (Request, error) {
	body, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		return Request{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/askpass", bytes.NewReader(body))
	if err != nil {
		return Request{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Request{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return Request{}, fmt.Errorf("create askpass request failed: %s", resp.Status)
	}
	var created Request
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return Request{}, err
	}
	return created, nil
}

func (c *Client) WaitForPassword(ctx context.Context, id string) (string, error) {
	for {
		password, retry, err := c.consume(ctx, id)
		if err == nil {
			return password, nil
		}
		if !retry {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(c.pollInterval):
		}
	}
}

func (c *Client) consume(ctx context.Context, id string) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/askpass/"+id+"/consume", nil)
	if err != nil {
		return "", false, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return "", false, err
		}
		return body.Password, false, nil
	case http.StatusConflict:
		return "", true, fmt.Errorf("askpass request %s is pending", id)
	case http.StatusGone:
		return "", false, fmt.Errorf("askpass request %s ended without a password", id)
	case http.StatusNotFound:
		return "", false, fmt.Errorf("askpass request %s not found", id)
	default:
		return "", false, fmt.Errorf("consume askpass request failed: %s", resp.Status)
	}
}
```

- [ ] **Step 4: Run askpass client tests**

Run: `go test ./internal/askpass -v`

Expected: PASS.

- [ ] **Step 5: Create askpass helper binary**

Create `cmd/websudo-askpass/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"websudo/internal/askpass"
	"websudo/internal/config"
)

func main() {
	cfg := config.Default()
	prompt := "Password:"
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}
	timeout := time.Duration(cfg.ApprovalTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := askpass.New("http://"+cfg.WebAddr, nil)
	req, err := client.Create(ctx, prompt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Open http://%s/askpass/%s to enter the sudo password.\n", cfg.WebAddr, req.ID)
	password, err := client.WaitForPassword(ctx, req.ID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, password)
}
```

- [ ] **Step 6: Run helper package tests and full suite**

Run: `go test ./internal/askpass ./cmd/websudo-askpass -v`

Expected: PASS.

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 7: Commit Task 4**

```bash
git add internal/askpass/client.go internal/askpass/client_test.go cmd/websudo-askpass/main.go
git commit -m "feat: add websudo askpass helper"
```

---

### Task 5: Documentation And End-To-End Verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README for sudo askpass PoC**

Replace `README.md` with:

````markdown
# websudo

Local browser askpass helper for sudo commands.

## Commands

```sh
websudo -v
websudo /usr/bin/true
paru --sudo websudo -Syu
```

`websudo` executes commands through the system `sudo` binary using `sudo -A`. Sudo still owns sudoers policy, PAM authentication, timestamp caching, environment handling, and command execution.

If sudo's timestamp cache is fresh, no browser prompt appears. If sudo needs a password, it invokes `websudo-askpass`; the helper creates a local browser prompt through `websudo-approverd` and prints the submitted password back to sudo for PAM validation.

## Manual Test

1. Build and install `websudo`, `websudo-askpass`, and `websudo-approverd` in `PATH`.
2. Start `websudo-approverd` as your user.
3. Run `websudo -v` or `websudo /usr/bin/true` in a terminal.
4. If sudo needs a password, open `http://127.0.0.1:17878` or the `/askpass/<id>` URL printed by `websudo-askpass`.
5. Enter your system sudo password. The password is delivered once to sudo and is not stored in SQLite.

## Legacy Root Executor

`websudo-rootd` and the old fixed-token root execution flow are legacy implementation pieces. The default PoC path does not use them.
````

- [ ] **Step 2: Run formatting and tests**

Run: `gofmt -w internal/config/config.go internal/config/config_test.go internal/sudoexec/run.go internal/sudoexec/run_test.go cmd/websudo/main.go internal/approverd/askpass_store.go internal/approverd/askpass_store_test.go internal/approverd/askpass_handlers.go internal/approverd/server.go internal/approverd/server_test.go internal/askpass/client.go internal/askpass/client_test.go cmd/websudo-askpass/main.go`

Expected: command exits 0.

Run: `go test ./...`

Expected: PASS for all packages, including legacy rootd and integration tests.

- [ ] **Step 3: Verify the default binary no longer imports legacy approval client**

Run: `go list -deps ./cmd/websudo`

Expected: output includes `websudo/internal/sudoexec` and does not include `websudo/internal/rootd`.

- [ ] **Step 4: Inspect git diff for password leaks**

Run: `git diff -- . ':!docs/superpowers/plans/2026-06-01-sudo-askpass-poc.md'`

Expected: no code path stores an askpass password in SQLite, no GET handler serializes a password, and `cmd/websudo-askpass/main.go` prints only the password to stdout.

- [ ] **Step 5: Commit Task 5**

```bash
git add README.md
git commit -m "docs: describe sudo askpass flow"
```

---

## Final Verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`

Expected: PASS for all packages.

- [ ] **Step 2: Check status**

Run: `git status --short`

Expected: clean working tree, except this plan file if it was not committed before implementation began.

- [ ] **Step 3: Summarize implemented commits**

Run: `git log --oneline main..HEAD`

Expected: includes commits for the design spec and each completed task.

---

## Self-Review Notes

- Spec coverage: Task 1 implements sudo `-A`, `-v`, `SUDO_ASKPASS`, sudo exit propagation, and default `cmd/websudo` path. Task 2 and Task 3 implement the in-memory askpass model and local web UI. Task 4 implements `websudo-askpass`. Task 5 documents migration and rootd deprecation.
- Password persistence: passwords appear only in `AskpassStore` memory, `POST /consume` responses, and askpass stdout. GET/status responses use `AskpassRequest`, which has no password field.
- Legacy code: `internal/cli`, `internal/rootd`, and legacy integration tests remain untouched so existing tests continue passing while the default binary uses `internal/sudoexec`.
