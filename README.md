# websudo

websudo is a Linux sudo wrapper that moves password prompts from the terminal to a local web page. Release packages are available for x86_64 and ARM64.

websudo still uses your configured sudo program, so existing sudoers rules and credential caching continue to apply. If sudo already has a valid cached credential, no browser prompt is shown.

## Install

### Arch Linux

Install the AUR package:

```sh
paru -S websudo-bin
```

### Debian / Ubuntu / Fedora

Download the matching `.deb` or `.rpm` from [GitHub Releases](https://github.com/rijuyuezhu/websudo/releases) and install it with your system package manager.

### Enable the approval service

Run this from the normal desktop login session of the user who will use websudo:

```sh
systemctl --user daemon-reload
systemctl --user enable --now websudo-approverd.service
```

The approval page is available at `http://127.0.0.1:17878` by default.

## Use

Run commands through websudo the same way you would invoke a sudo wrapper:

```sh
websudo -v
websudo /usr/bin/true
websudo pacman -Syu
```

For paru:

```sh
paru --sudo websudo -Syu
```

When sudo needs a password, open the approval page, sign in with your machine password, review the command and working directory, and submit the password. The browser session lasts up to 72 hours or until you choose **Logout**.

websudo works with traditional sudo by default. To use sudo-rs or another deliberately selected sudo-compatible executable, configure `WEBSUDO_SUDO_PATH` as described below.

## Configure

Configuration is optional. If `/etc/websudo/websudo.env` does not exist, websudo uses these defaults:

```env
WEBSUDO_WEB_ADDR=127.0.0.1:17878
WEBSUDO_APPROVAL_TIMEOUT_SECONDS=600
WEBSUDO_SUDO_PATH=/usr/bin/sudo
```

Packages also install an example file at `/etc/websudo/websudo.env.example`.

Only these three keys are accepted:

- `WEBSUDO_WEB_ADDR` — numeric loopback address and non-zero port, such as `127.0.0.1:17878` or `[::1]:17878`.
- `WEBSUDO_APPROVAL_TIMEOUT_SECONDS` — positive integer number of seconds before an unanswered request expires.
- `WEBSUDO_SUDO_PATH` — absolute path to the sudo-compatible executable, for example `/usr/bin/sudo-rs`.

Keep `/etc/websudo/websudo.env` administrator-controlled. Unknown keys, malformed lines, and invalid values cause websudo to reject the configuration instead of silently falling back.

websudo itself only listens on loopback. If you expose the approval page through your own proxy, VPN, or Tailscale setup, keep the websudo backend on a loopback address; that external proxy setup is outside websudo.

After changing the configuration, restart the approval service:

```sh
systemctl --user restart websudo-approverd.service
```

## Upgrade

After upgrading the package, reload the user unit and restart the daemon if it is running:

```sh
systemctl --user daemon-reload
systemctl --user try-restart websudo-approverd.service
```

## Uninstall

Before removing the package, disable the user service from the same desktop login session:

```sh
systemctl --user disable --now websudo-approverd.service
```

Then remove the package with your normal package manager.

## Build from source

For development or manual testing, install Go 1.27+ and `just`, then run:

```sh
just build
build/websudo-approverd
```

Open `http://127.0.0.1:17878`, then use `build/websudo` from another terminal. For normal use, prefer an installed package.
