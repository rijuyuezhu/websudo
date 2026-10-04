#!/bin/sh
set -eu

die() {
	printf '%s\n' "$*" >&2
	exit 1
}

if [ "$#" -ne 1 ]; then
	die "Usage: $0 <expected-architecture-substring>"
fi

expected_arch=$1
user_unit=/usr/lib/systemd/user/websudo-approverd.service
env_example=/etc/websudo/websudo.env.example
readme=/usr/share/websudo/README.md

check_binary() {
	path=$1
	[ -x "$path" ] || die "Expected executable not found: $path"
	output=$(file "$path")
	case "$output" in
		*"$expected_arch"*) ;;
		*) die "file output for ${path} did not contain ${expected_arch}: ${output}" ;;
	esac
	printf '%s\n' "$output"
}

check_binary /usr/bin/websudo
check_binary /usr/bin/websudo-askpass
check_binary /usr/bin/websudo-approverd

[ -f "$user_unit" ] || die "User unit not found: $user_unit"
[ -f "$env_example" ] || die "Environment example not found: $env_example"
[ -f "$readme" ] || die "README not found: $readme"
grep -Fq 'WEBSUDO_WEB_ADDR=127.0.0.1:17878' "$env_example" || die "Environment example missing WEBSUDO_WEB_ADDR default"
grep -Fq 'systemctl --user enable --now websudo-approverd.service' "$readme" || die "README missing user service setup command"
grep -Fq 'systemctl --user try-restart websudo-approverd.service' "$readme" || die "README missing user service upgrade command"
grep -Fq 'systemctl --user disable --now websudo-approverd.service' "$readme" || die "README missing user service uninstall command"

runtime_dir=$(mktemp -d)
trap 'rm -rf "$runtime_dir"' EXIT HUP INT TERM
XDG_RUNTIME_DIR="$runtime_dir" systemd-analyze --user verify "$user_unit"

if command -v dpkg-query >/dev/null 2>&1; then
	depends=$(dpkg-query -W -f='${Depends}' websudo)
	case "$depends" in
		*"sudo | sudo-rs"*) ;;
		*) die "Deb package dependency does not allow sudo or sudo-rs: $depends" ;;
	esac
elif command -v rpm >/dev/null 2>&1; then
	rpm -q --requires websudo | grep -Fxq '(sudo or sudo-rs)' || die 'RPM dependency does not allow sudo or sudo-rs'
fi

remove_package() {
	if command -v apt-get >/dev/null 2>&1; then
		DEBIAN_FRONTEND=noninteractive apt-get remove -y websudo
		return
	fi
	if command -v dnf >/dev/null 2>&1; then
		dnf -y remove websudo
		return
	fi
	die 'No supported package manager found for removal verification'
}

remove_package

[ ! -e /usr/bin/websudo ] || die 'websudo binary remained after package removal'
[ ! -e "$user_unit" ] || die "User unit remained after package removal: $user_unit"
[ ! -e "$readme" ] || die "README remained after package removal: $readme"

printf '%s\n' "Installed package verification passed for ${expected_arch}."
