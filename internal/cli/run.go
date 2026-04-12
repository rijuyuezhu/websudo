package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	clientpkg "websudo/internal/client"
	"websudo/internal/config"
	"websudo/internal/model"
	"websudo/internal/rootd"
)

type ApprovalClient interface {
	CreateAndWait(context.Context, model.Request) (clientpkg.Request, error)
}

type Executor interface {
	Execute(context.Context, model.Command) (model.Result, error)
}

type Dependencies struct {
	ApprovalClient ApprovalClient
	Executor       Executor
	Config         config.Config
	Now            func() time.Time
	CurrentUser    func() (*user.User, error)
	Hostname       func() (string, error)
	LookPath       func(string) (string, error)
	TTYName        func() (string, error)
}

type invocation struct {
	validate bool
	command  []string
}

type socketExecutor struct {
	socketPath string
}

type ttyTimestampCache struct {
	dir     string
	timeout time.Duration
	now     func() time.Time
	ttyName func() (string, error)
}

func Run(ctx context.Context, dep Dependencies, argv []string, cwd string) (int, string, string, error) {
	inv, err := parseInvocation(argv)
	if err != nil {
		return 0, "", "", err
	}

	cfg := dep.Config
	if cfg == (config.Config{}) {
		cfg = config.Default()
	}
	if dep.Now == nil {
		dep.Now = time.Now
	}
	if dep.CurrentUser == nil {
		dep.CurrentUser = user.Current
	}
	if dep.Hostname == nil {
		dep.Hostname = os.Hostname
	}
	if dep.LookPath == nil {
		dep.LookPath = exec.LookPath
	}
	if dep.TTYName == nil {
		dep.TTYName = currentTTYName
	}
	if dep.Executor == nil {
		dep.Executor = socketExecutor{socketPath: cfg.RootSocketPath}
	}

	cache := ttyTimestampCache{
		dir:     cfg.TimestampDir,
		timeout: time.Duration(cfg.TTYTimeoutSeconds) * time.Second,
		now:     dep.Now,
		ttyName: dep.TTYName,
	}

	if inv.validate {
		if cache.isFresh() {
			_ = cache.touch()
			return 0, "", "", nil
		}
		result, err := runWithApproval(ctx, dep, []string{"true"}, cwd)
		if err != nil {
			return 0, "", "", err
		}
		_ = cache.touch()
		return result.exitCode, result.stdout, result.stderr, nil
	}

	command, err := resolveCommand(dep.LookPath, inv.command, cwd)
	if err != nil {
		return 0, "", "", err
	}
	if cache.isFresh() {
		_ = cache.touch()
		result, err := dep.Executor.Execute(ctx, command)
		if err != nil {
			return 0, "", "", err
		}
		return resultToExit(result), result.Stdout, result.Stderr, nil
	}

	result, err := runWithApproval(ctx, dep, inv.command, cwd)
	if err != nil {
		return 0, "", "", err
	}
	_ = cache.touch()
	return result.exitCode, result.stdout, result.stderr, nil
}

func parseInvocation(argv []string) (invocation, error) {
	if len(argv) == 0 {
		return invocation{}, errors.New("no command provided")
	}
	if argv[0] != "-v" {
		return invocation{command: append([]string(nil), argv...)}, nil
	}
	if len(argv) != 1 {
		return invocation{}, errors.New("-v does not accept a command")
	}
	return invocation{validate: true}, nil
}

func runWithApproval(ctx context.Context, dep Dependencies, argv []string, cwd string) (runResult, error) {
	command, err := resolveCommand(dep.LookPath, argv, cwd)
	if err != nil {
		return runResult{}, err
	}
	currentUser, err := dep.CurrentUser()
	if err != nil {
		return runResult{}, err
	}
	hostname, err := dep.Hostname()
	if err != nil {
		return runResult{}, err
	}
	uid, _ := strconv.Atoi(currentUser.Uid)
	gid, _ := strconv.Atoi(currentUser.Gid)
	now := dep.Now().UTC()

	req := model.NewRequest(
		"req-"+now.Format("20060102150405.000000000"),
		now,
		model.Requester{UID: uid, GID: gid, Username: currentUser.Username, Hostname: hostname},
		command,
	)

	finalReq, err := dep.ApprovalClient.CreateAndWait(ctx, req)
	if err != nil {
		return runResult{}, err
	}
	if finalReq.Result == nil {
		return runResult{}, fmt.Errorf("request %s", finalReq.Status)
	}
	return runResult{
		exitCode: resultToExit(model.Result{ExitCode: finalReq.Result.ExitCode, Signal: finalReq.Result.Signal}),
		stdout:   finalReq.Result.Stdout,
		stderr:   finalReq.Result.Stderr,
	}, nil
}

type runResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func resolveCommand(lookPath func(string) (string, error), argv []string, cwd string) (model.Command, error) {
	if len(argv) == 0 {
		return model.Command{}, errors.New("no command provided")
	}
	resolvedPath, err := lookPath(argv[0])
	if err != nil {
		return model.Command{}, err
	}
	return model.Command{ResolvedPath: resolvedPath, Argv: append([]string(nil), argv...), Cwd: cwd}, nil
}

func resultToExit(result model.Result) int {
	if result.Signal != 0 {
		return 128 + result.Signal
	}
	return result.ExitCode
}

func (e socketExecutor) Execute(ctx context.Context, command model.Command) (model.Result, error) {
	response, err := rootd.Execute(ctx, e.socketPath, rootd.ExecRequest{
		ResolvedPath: command.ResolvedPath,
		Argv:         command.Argv,
		Cwd:          command.Cwd,
	})
	if err != nil {
		return model.Result{}, err
	}
	result := model.Result{
		ExitCode: response.Result.ExitCode,
		Signal:   response.Result.Signal,
		Stdout:   response.Result.Stdout,
		Stderr:   response.Result.Stderr,
	}
	if response.Error != "" {
		return result, errors.New(response.Error)
	}
	return result, nil
}

func currentTTYName() (string, error) {
	for _, fd := range []int{0, 1, 2} {
		if _, err := unix.IoctlGetTermios(fd, unix.TCGETS); err != nil {
			continue
		}
		path, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
		if err != nil {
			continue
		}
		if strings.TrimSpace(path) != "" {
			return path, nil
		}
	}
	return "", errors.New("no tty available")
}

func (c ttyTimestampCache) isFresh() bool {
	if c.timeout <= 0 {
		return false
	}
	path, ok := c.path()
	if !ok {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	stampedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return c.now().Before(stampedAt.Add(c.timeout))
}

func (c ttyTimestampCache) touch() error {
	if c.timeout <= 0 {
		return nil
	}
	path, ok := c.path()
	if !ok {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(c.now().UTC().Format(time.RFC3339Nano)+"\n"), 0o600)
}

func (c ttyTimestampCache) path() (string, bool) {
	if strings.TrimSpace(c.dir) == "" || c.ttyName == nil {
		return "", false
	}
	tty, err := c.ttyName()
	if err != nil || strings.TrimSpace(tty) == "" {
		return "", false
	}
	hash := sha256.Sum256([]byte(tty))
	return filepath.Join(c.dir, fmt.Sprintf("tty-%x", hash[:8])), true
}
