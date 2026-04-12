# websudo

Local browser approval for root commands.

## Commands

```sh
websudo -v
websudo /usr/bin/true
paru --sudo websudo -Syu
```

`websudo -v` refreshes the approval timestamp for the current TTY. Successful approvals are cached per TTY for 5 minutes by default, similar to `sudo`.

## Manual Test

1. Start `websudo-rootd` as root.
2. Start `websudo-approverd` as your user.
3. Run `websudo /usr/bin/true`.
4. Open `http://127.0.0.1:17878`.
5. Approve the pending request with your configured token.
