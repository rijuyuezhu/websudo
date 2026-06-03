#!/bin/sh
set -eu

die() {
	printf '%s\n' "$*" >&2
	exit 1
}

require_contains() {
	file=$1
	pattern=$2
	description=$3

	grep -Fq "$pattern" "$file" || die "${file} missing ${description}: ${pattern}"
}

install_script=packaging/aur/websudo-bin.install
pkgbuild_template=packaging/aur/PKGBUILD.template
workflow=.github/workflows/ci.yml

[ -f "$install_script" ] || die "AUR install script not found: ${install_script}"

require_contains "$pkgbuild_template" 'install=websudo-bin.install' 'AUR install hook reference'
require_contains "$install_script" 'post_install()' 'post-install hook'
require_contains "$install_script" 'post_upgrade()' 'post-upgrade hook'
require_contains "$install_script" 'websudo-systemd-setup' 'systemd setup helper reminder'
require_contains "$workflow" 'packaging/aur/websudo-bin.install' 'AUR install script copy step'

printf '%s\n' 'AUR package metadata verification passed.'
