# Sudo Askpass PoC Design

Date: 2026-06-01

## Summary

This PoC changes `websudo` from a self-managed root executor into a wrapper around the system `sudo` command. The goal is to reuse sudo's existing authorization, PAM authentication, timestamp cache, environment handling, logging, and command execution instead of duplicating those security-sensitive pieces in Go.

The new path uses `sudo -A` and a `websudo-askpass` helper. When sudo's timestamp cache is fresh, sudo executes the command without invoking the helper and no web UI appears. When sudo needs a password, it invokes `websudo-askpass`, which asks the local `websudo-approverd` service to present a browser prompt. The user enters the system sudo password in the browser. The helper prints that password to stdout for sudo, and sudo/PAM validates it.

The PoC intentionally does not implement cookies, login sessions, or a new Vue frontend. Those features are deferred and must remain convenience/access-control layers around the browser UI, not replacements for sudo/PAM authentication.

## Goals

- Make `websudo <command> [args...]` execute through the system `sudo` binary.
- Make `websudo -v` map to sudo's native validation flow.
- Use `SUDO_ASKPASS` and `sudo -A` when sudo needs a password.
- Preserve sudoers policy, PAM authentication, sudo timestamp behavior, command execution semantics, and exit codes.
- Remove the new execution path's dependency on `websudo-rootd` and the custom tty timestamp cache.
- Keep the PoC small enough to test without requiring real root execution in automated tests.

## Non-Goals

- Do not implement cookie login or persistent browser sessions in this PoC.
- Do not replace the HTML UI with Vue or another frontend framework in this PoC.
- Do not implement a polkit or `pkexec` integration in this PoC.
- Do not authenticate with the existing fixed six-digit websudo token.
- Do not store system passwords, password prompts, or password responses in SQLite.
- Do not keep using `websudo-rootd` for the new default execution path.

## Chosen Approach

Use sudo askpass:

```text
websudo command
  -> sudo -A -- command
    -> sudoers policy and sudo timestamp check
    -> if password is required:
       -> websudo-askpass "<sudo prompt>"
          -> create pending in-memory prompt in approverd
          -> browser submits system sudo password
          -> helper prints password to stdout
    -> sudo/PAM authenticates
    -> sudo executes command
```

This is preferred over `pkexec` because this project is specifically a sudo-compatible helper for workflows such as `paru --sudo websudo`. `pkexec` is action-oriented and polkit-based; it is better suited for fixed privileged helpers than arbitrary command execution. It would also replace sudoers and sudo timestamp semantics instead of reusing them.

This is preferred over a sudo approval plugin for the PoC because askpass is much smaller to build and deploy. A sudo plugin remains future work if websudo needs deeper integration into sudo's policy pipeline.

## Components

### `cmd/websudo`

`websudo` becomes a sudo wrapper. It should:

- Parse the existing command shape, including `-v`.
- Resolve the askpass helper path from configuration or a default binary name.
- Execute the configured sudo binary with `-A`.
- For command execution, pass arguments as `sudo -A -- <command> [args...]`.
- For validation, pass arguments as `sudo -A -v`.
- Set `SUDO_ASKPASS` for the sudo child process.
- Wire stdin, stdout, and stderr directly to the caller.
- Return sudo's exit status, including signal-style exits where practical.

`websudo` should not create approval requests or invoke `websudo-rootd` in the new path.

### `cmd/websudo-askpass`

`websudo-askpass` is the helper process invoked by sudo. It should:

- Accept the sudo prompt passed by sudo as its first argument.
- Send a new askpass request to `websudo-approverd` over the local HTTP API.
- Wait for completion until approval timeout or context cancellation.
- Print the submitted password to stdout on success.
- Print no password on rejection, timeout, or service error.
- Exit non-zero on rejection, timeout, or service error.

The helper must not validate the password. Sudo/PAM remains the only password authority.

### `cmd/websudo-approverd`

The approval daemon remains a normal user HTTP service bound to `127.0.0.1` by default. For the PoC it gains an in-memory askpass store and UI endpoints. It should:

- List pending password prompts.
- Render a prompt detail page with a password input.
- Accept one password response per askpass request.
- Support rejection so the helper can fail without printing a password.
- Expire pending prompts using `ApprovalTimeoutSeconds`.
- Never persist passwords or completed password responses.

The existing SQLite request history may remain for legacy code during the PoC, but the askpass flow must not store password data in SQLite.

### Legacy Components

`websudo-rootd`, `rootd`, custom request approval execution, and custom tty timestamp cache are legacy for this PoC. They may remain in the repository temporarily to reduce the first change size, but the default `websudo` path must not use them. A future cleanup can delete or fully isolate them after the askpass flow is validated.

## Data Model

Add an in-memory askpass request model with fields equivalent to:

```text
id: random opaque identifier
prompt: sudo prompt string
createdAt: UTC timestamp
status: pending | completed | denied | expired
password: present only in memory until consumed by websudo-askpass
```

IDs must be generated by `websudo-approverd`, not supplied by the helper. Passwords are write-once and read-once. After `websudo-askpass` consumes a completed request, the password should be removed from the store.

## HTTP Flow

The PoC can use small JSON endpoints and basic server-rendered HTML:

- `POST /api/askpass` creates a pending prompt and returns its id.
- `GET /api/askpass/:id` returns status without password data.
- `POST /api/askpass/:id/complete` accepts the password from the browser.
- `POST /api/askpass/:id/deny` rejects the prompt.
- `GET /askpass/:id` renders the prompt page.
- `GET /` may include pending askpass prompts alongside or instead of legacy requests.

No GET endpoint may return a password. Completion endpoints should not echo submitted passwords.

## Error Handling

- If `websudo-approverd` is unavailable, `websudo-askpass` exits non-zero and prints no password.
- If the browser rejects the request, `websudo-askpass` exits non-zero and prints no password.
- If the request expires, `websudo-askpass` exits non-zero and prints no password.
- If sudo rejects the password, sudo may invoke askpass again; each invocation creates a new web prompt.
- If sudoers rejects a command, websudo does not bypass the rejection.
- If sudo's timestamp cache is fresh, sudo does not invoke askpass and no web prompt appears.

## Security Boundaries

- Sudo remains the only component that authorizes and executes root commands.
- PAM remains the only component that validates the system password.
- `websudo-approverd` is not root and does not execute commands.
- Passwords are held only in process memory long enough to deliver them from the browser to the askpass helper.
- Passwords must not be logged, stored in SQLite, exposed through status APIs, or included in request history.
- Browser login, cookies, and future frontend work are access-control and UX features only; they must not replace sudo/PAM.

## Configuration

Add or reserve configuration fields for:

- sudo binary path, default `/usr/bin/sudo`.
- askpass helper path, default resolved from `websudo-askpass` in `PATH` or a configured absolute path.
- approverd base URL, derived from `WebAddr` as `http://<WebAddr>`.
- askpass poll interval, default 250 milliseconds to match the existing approverd client polling cadence.

The existing `ApprovalTimeoutSeconds` controls askpass request expiration.

## Testing Strategy

Automated tests must not require real sudo or root access.

- Unit test the sudo runner with a fake command executable that records argv and environment, proving `SUDO_ASKPASS` and `sudo -A` are used.
- Unit test `websudo -v` mapping to `sudo -A -v`.
- Unit test exit code propagation from the sudo child process.
- Unit test the askpass client success path: completed request causes the helper logic to return only the password.
- Unit test askpass rejection, timeout, and approverd unavailable paths: no password is returned.
- Handler-test askpass prompt rendering, completion, denial, expiration, one-time password consumption, and absence of passwords from status responses.
- Keep legacy tests passing until legacy code is intentionally deleted or isolated in a future cleanup task.

## Migration Notes

- Users must have `sudo` installed and configured.
- Users must run `websudo-approverd` before sudo needs a password through `websudo`.
- `websudo-rootd` systemd units are no longer required for the PoC path and should be documented as deprecated.
- Existing custom timestamp files are ignored by the new path. Sudo's own timestamp controls reauthentication.
- Existing fixed-token approval is not used by the new default path.

## Future Work

- Add cookie-backed login to the web UI so an already logged-in browser can approve prompts more quickly.
- Replace the minimal HTML with Vue or another frontend framework.
- Add CSRF, Origin checks, and rate limiting before exposing richer browser workflows.
- Remove or fully quarantine legacy rootd/custom timestamp code after the sudo askpass path is stable.
- Consider a sudo approval plugin if future requirements need approval to be represented inside sudo's plugin pipeline.
- Consider a separate polkit authentication agent only if the project expands beyond sudo-compatible command execution.
