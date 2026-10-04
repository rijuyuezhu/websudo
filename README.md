# websudo

Local browser askpass helper for sudo commands.

## Commands

```sh
websudo -v
websudo /usr/bin/true
paru --sudo websudo -Syu
```

`websudo` executes commands through an explicitly selected sudo-compatible executable using `-A`. The selected implementation still owns sudoers policy, PAM authentication, timestamp caching, environment handling, and command execution. Packages do not force a particular sudo implementation: use traditional sudo or sudo-rs, and set `WEBSUDO_SUDO_PATH` when the executable is not `/usr/bin/sudo`.

If the selected implementation's timestamp cache is fresh, no browser prompt appears. If it needs a password, it invokes `websudo-askpass`; the helper creates a local browser prompt through `websudo-approverd` and prints the submitted password back for PAM validation.

## Configuration

All websudo binaries read optional configuration directly from `/etc/websudo/websudo.env`. Packages install an example template at `/etc/websudo/websudo.env.example`.

Supported keys:

```env
WEBSUDO_WEB_ADDR=127.0.0.1:17878
WEBSUDO_APPROVAL_TIMEOUT_SECONDS=600
WEBSUDO_SUDO_PATH=/usr/bin/sudo
```

The file is the configuration source for these values; per-process environment variables do not override it. Keep the file administrator-controlled. `WEBSUDO_SUDO_PATH` must be an absolute path and may point to any deliberately selected sudo-compatible executable, including sudo-rs.

## User Service

The canonical unit source is `packaging/systemd/websudo-approverd.service`; packages install it as `/usr/lib/systemd/user/websudo-approverd.service` and install this README as `/usr/share/websudo/README.md`. From the normal login session of the desktop user that will use websudo, enable it with:

```sh
systemctl --user daemon-reload
systemctl --user enable --now websudo-approverd.service
```

websudo does not require systemd user lingering; the approval daemon is intended to run with the user's login session.

After upgrading websudo, reload the unit and restart the daemon if it is currently running:

```sh
systemctl --user daemon-reload
systemctl --user try-restart websudo-approverd.service
```

Before uninstalling the package, disable the user service from that same session:

```sh
systemctl --user disable --now websudo-approverd.service
```

## Manual Test

1. Install frontend dependencies once with `npm install --prefix web`.
2. Build the project with `just build`.
3. Start `build/websudo-approverd` as your user.
4. Open `http://127.0.0.1:17878`.
5. Log in with the current machine password. The browser session lasts up to 72 hours or until logout.
6. Run `build/websudo -v` or `build/websudo /usr/bin/true` in a terminal. `websudo` uses the `websudo-askpass` binary built alongside it.
7. If the selected sudo-compatible executable needs a password, approve the prompt in the web UI and submit it.
8. Use `Logout` in the web UI to clear the browser session.
