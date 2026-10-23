#!/bin/sh
# Re-verify a Luuna Atlas release archive before anyone extracts or ships it.
#
#   sh scripts/release/check-archive.sh [--require-exec] ARCHIVE
#
# Checks, in order, without extracting anything until the listing is clean:
#   1. file name is luuna-atlas_<version>_<darwin|linux>_<arm64|amd64>.tar.gz
#   2. SHA256SUMS beside the archive exists (regular file) and holds exactly
#      one matching entry for it
#   3. the entry list is exactly bin/lat and bin/luuna-atlas (rejects absolute
#      paths, "..", extra or missing entries)
#   4. both entries are regular files with mode 0755 (rejects links, devices)
#   5. after extraction to a private temp dir: regular, non-link, executable
#   6. on a matching host: `--version` and `--help` exit 0 for both names and
#      the alias prints the same output as the primary (arguments preserved)
# --require-exec fails when step 6 cannot run because the host does not match.
set -eu

die() {
	printf 'check-archive.sh: %s\n' "$1" >&2
	exit "${2:-1}"
}

require_exec=false
archive=""
while [ "$#" -gt 0 ]; do
	case "$1" in
	--require-exec)
		require_exec=true
		shift
		;;
	-h | --help)
		sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	-*) die "unknown option: $1" 2 ;;
	*)
		[ -z "$archive" ] || die "exactly one archive is accepted" 2
		archive=$1
		shift
		;;
	esac
done
[ -n "$archive" ] || die "usage: check-archive.sh [--require-exec] ARCHIVE" 2
[ -f "$archive" ] && [ ! -L "$archive" ] || die "not a regular file: $archive" 2

name=$(basename -- "$archive")
printf '%s\n' "$name" | grep -Eq '^luuna-atlas_[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?_(darwin|linux)_(arm64|amd64)\.tar\.gz$' ||
	die "unexpected archive name: $name"
rest=${name%.tar.gz}
arch=${rest##*_}
rest=${rest%_*}
os=${rest##*_}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "sha256sum or shasum is required"
	fi
}

dir=$(CDPATH='' cd -- "$(dirname -- "$archive")" && pwd -P)
sums=$dir/SHA256SUMS
# The checksum gate is mandatory: a missing, linked or special sums file fails.
[ -f "$sums" ] && [ ! -L "$sums" ] || die "missing SHA256SUMS beside $name"
actual=$(sha256_of "$archive")
matches=$(awk -v n="$name" 'NF == 2 && $2 == n { c++ } END { print c + 0 }' "$sums") ||
	die "cannot read SHA256SUMS beside $name"
[ "$matches" -ne 0 ] || die "SHA256SUMS has no entry for $name"
[ "$matches" -eq 1 ] || die "SHA256SUMS has $matches entries for $name"
expected=$(awk -v n="$name" 'NF == 2 && $2 == n { print $1 }' "$sums")
[ "$expected" = "$actual" ] || die "checksum mismatch for $name"
echo "ok checksum $actual"

listing=$(tar -tzf "$archive") || die "cannot list archive"
names=$(printf '%s\n' "$listing" | LC_ALL=C sort | tr '\n' ' ')
[ "$names" = "bin/lat bin/luuna-atlas " ] || die "unexpected entries: $names"
echo "ok entries bin/lat bin/luuna-atlas"

verbose=$(tar -tzvf "$archive") || die "cannot list archive"
[ "$(printf '%s\n' "$verbose" | wc -l | tr -d ' ')" = 2 ] || die "unexpected verbose listing"
printf '%s\n' "$verbose" | while IFS= read -r line; do
	mode=$(printf '%s' "$line" | awk '{print $1}')
	[ "$mode" = "-rwxr-xr-x" ] || die "entry is not a 0755 regular file: $line"
done
echo "ok modes regular 0755"

work=$(mktemp -d "${TMPDIR:-/tmp}/luuna-atlas-check.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir "$work/x" "$work/home"
tar -xzf "$archive" -C "$work/x" bin/lat bin/luuna-atlas
for exe in luuna-atlas lat; do
	p=$work/x/bin/$exe
	[ -f "$p" ] && [ ! -L "$p" ] && [ -x "$p" ] || die "extracted $exe is not a regular executable file"
done
cmp -s "$work/x/bin/luuna-atlas" "$work/x/bin/lat" || die "lat differs from luuna-atlas"
echo "ok extracted executables"

case "$(uname -s)" in Darwin) host_os=darwin ;; Linux) host_os=linux ;; *) host_os=other ;; esac
case "$(uname -m)" in arm64 | aarch64) host_arch=arm64 ;; x86_64 | amd64) host_arch=amd64 ;; *) host_arch=other ;; esac
if [ "$host_os" != "$os" ] || [ "$host_arch" != "$arch" ]; then
	if $require_exec; then
		die "archive is $os/$arch but host is $host_os/$host_arch; cannot execute"
	fi
	echo "skip exec: archive is $os/$arch, host is $host_os/$host_arch"
	echo "PASS $name (static checks only)"
	exit 0
fi

# Run inside an empty sandbox so no real project store or home is touched.
run() {
	(cd "$work/home" && HOME="$work/home" XDG_CONFIG_HOME="$work/home/.config" NO_COLOR=1 "$@")
}
for flag in --version --help; do
	a=$(run "$work/x/bin/luuna-atlas" "$flag") || die "luuna-atlas $flag failed"
	b=$(run "$work/x/bin/lat" "$flag") || die "lat $flag failed"
	[ -n "$a" ] || die "luuna-atlas $flag printed nothing"
	[ "$a" = "$b" ] || die "lat $flag output differs from luuna-atlas"
done
[ -z "$(ls -A "$work/home")" ] || die "executables wrote files into the sandbox home"
echo "ok exec --version/--help ($(run "$work/x/bin/lat" --version | head -1))"
echo "PASS $name"
