package model

import (
	"fmt"
	"time"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusApproved  Status = "approved"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusDenied    Status = "denied"
	StatusExpired   Status = "expired"
)

type Requester struct {
	UID      int
	GID      int
	Username string
	Hostname string
}

type Command struct {
	ResolvedPath string
	Argv         []string
	Cwd          string
}

type Request struct {
	id          string
	createdAt   time.Time
	requestedBy Requester
	command     Command
	status      Status
}

func NewRequest(id string, createdAt time.Time, requestedBy Requester, command Command) Request {
	return newRequest(id, createdAt, requestedBy, command, StatusPending)
}

func NewStoredRequest(id string, createdAt time.Time, requestedBy Requester, command Command, status Status) Request {
	return newRequest(id, createdAt, requestedBy, command, status)
}

func newRequest(id string, createdAt time.Time, requestedBy Requester, command Command, status Status) Request {
	return Request{
		id:          id,
		createdAt:   createdAt,
		requestedBy: requestedBy,
		command: Command{
			ResolvedPath: command.ResolvedPath,
			Argv:         append([]string(nil), command.Argv...),
			Cwd:          command.Cwd,
		},
		status: status,
	}
}

func (r Request) ID() string {
	return r.id
}

func (r Request) CreatedAt() time.Time {
	return r.createdAt
}

func (r Request) RequestedBy() Requester {
	return r.requestedBy
}

func (r Request) Command() Command {
	return Command{
		ResolvedPath: r.command.ResolvedPath,
		Argv:         append([]string(nil), r.command.Argv...),
		Cwd:          r.command.Cwd,
	}
}

func (r Request) Status() Status {
	return r.status
}

func (r Request) Transition(next Status) (Request, error) {
	if !canTransition(r.status, next) {
		return Request{}, fmt.Errorf("invalid request transition: %s -> %s", r.status, next)
	}

	r.status = next
	return r, nil
}

func canTransition(from, to Status) bool {
	switch from {
	case StatusPending:
		return to == StatusApproved || to == StatusDenied || to == StatusExpired
	case StatusApproved:
		return to == StatusRunning
	case StatusRunning:
		return to == StatusSucceeded || to == StatusFailed
	default:
		return false
	}
}
