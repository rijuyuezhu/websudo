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

## Manual Test

1. Install frontend dependencies once with `npm install --prefix web`.
2. Build the project with `just build`.
3. Start `build/websudo-approverd` as your user.
4. Open `http://127.0.0.1:17878`.
5. Log in with the current machine password. The browser session lasts up to 72 hours or until logout.
6. Run `WEBSUDO_ASKPASS_PATH="$PWD/build/websudo-askpass" build/websudo -v` or `WEBSUDO_ASKPASS_PATH="$PWD/build/websudo-askpass" build/websudo /usr/bin/true` in a terminal.
7. If sudo needs a password, approve the prompt in the web UI and submit the sudo password.
8. Use `Logout` in the web UI to clear the browser session.

## Legacy Root Executor

`websudo-rootd` and the old fixed-token root execution flow are legacy implementation pieces. The default PoC path does not use them.
