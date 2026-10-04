package processauth

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUnixPeerCredentialsReturnsCurrentProcessForLocalSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "peer.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer func() { _ = listener.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	client, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer func() { _ = client.Close() }()
	server := <-accepted
	defer func() { _ = server.Close() }()

	cred, err := UnixPeerCredentials(server)
	if err != nil {
		t.Fatalf("UnixPeerCredentials() error = %v", err)
	}
	if cred.PID != os.Getpid() {
		t.Fatalf("peer pid = %d, want %d", cred.PID, os.Getpid())
	}
	if cred.UID != uint32(os.Getuid()) {
		t.Fatalf("peer uid = %d, want %d", cred.UID, os.Getuid())
	}
}

func TestVerifyExecutableAcceptsCurrentProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() error = %v", err)
	}
	if err := VerifyExecutable(os.Getpid(), executable); err != nil {
		t.Fatalf("VerifyExecutable() error = %v", err)
	}
}

func TestVerifyExecutableRejectsDifferentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other")
	if err := os.WriteFile(path, []byte("other"), 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := VerifyExecutable(os.Getpid(), path); err == nil {
		t.Fatal("VerifyExecutable() error = nil, want mismatch")
	}
}

func TestParseParentPIDHandlesSpacesAndParentheses(t *testing.T) {
	parentPID, err := ParseParentPID("123 (name with ) paren) S 456 1 1 0")
	if err != nil {
		t.Fatalf("ParseParentPID() error = %v", err)
	}
	if parentPID != 456 {
		t.Fatalf("parent pid = %d, want 456", parentPID)
	}
}

func TestProcessUIDsReturnsCurrentUser(t *testing.T) {
	uids, err := ProcessUIDs(os.Getpid())
	if err != nil {
		t.Fatalf("ProcessUIDs() error = %v", err)
	}
	if uids.Real != uint32(os.Getuid()) || uids.Effective != uint32(os.Geteuid()) {
		t.Fatalf("uids = %#v, want real=%d effective=%d", uids, os.Getuid(), os.Geteuid())
	}
}

func TestParseCommandLine(t *testing.T) {
	got, err := ParseCommandLine([]byte("/usr/bin/websudo\x00/usr/bin/id\x00\x00"))
	if err != nil {
		t.Fatalf("ParseCommandLine() error = %v", err)
	}
	want := []string{"/usr/bin/websudo", "/usr/bin/id", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseCommandLine() = %#v, want %#v", got, want)
	}
}

func TestCommandLineAndWorkingDirectoryForCurrentProcess(t *testing.T) {
	argv, err := CommandLine(os.Getpid())
	if err != nil {
		t.Fatalf("CommandLine() error = %v", err)
	}
	if len(argv) == 0 || argv[0] == "" {
		t.Fatalf("CommandLine() = %#v, want non-empty argv", argv)
	}

	cwd, err := WorkingDirectory(os.Getpid())
	if err != nil {
		t.Fatalf("WorkingDirectory() error = %v", err)
	}
	if !filepath.IsAbs(cwd) {
		t.Fatalf("WorkingDirectory() = %q, want absolute path", cwd)
	}
}
