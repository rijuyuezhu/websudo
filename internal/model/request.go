package model

import (
	"encoding/json"
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

type Result struct {
	ExitCode int    `json:"exitCode"`
	Signal   int    `json:"signal,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

type Request struct {
	id          string
	createdAt   time.Time
	requestedBy Requester
	command     Command
	status      Status
	result      *Result
}

func NewRequest(id string, createdAt time.Time, requestedBy Requester, command Command) Request {
	return newRequest(id, createdAt, requestedBy, command, StatusPending, nil)
}

func NewStoredRequest(id string, createdAt time.Time, requestedBy Requester, command Command, status Status, result *Result) Request {
	return newRequest(id, createdAt, requestedBy, command, status, result)
}

func newRequest(id string, createdAt time.Time, requestedBy Requester, command Command, status Status, result *Result) Request {
	var resultCopy *Result
	if result != nil {
		copied := *result
		resultCopy = &copied
	}
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
		result: resultCopy,
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

func (r Request) Result() *Result {
	if r.result == nil {
		return nil
	}
	result := *r.result
	return &result
}

func (r Request) Transition(next Status) (Request, error) {
	if !canTransition(r.status, next) {
		return Request{}, fmt.Errorf("invalid request transition: %s -> %s", r.status, next)
	}

	r.status = next
	return r, nil
}

func (r Request) WithResult(result Result) (Request, error) {
	next := StatusSucceeded
	if result.ExitCode != 0 || result.Signal != 0 {
		next = StatusFailed
	}
	if !canTransition(r.status, next) {
		return Request{}, fmt.Errorf("invalid request transition: %s -> %s", r.status, next)
	}
	r.status = next
	resultCopy := result
	r.result = &resultCopy
	return r, nil
}

func (r Request) MarshalJSON() ([]byte, error) {
	type requestJSON struct {
		ID          string    `json:"id"`
		CreatedAt   time.Time `json:"createdAt"`
		RequestedBy Requester `json:"requestedBy"`
		Command     Command   `json:"command"`
		Status      Status    `json:"status"`
		Result      *Result   `json:"result,omitempty"`
	}
	return json.Marshal(requestJSON{
		ID:          r.id,
		CreatedAt:   r.createdAt,
		RequestedBy: r.requestedBy,
		Command:     r.Command(),
		Status:      r.status,
		Result:      r.Result(),
	})
}

func (r *Request) UnmarshalJSON(data []byte) error {
	type requestJSON struct {
		ID          string    `json:"id"`
		CreatedAt   time.Time `json:"createdAt"`
		RequestedBy Requester `json:"requestedBy"`
		Command     Command   `json:"command"`
		Status      Status    `json:"status"`
		Result      *Result   `json:"result"`
	}
	var payload requestJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	*r = newRequest(payload.ID, payload.CreatedAt, payload.RequestedBy, payload.Command, payload.Status, payload.Result)
	return nil
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
