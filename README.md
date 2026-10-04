# websudo

Local browser askpass helper for sudo commands.

## Commands

```sh
websudo -v
websudo /usr/bin/true
paru --sudo websudo -Syu
```

`websudo` executes commands through the system `sudo` binary using `sudo -A`. Sudo still owns sudoers policy, PAM authentication, timestamp caching, environment handling, and command execution.

If sudo's timestamp cache is fresh, no browser prompt appears. If sudo needs a password, it invokes `websudo-askpass`; the helper creates a local browser prompt through `websudo-approverd` and prints the submitted password back to sudo for PAM validation.

## Configuration

All websudo binaries read optional configuration directly from `/etc/websudo/websudo.env`. Packages install an example template at `/etc/websudo/websudo.env.example`.

Supported keys:

```env
WEBSUDO_WEB_ADDR=127.0.0.1:17878
WEBSUDO_APPROVAL_TIMEOUT_SECONDS=600
WEBSUDO_SUDO_PATH=/usr/bin/sudo
```

The file is the configuration source for these values; per-process environment variables do not override it. Keep the file administrator-controlled. `WEBSUDO_SUDO_PATH` must be an absolute path and may point to any deliberately selected sudo-compatible executable, including sudo-rs.

## Manual Test

1. Install frontend dependencies once with `npm install --prefix web`.
2. Build the project with `just build`.
3. Start `build/websudo-approverd` as your user.
4. Open `http://127.0.0.1:17878`.
5. Log in with the current machine password. The browser session lasts up to 72 hours or until logout.
6. Run `build/websudo -v` or `build/websudo /usr/bin/true` in a terminal. `websudo` uses the `websudo-askpass` binary built alongside it.
7. If sudo needs a password, approve the prompt in the web UI and submit the sudo password.
8. Use `Logout` in the web UI to clear the browser session.
