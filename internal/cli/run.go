package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"time"

	clientpkg "websudo/internal/client"
	"websudo/internal/model"
)

type ApprovalClient interface {
	CreateAndWait(context.Context, model.Request) (clientpkg.Request, error)
}

func Run(ctx context.Context, approvalClient ApprovalClient, argv []string, cwd string) (int, string, string, error) {
	if len(argv) == 0 {
		return 0, "", "", errors.New("no command provided")
	}

	resolvedPath, err := exec.LookPath(argv[0])
	if err != nil {
		return 0, "", "", err
	}
	currentUser, err := user.Current()
	if err != nil {
		return 0, "", "", err
	}
	hostname, err := os.Hostname()
	if err != nil {
		return 0, "", "", err
	}
	uid, _ := strconv.Atoi(currentUser.Uid)
	gid, _ := strconv.Atoi(currentUser.Gid)
	now := time.Now().UTC()

	req := model.NewRequest(
		"req-"+now.Format("20060102150405.000000000"),
		now,
		model.Requester{UID: uid, GID: gid, Username: currentUser.Username, Hostname: hostname},
		model.Command{ResolvedPath: resolvedPath, Argv: append([]string(nil), argv...), Cwd: cwd},
	)

	finalReq, err := approvalClient.CreateAndWait(ctx, req)
	if err != nil {
		return 0, "", "", err
	}
	if finalReq.Result == nil {
		return 1, "", "", fmt.Errorf("request %s", finalReq.Status)
	}
	result := finalReq.Result
	if result.Signal != 0 {
		return 128 + result.Signal, result.Stdout, result.Stderr, nil
	}
	return result.ExitCode, result.Stdout, result.Stderr, nil
}
