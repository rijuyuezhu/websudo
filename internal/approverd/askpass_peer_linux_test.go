package approverd

import (
	"errors"
	"net"
	"os"
	"testing"

	"websudo/internal/processauth"
)

func TestVerifyAskpassProcessChainAcceptsTrustedChain(t *testing.T) {
	uid := uint32(os.Getuid())
	verified := map[int]string{}
	inspector := askpassProcessInspector{
		peerCredentials: func(net.Conn) (processauth.PeerCredentials, error) {
			return processauth.PeerCredentials{PID: 100, UID: uid}, nil
		},
		sibling: func(name string) (string, error) {
			return "/trusted/" + name, nil
		},
		verifyExecutable: func(pid int, path string) error {
			verified[pid] = path
			return nil
		},
		parentPID: func(pid int) (int, error) {
			switch pid {
			case 100:
				return 200, nil
			case 200:
				return 300, nil
			default:
				return 0, errors.New("unexpected pid")
			}
		},
		processUIDs: func(pid int) (processauth.UIDs, error) {
			switch pid {
			case 200:
				return processauth.UIDs{Real: uid, Effective: 0}, nil
			case 300:
				return processauth.UIDs{Real: uid, Effective: uid}, nil
			default:
				return processauth.UIDs{}, errors.New("unexpected pid")
			}
		},
	}

	if err := verifyAskpassProcessChainWith(nil, inspector); err != nil {
		t.Fatalf("verifyAskpassProcessChainWith() error = %v", err)
	}
	if verified[100] != "/trusted/websudo-askpass" {
		t.Fatalf("askpass executable = %q", verified[100])
	}
	if verified[300] != "/trusted/websudo" {
		t.Fatalf("websudo executable = %q", verified[300])
	}
}

func TestVerifyAskpassProcessChainRejectsUnexpectedPeerUID(t *testing.T) {
	uid := uint32(os.Getuid())
	inspector := askpassProcessInspector{
		peerCredentials: func(net.Conn) (processauth.PeerCredentials, error) {
			return processauth.PeerCredentials{PID: 100, UID: uid + 1}, nil
		},
	}
	if err := verifyAskpassProcessChainWith(nil, inspector); err == nil {
		t.Fatal("verifyAskpassProcessChainWith() error = nil, want uid rejection")
	}
}

func TestVerifyAskpassProcessChainRejectsNonPrivilegedSudoParent(t *testing.T) {
	uid := uint32(os.Getuid())
	inspector := trustedChainInspector(uid)
	inspector.processUIDs = func(pid int) (processauth.UIDs, error) {
		if pid == 200 {
			return processauth.UIDs{Real: uid, Effective: uid}, nil
		}
		return processauth.UIDs{Real: uid, Effective: uid}, nil
	}
	if err := verifyAskpassProcessChainWith(nil, inspector); err == nil {
		t.Fatal("verifyAskpassProcessChainWith() error = nil, want sudo credential rejection")
	}
}

func TestVerifyAskpassProcessChainRejectsUntrustedWebsudoExecutable(t *testing.T) {
	uid := uint32(os.Getuid())
	inspector := trustedChainInspector(uid)
	inspector.verifyExecutable = func(pid int, path string) error {
		if pid == 300 {
			return errors.New("not trusted")
		}
		return nil
	}
	if err := verifyAskpassProcessChainWith(nil, inspector); err == nil {
		t.Fatal("verifyAskpassProcessChainWith() error = nil, want websudo executable rejection")
	}
}

func trustedChainInspector(uid uint32) askpassProcessInspector {
	return askpassProcessInspector{
		peerCredentials: func(net.Conn) (processauth.PeerCredentials, error) {
			return processauth.PeerCredentials{PID: 100, UID: uid}, nil
		},
		sibling: func(name string) (string, error) {
			return "/trusted/" + name, nil
		},
		verifyExecutable: func(int, string) error { return nil },
		parentPID: func(pid int) (int, error) {
			switch pid {
			case 100:
				return 200, nil
			case 200:
				return 300, nil
			default:
				return 0, errors.New("unexpected pid")
			}
		},
		processUIDs: func(pid int) (processauth.UIDs, error) {
			switch pid {
			case 200:
				return processauth.UIDs{Real: uid, Effective: 0}, nil
			case 300:
				return processauth.UIDs{Real: uid, Effective: uid}, nil
			default:
				return processauth.UIDs{}, errors.New("unexpected pid")
			}
		},
	}
}
