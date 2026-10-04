package approverd

import (
	"errors"
	"fmt"
	"net"
	"os"

	"websudo/internal/processauth"
)

type askpassProcessInspector struct {
	peerCredentials  func(net.Conn) (processauth.PeerCredentials, error)
	sibling          func(string) (string, error)
	verifyExecutable func(int, string) error
	parentPID        func(int) (int, error)
	processUIDs      func(int) (processauth.UIDs, error)
	commandLine      func(int) ([]string, error)
	workingDirectory func(int) (string, error)
}

func defaultAskpassProcessInspector() askpassProcessInspector {
	return askpassProcessInspector{
		peerCredentials:  processauth.UnixPeerCredentials,
		sibling:          processauth.SiblingExecutable,
		verifyExecutable: processauth.VerifyExecutable,
		parentPID:        processauth.ParentPID,
		processUIDs:      processauth.ProcessUIDs,
		commandLine:      processauth.CommandLine,
		workingDirectory: processauth.WorkingDirectory,
	}
}

func verifyAskpassProcessChain(conn net.Conn) (AskpassProvenance, error) {
	return verifyAskpassProcessChainWith(conn, defaultAskpassProcessInspector())
}

func verifyAskpassProcessChainWith(conn net.Conn, inspector askpassProcessInspector) (AskpassProvenance, error) {
	cred, err := inspector.peerCredentials(conn)
	if err != nil {
		return AskpassProvenance{}, err
	}
	currentUID := uint32(os.Getuid())
	if cred.UID != currentUID {
		return AskpassProvenance{}, errors.New("askpass peer has unexpected uid")
	}

	askpassPath, err := inspector.sibling("websudo-askpass")
	if err != nil {
		return AskpassProvenance{}, err
	}
	if err := inspector.verifyExecutable(cred.PID, askpassPath); err != nil {
		return AskpassProvenance{}, fmt.Errorf("verify askpass executable: %w", err)
	}

	sudoPID, err := inspector.parentPID(cred.PID)
	if err != nil {
		return AskpassProvenance{}, err
	}
	sudoUIDs, err := inspector.processUIDs(sudoPID)
	if err != nil {
		return AskpassProvenance{}, err
	}
	if sudoUIDs.Real != currentUID || sudoUIDs.Effective != 0 {
		return AskpassProvenance{}, errors.New("askpass parent does not have sudo credentials")
	}

	websudoPID, err := inspector.parentPID(sudoPID)
	if err != nil {
		return AskpassProvenance{}, err
	}
	websudoPath, err := inspector.sibling("websudo")
	if err != nil {
		return AskpassProvenance{}, err
	}
	if err := inspector.verifyExecutable(websudoPID, websudoPath); err != nil {
		return AskpassProvenance{}, fmt.Errorf("verify websudo executable: %w", err)
	}
	websudoUIDs, err := inspector.processUIDs(websudoPID)
	if err != nil {
		return AskpassProvenance{}, err
	}
	if websudoUIDs.Real != currentUID || websudoUIDs.Effective != currentUID {
		return AskpassProvenance{}, errors.New("sudo parent does not have websudo credentials")
	}

	commandLine, err := inspector.commandLine(websudoPID)
	if err != nil {
		return AskpassProvenance{}, err
	}
	if len(commandLine) < 2 {
		return AskpassProvenance{}, errors.New("websudo invocation has no command arguments")
	}
	cwd, err := inspector.workingDirectory(websudoPID)
	if err != nil {
		return AskpassProvenance{}, err
	}
	return AskpassProvenance{
		Command: append([]string(nil), commandLine[1:]...),
		CWD:     cwd,
	}, nil
}
