package processauth

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type PeerCredentials struct {
	PID int
	UID uint32
	GID uint32
}

type UIDs struct {
	Real      uint32
	Effective uint32
}

func UnixPeerCredentials(conn net.Conn) (PeerCredentials, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return PeerCredentials{}, errors.New("connection is not a Unix socket")
	}

	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return PeerCredentials{}, fmt.Errorf("inspect Unix peer: %w", err)
	}
	var cred *syscall.Ucred
	var controlErr error
	if err := rawConn.Control(func(fd uintptr) {
		cred, controlErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return PeerCredentials{}, fmt.Errorf("inspect Unix socket: %w", err)
	}
	if controlErr != nil {
		return PeerCredentials{}, fmt.Errorf("inspect Unix peer credentials: %w", controlErr)
	}
	if cred == nil {
		return PeerCredentials{}, errors.New("unix peer credentials are unavailable")
	}
	return PeerCredentials{PID: int(cred.Pid), UID: cred.Uid, GID: cred.Gid}, nil
}

func SiblingExecutable(name string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolve current executable symlinks: %w", err)
	}

	sibling := filepath.Join(filepath.Dir(executable), name)
	sibling, err = filepath.EvalSymlinks(sibling)
	if err != nil {
		return "", fmt.Errorf("resolve sibling %s: %w", name, err)
	}
	info, err := os.Stat(sibling)
	if err != nil {
		return "", fmt.Errorf("inspect sibling %s: %w", name, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("sibling %s is not executable: %s", name, sibling)
	}
	return sibling, nil
}

func VerifyExecutable(pid int, expectedPath string) error {
	processInfo, err := os.Stat(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return fmt.Errorf("inspect process %d executable: %w", pid, err)
	}
	expectedInfo, err := os.Stat(expectedPath)
	if err != nil {
		return fmt.Errorf("inspect trusted executable %s: %w", expectedPath, err)
	}
	if !os.SameFile(processInfo, expectedInfo) {
		return fmt.Errorf("process %d executable is not trusted", pid)
	}
	return nil
}

func ParentPID(pid int) (int, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, fmt.Errorf("inspect process %d parent: %w", pid, err)
	}
	return ParseParentPID(string(data))
}

func ParseParentPID(text string) (int, error) {
	closeParen := strings.LastIndex(text, ") ")
	if closeParen < 0 {
		return 0, errors.New("invalid process stat")
	}
	fields := strings.Fields(text[closeParen+2:])
	if len(fields) < 2 {
		return 0, errors.New("invalid process stat")
	}
	parentPID, err := strconv.Atoi(fields[1])
	if err != nil || parentPID <= 0 {
		return 0, errors.New("invalid parent pid")
	}
	return parentPID, nil
}

func ProcessUIDs(pid int) (UIDs, error) {
	file, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return UIDs{}, fmt.Errorf("inspect process %d status: %w", pid, err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "Uid:"))
		if len(fields) < 2 {
			return UIDs{}, errors.New("invalid process uid status")
		}
		realUID, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil {
			return UIDs{}, errors.New("invalid process real uid")
		}
		effectiveUID, err := strconv.ParseUint(fields[1], 10, 32)
		if err != nil {
			return UIDs{}, errors.New("invalid process effective uid")
		}
		return UIDs{Real: uint32(realUID), Effective: uint32(effectiveUID)}, nil
	}
	if err := scanner.Err(); err != nil {
		return UIDs{}, fmt.Errorf("inspect process %d status: %w", pid, err)
	}
	return UIDs{}, errors.New("process uid status is missing")
}

func CommandLine(pid int) ([]string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return nil, fmt.Errorf("inspect process %d command line: %w", pid, err)
	}
	return ParseCommandLine(data)
}

func ParseCommandLine(data []byte) ([]string, error) {
	parts := strings.Split(string(data), "\x00")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 || parts[0] == "" {
		return nil, errors.New("process command line is empty")
	}
	return parts, nil
}

func WorkingDirectory(pid int) (string, error) {
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return "", fmt.Errorf("inspect process %d working directory: %w", pid, err)
	}
	if cwd == "" {
		return "", errors.New("process working directory is empty")
	}
	return cwd, nil
}
