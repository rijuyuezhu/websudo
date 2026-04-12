# websudo Design

Date: 2026-04-12

## Summary

`websudo` is a local approval-based privilege escalation tool for Linux.

It is not a general-purpose `sudo` replacement. The primary goal is to let commands that expect a `sudo`-like helper, especially `paru --sudo websudo`, block on a local approval workflow instead of waiting for terminal password entry. A user reviews pending root requests in a web UI and approves or rejects each one. Approval requires entering a fixed short token, default length 6, that is stored in local configuration and does not rotate per request.

The first version runs entirely on the local machine. It does not depend on Tailscale, reverse proxies, or any external identity provider. The web UI binds to `127.0.0.1` only. Remote access, if needed, will be added later outside this design.

## Goals

- Support `websudo <command> [args...]`.
- Support `paru --sudo websudo`.
- Replace interactive terminal password entry with a local approval workflow.
- Let the approver inspect the exact frozen command before approving.
- Return stdout, stderr, and exit status back to the original caller so tools like `paru` behave normally.
- Keep the implementation small and locally debuggable.

## Non-Goals

- Replace system `sudo` for all use cases.
- Integrate with polkit or `pkexec` in v1.
- Provide strong multi-factor or per-request challenge authentication.
- Expose the approval UI to the network in v1.
- Enforce an allowlist in v1.

## Background And Constraints

The target workflow is a Linux machine that already runs package management and other root-requiring commands locally, but may be operated from a remote context where entering a password into the original terminal is inconvenient or impossible. `paru` supports `--sudo <file>`, so a compatible wrapper command is sufficient for the initial package management use case.

The user explicitly prefers:

- a dedicated tool rather than modifying system `sudo`
- support for direct use as `websudo command`
- support for `paru --sudo websudo`
- a review UI that shows pending requests and full command details
- a fixed short token, default 6 digits, used in the UI to confirm approvals
- accepting some additional security risk in exchange for remote convenience
- no allowlist in the first version

## User-Facing Behavior

### CLI Entry Point

`websudo` is a normal executable in `PATH`.

Supported usage:

```sh
websudo <command> [args...]
paru --sudo websudo
```

High-level behavior:

1. The caller invokes `websudo` with a command.
2. `websudo` freezes the request details.
3. `websudo` submits the request to the local approval service.
4. `websudo` waits for the final result.
5. The user opens the local web UI, inspects the request, and approves or rejects it.
6. On approval, the root executor runs the exact frozen request.
7. `websudo` relays stdout, stderr, and exit status back to the original caller.

### Web UI

The local approval UI shows a queue of pending requests and recent completed requests.

For each request the UI shows:

- request id
- status
- requested executable path
- full argument vector
- working directory
- requesting user
- requesting uid
- hostname
- submit time
- optional short command preview

Approval UX:

- user opens a request details page or expanded row
- user reviews the frozen command
- user clicks approve or reject
- approve requires entering the configured token
- reject does not require the token

The UI never allows editing the command contents.

## Architecture

The system is split into three local components.

### 1. `websudo` CLI shim

Responsibilities:

- validate CLI usage
- resolve the requested executable path
- capture argv, cwd, uid, gid, username, hostname, environment subset if needed
- create a frozen request payload
- submit the payload to the approval service
- wait for completion
- stream or replay command output to the caller
- exit with the same final status as the executed root command

It does not run commands as root itself.

### 2. Approval service

Runs as the normal user.

Responsibilities:

- expose a local HTTP API bound to `127.0.0.1`
- serve the approval UI
- store request state
- validate the fixed token for approval actions
- forward approved requests to the root executor
- store and expose command results
- notify waiting `websudo` clients when state changes

This service is the orchestration layer. It knows about web requests and pending approvals, but it is never trusted to run root commands directly.

### 3. Root executor

Runs as root.

Responsibilities:

- listen on a local Unix domain socket only
- accept execution requests only from the local approval service path and peer credentials
- execute the exact frozen command payload
- capture stdout, stderr, exit status, and signal termination
- return results to the approval service

This component has no web logic and no HTML/UI surface.

## Communication Model

### `websudo` to approval service

Transport: local HTTP on `127.0.0.1`.

Endpoints:

- `POST /api/requests` to create a request
- `GET /api/requests/:id` to fetch status
- optional `GET /api/requests/:id/events` for SSE updates

For v1, long polling is acceptable if it keeps the implementation simpler. SSE is preferred if it stays small.

### Approval service to root executor

Transport: Unix domain socket.

Reasoning:

- no TCP port exposure
- clean local trust boundary
- straightforward permission control on the socket path
- can verify peer credentials on Linux

### Approval service to browser

Transport: local HTTP.

No network-facing bind in v1. The service listens on `127.0.0.1` only.

## Request Model

Each request is immutable once created, except for its lifecycle state and execution result.

Suggested fields:

```json
{
  "id": "uuid-or-nanoid",
  "status": "pending",
  "createdAt": "2026-04-12T03:00:00Z",
  "requestedBy": {
    "uid": 1000,
    "gid": 1000,
    "username": "rijuyuezhu",
    "hostname": "rjyz-linux"
  },
  "command": {
    "resolvedPath": "/usr/bin/pacman",
    "argv": ["/usr/bin/pacman", "-U", "/tmp/pkg.tar.zst"],
    "cwd": "/home/rijuyuezhu/Code/foo"
  },
  "approval": {
    "approvedBy": null,
    "approvedAt": null,
    "rejectedAt": null
  },
  "result": null
}
```

Important property: the command payload is frozen at request creation time and cannot be modified from the UI.

## State Machine

Request states:

- `pending`
- `approved`
- `running`
- `succeeded`
- `failed`
- `denied`
- `expired`

Transitions:

- `pending -> approved`
- `pending -> denied`
- `pending -> expired`
- `approved -> running`
- `running -> succeeded`
- `running -> failed`

Notes:

- `approved` is a short transitional state before execution starts.
- `failed` covers both non-zero exit codes and infrastructure-level execution failures.
- command exit detail should distinguish process failure from executor failure in metadata, even if both surface as non-zero to the caller.

## Token Model

The approval token is a fixed locally configured secret used by the UI to confirm approval actions.

Properties:

- default length: 6 characters
- configurable value and length
- does not rotate per request by default
- not printed to the terminal
- stored locally by the approval service configuration

Security posture:

- this token is a convenience confirmation secret, not strong identity proof
- anyone who can reach the UI and knows the token can approve requests
- this is an accepted v1 trade-off

Storage recommendation:

- store a hash of the token instead of raw plaintext when practical
- compare using constant-time comparison

Even though the user accepts additional risk, hashed storage is a low-cost improvement and should be included.

## Output Handling

The executed root command must behave as normally as possible to the original caller.

Required behavior:

- preserve stdout
- preserve stderr
- preserve exit code

Implementation options:

1. Buffer full stdout/stderr in the approval service and replay on completion.
2. Stream stdout/stderr incrementally from root executor to approval service to `websudo`.

Recommendation: start with buffered output if that keeps v1 smaller. For `paru` compatibility, buffered output is acceptable as long as command success and failure propagate correctly. If buffering causes poor UX for long-running operations, streaming can be added in v2.

## Failure Semantics

### Approval service unavailable

- `websudo` exits immediately with non-zero status
- stderr reports that the approval service is unavailable
- no fallback to system `sudo`

### Root executor unavailable

- approval may still appear in the UI
- approval action fails when dispatching execution
- final request state becomes `failed`
- `websudo` exits non-zero and surfaces the infrastructure error

### Approval timeout

- pending requests expire automatically after a configurable timeout
- default timeout should be long enough for remote approval, such as 10 minutes
- expired requests transition to `expired`
- `websudo` exits non-zero

### Manual rejection

- request transitions to `denied`
- `websudo` exits non-zero

### Command exits non-zero

- request transitions to `failed`
- recorded metadata includes the exact exit code
- `websudo` exits with that same code

### Command terminated by signal

- request transitions to `failed`
- metadata records the signal
- `websudo` returns `128 + signal` to match common shell behavior

## Security Model

This tool intentionally accepts more risk than normal `sudo` password entry, but the design still keeps the trust boundaries explicit.

### Boundaries

- browser/UI layer is not root
- approval service is not root
- root executor is root but has no network listener
- only the root executor actually launches privileged commands

### Main risks accepted in v1

- no allowlist: any command can be submitted for approval
- fixed approval token: repeated use increases exposure if observed or leaked
- local-only UI is assumed to be protected by whatever outer access method the operator chooses later

### Minimum mitigations kept in v1

- immutable frozen request payload
- UI cannot alter commands
- Unix socket for root executor
- token hash storage
- request audit records

## Audit And Persistence

The approval service should persist request history to disk so the UI survives restart and provides a minimal audit trail.

Suggested persisted data:

- request metadata
- approval or rejection timestamps
- requester information
- token approval success event
- execution result

SQLite is the preferred v1 storage format because it keeps the implementation simple and avoids ad hoc JSON file mutation races.

Data retention can be simple in v1:

- retain recent history until the user clears it or a future retention setting is added

## Configuration

Suggested config file locations:

- user service config: `~/.config/websudo/config.toml`
- root executor config: `/etc/websudo/config.toml` only if needed

Minimum user-configurable settings:

- web bind address, default `127.0.0.1`
- web port
- approval timeout
- fixed token hash
- token length metadata or display hint
- request history retention limit if implemented

## Packaging And Repository Layout

Project path:

- repository root: `~/Code/websudo`
- design docs live under that repository

Suggested repository structure:

```text
websudo/
  cmd/
    websudo/
    websudo-approverd/
    websudo-rootd/
  internal/
  docs/
    superpowers/
      specs/
        2026-04-12-websudo-design.md
```

The exact language is not fixed by this design, but the structure assumes a small multi-binary local service project.

## Testing Strategy

Testing needs to focus on behavior boundaries rather than UI polish.

### Unit tests

- CLI parsing for direct invocation and `paru`-compatible invocation
- request state transitions
- token hash verification
- timeout handling
- exit code mapping

### Integration tests

- create request via CLI and approve via HTTP API
- reject a request and verify caller exit code
- executor unavailable path
- command non-zero exit path
- signal termination path if feasible

### Manual tests

- `websudo /usr/bin/true`
- `websudo /usr/bin/false`
- `paru --sudo websudo -S <pkg>` on a safe package operation
- approval from browser while caller waits in another terminal or remote session

## Open Questions Deferred Out Of Scope

- whether to stream stdout/stderr live in v1 or only replay on completion
- whether to add remote exposure helpers such as Tailscale or SSH port forwarding
- whether to add optional per-request token rotation or WebAuthn later
- whether to add command templates, favorites, or an allowlist in v2
- whether to support multi-user separation on a shared machine

## Recommended Implementation Direction

Build the smallest local-first version that proves the end-to-end path:

1. `websudo` CLI shim that can wrap a command and wait.
2. Local approval service with pending/completed UI.
3. Root executor over Unix socket.
4. Fixed-token approval flow.
5. Buffered stdout/stderr replay and exit-code preservation.

This keeps the initial scope aligned with the actual problem: making `paru --sudo websudo` and `websudo command` usable in remote scenarios where terminal password entry is inconvenient.
