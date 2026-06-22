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
test_uid=${WEBSUDO_TEST_UID:-1001}
test_user=${WEBSUDO_TEST_USER:-websudo-test}
user_unit=/usr/lib/systemd/user/websudo-approverd.service
env_example=/etc/websudo/websudo.env.example

if getent passwd "$test_user" >/dev/null 2>&1; then
	actual_uid=$(id -u "$test_user")
	if [ "$actual_uid" != "$test_uid" ]; then
		die "User ${test_user} has uid ${actual_uid}, expected ${test_uid}"
	fi
else
	useradd -u "$test_uid" -m -s /bin/sh "$test_user"
fi

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

setup=/usr/bin/websudo-systemd-setup
[ -x "$setup" ] || die "Expected executable not found: $setup"
setup_output=$(file "$setup")
case "$setup_output" in
	*"shell script"*|*"text executable"*) ;;
	*) die "file output for ${setup} did not describe an executable shell script: ${setup_output}" ;;
esac
printf '%s\n' "$setup_output"

[ -f "$user_unit" ] || die "User unit not found: $user_unit"
[ -f "$env_example" ] || die "Environment example not found: $env_example"
grep -Fq 'WEBSUDO_WEB_ADDR=127.0.0.1:17878' "$env_example" || die "Environment example missing WEBSUDO_WEB_ADDR default"

systemd-analyze --user verify "$user_unit"

remove_package() {
	if command -v apt-get >/dev/null 2>&1; then
		DEBIAN_FRONTEND=noninteractive apt-get remove -y websudo
		return
	fi
	if command -v dnf >/dev/null 2>&1; then
		dnf -y remove websudo
		return
	fi
	if command -v pacman >/dev/null 2>&1; then
		pacman -R --noconfirm websudo
		return
	fi
	die 'No supported package manager found for removal verification'
}

remove_package

[ ! -e /usr/bin/websudo ] || die 'websudo binary remained after package removal'
[ ! -e "$user_unit" ] || die "User unit remained after package removal: $user_unit"

printf '%s\n' "Installed package verification passed for ${expected_arch}."
