#!/bin/sh
set -eu

die() {
	printf '%s\n' "$*" >&2
	exit 1
}

usage() {
	die "Usage: $0 <debian|fedora|arch> <expected-architecture-substring> [image]"
}

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
	usage
fi

distro=$1
expected_arch=$2
image=${3:-}

command -v docker >/dev/null 2>&1 || die "Required command not found: docker"

dist_dir=${WEBSUDO_DIST_DIR:-dist}
case "$dist_dir" in
	/*) ;;
	*) dist_dir=$(pwd -P)/$dist_dir ;;
esac
[ -d "$dist_dir" ] || die "Dist directory not found: $dist_dir"

script_dir=$(CDPATH= cd "$(dirname "$0")" && pwd -P)
verifier=${script_dir}/verify-installed-package.sh
verifier_target=/var/tmp/verify-installed-package.sh
[ -f "$verifier" ] || die "Verifier script not found: $verifier"

case "$distro" in
	debian)
		image=${image:-debian:bookworm}
		boot_cmd='apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y systemd systemd-sysv dbus && exec /sbin/init'
		install_cmd='apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates file sudo passwd && set -- /dist/*.deb; [ -e "$1" ] || { printf "%s\n" "No .deb package found in /dist" >&2; exit 1; }; DEBIAN_FRONTEND=noninteractive apt-get install -y "$1"'
		;;
	fedora)
		image=${image:-fedora:latest}
		boot_cmd='dnf -y install systemd dbus && exec /sbin/init'
		install_cmd='dnf -y install file sudo shadow-utils && set -- /dist/*.rpm; [ -e "$1" ] || { printf "%s\n" "No .rpm package found in /dist" >&2; exit 1; }; dnf -y install "$1"'
		;;
	arch)
		image=${image:-archlinux:latest}
		boot_cmd='pacman -Syu --noconfirm --needed systemd dbus && exec /usr/lib/systemd/systemd'
		install_cmd='pacman -Syu --noconfirm --needed file sudo shadow && set -- /dist/*.pkg.tar.*; [ -e "$1" ] || { printf "%s\n" "No Arch package found in /dist" >&2; exit 1; }; pacman -U --noconfirm "$1"'
		;;
	*)
		usage
		;;
esac

container="websudo-package-${distro}-$$"

cleanup() {
	docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

docker run \
	--detach \
	--name "$container" \
	--privileged \
	--cgroupns=host \
	--stop-signal SIGRTMIN+3 \
	--volume /sys/fs/cgroup:/sys/fs/cgroup:rw \
	--volume "${dist_dir}:/dist:ro" \
	--tmpfs /run \
	--tmpfs /run/lock \
	--tmpfs /tmp \
	"$image" sh -c "$boot_cmd" >/dev/null

pid1_comm=
status=
i=0
while [ "$i" -lt 300 ]; do
	pid1_comm=$(docker exec "$container" sh -c 'cat /proc/1/comm 2>/dev/null || true' 2>/dev/null || true)
	status=$(docker exec "$container" systemctl is-system-running 2>/dev/null || true)
	if [ "$pid1_comm" = systemd ]; then
		case "$status" in
			running|degraded) break ;;
		esac
	fi
	i=$((i + 1))
	sleep 1
done

if [ "$pid1_comm" != systemd ]; then
	printf '%s\n' "Last PID1 command: ${pid1_comm:-unavailable}" >&2
	printf '%s\n' "Last systemctl state: ${status:-unavailable}" >&2
	printf '%s\n' "Docker logs:" >&2
	docker logs "$container" >&2 || true
	die "Container systemd did not become ready"
fi

case "$status" in
	running|degraded) ;;
	*)
		printf '%s\n' "Last PID1 command: ${pid1_comm:-unavailable}" >&2
		printf '%s\n' "Last systemctl state: ${status:-unavailable}" >&2
		printf '%s\n' "Docker logs:" >&2
		docker logs "$container" >&2 || true
		die "Container systemd did not become ready"
		;;
esac

docker exec "$container" sh -eu -c "$install_cmd"
docker cp "$verifier" "${container}:${verifier_target}"
docker exec "$container" chmod +x "$verifier_target"
docker exec "$container" sh -eu -c 'mkdir -p /run/user/0 && chmod 700 /run/user/0'
docker exec "$container" env XDG_RUNTIME_DIR=/run/user/0 "$verifier_target" "$expected_arch"

printf '%s\n' "Package verification passed in ${distro} container (${image})."
