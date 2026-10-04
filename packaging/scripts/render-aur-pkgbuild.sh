#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 4 ]; then
	printf 'Usage: %s <version> <x86_64-sha256> <aarch64-sha256> <output>\n' "$0" >&2
	exit 1
fi

version=$1
x86_64_sha256=$2
aarch64_sha256=$3
output=$4

template=$(CDPATH='' cd "$(dirname "$0")/../aur" && pwd -P)/PKGBUILD.template

while IFS= read -r line; do
	line=${line//@PKGVER@/$version}
	line=${line//@TARBALL_SHA256_X86_64@/$x86_64_sha256}
	line=${line//@TARBALL_SHA256_AARCH64@/$aarch64_sha256}
	printf '%s\n' "$line"
done < "$template" > "$output"
