#!/bin/sh
set -eu

die() {
	printf '%s\n' "$*" >&2
	exit 1
}

usage() {
	die "Usage: $0 <debian|fedora> <expected-architecture-substring>"
}

if [ "$#" -ne 2 ]; then
	usage
fi

distro=$1
expected_arch=$2
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
		install_cmd='apt-get update; DEBIAN_FRONTEND=noninteractive apt-get install -y file; set -- /dist/*.deb; [ -e "$1" ] || { printf "%s\n" "No .deb package found in /dist" >&2; exit 1; }; DEBIAN_FRONTEND=noninteractive apt-get install -y "$1"'
		;;
	fedora)
		image=${image:-fedora:latest}
		install_cmd='dnf -y install file; set -- /dist/*.rpm; [ -e "$1" ] || { printf "%s\n" "No .rpm package found in /dist" >&2; exit 1; }; dnf -y install "$1"'
		;;
	*)
		usage
		;;
esac

docker run \
	--rm \
	--env "WEBSUDO_EXPECTED_ARCH=$expected_arch" \
	--volume "${dist_dir}:/dist:ro" \
	--volume "${verifier}:${verifier_target}:ro" \
	"$image" sh -eu -c "$install_cmd; exec sh '$verifier_target' \"\$WEBSUDO_EXPECTED_ARCH\""

printf '%s\n' "Package verification passed in ${distro} container (${image})."
