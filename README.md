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

1. Build and install `websudo`, `websudo-askpass`, and `websudo-approverd` in `PATH`.
2. Start `websudo-approverd` as your user.
3. Run `websudo -v` or `websudo /usr/bin/true` in a terminal.
4. If sudo needs a password, open `http://127.0.0.1:17878` or the `/askpass/<id>` URL printed by `websudo-askpass`.
5. Enter your system sudo password. The password is delivered once to sudo and is not stored in SQLite.

## Legacy Root Executor

`websudo-rootd` and the old fixed-token root execution flow are legacy implementation pieces. The default PoC path does not use them.
