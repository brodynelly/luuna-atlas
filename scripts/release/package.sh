#!/bin/sh
# Build one host-native public archive of Luuna Atlas. Nothing is published.
#
#   sh scripts/release/package.sh --version 0.0.0-local --out DIR [--os OS --arch ARCH]
#
# Writes into DIR:
#   luuna-atlas_<version>_<os>_<arch>.tar.gz   entries: bin/luuna-atlas, bin/lat (regular, 0755)
#   SHA256SUMS                                  archive, LICENSE, UPSTREAM.md
#   provenance.json                             self-reported build metadata (not an attestation)
#   LICENSE, UPSTREAM.md                        attribution that accompanies the archive
#
# The archive version (--version) is independent of the internal CLI version
# (`const Version` in internal/atlas/app.go). That symbol is a Go constant, so it
# cannot be overridden with -ldflags -X and the script does not try to.
set -eu

usage() {
	cat <<'EOF'
Usage: sh scripts/release/package.sh --version VERSION --out DIR [--os OS] [--arch ARCH]

  --version VERSION  archive version: MAJOR.MINOR.PATCH with optional -prerelease
                     (letters, digits, dots, hyphens); no leading "v"
  --out DIR          output directory (created if missing; must not be a symlink)
  --os OS            darwin | linux (default: host)
  --arch ARCH        arm64 | amd64 (default: host)
  --require-clean    fail unless the checkout is a clean Git work tree
                     (tracked changes or untracked files make it dirty)
  -h, --help         show this help

Builds with the local Go toolchain only (GOTOOLCHAIN=local, GOPROXY=off,
-mod=vendor, CGO_ENABLED=0). Cross targets build but are not executed here.
EOF
}

die() {
	printf 'package.sh: %s\n' "$1" >&2
	exit "${2:-1}"
}

version=""
out=""
target_os=""
target_arch=""
require_clean=false
while [ "$#" -gt 0 ]; do
	case "$1" in
	--version)
		[ "$#" -ge 2 ] || die "--version needs a value" 2
		version=$2
		shift 2
		;;
	--out)
		[ "$#" -ge 2 ] || die "--out needs a value" 2
		out=$2
		shift 2
		;;
	--os)
		[ "$#" -ge 2 ] || die "--os needs a value" 2
		target_os=$2
		shift 2
		;;
	--arch)
		[ "$#" -ge 2 ] || die "--arch needs a value" 2
		target_arch=$2
		shift 2
		;;
	--require-clean)
		require_clean=true
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*) die "unknown argument: $1 (see --help)" 2 ;;
	esac
done

[ -n "$version" ] || die "--version is required" 2
[ -n "$out" ] || die "--out is required" 2

# Strict version grammar: keeps the value safe in file names, JSON and shells.
if ! printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$'; then
	die "invalid --version '$version' (expected MAJOR.MINOR.PATCH[-prerelease])" 2
fi
[ "$(printf '%s' "$version" | wc -c | tr -d ' ')" -le 64 ] || die "--version is too long" 2

host_os() {
	case "$(uname -s)" in
	Darwin) echo darwin ;;
	Linux) echo linux ;;
	*) echo unsupported ;;
	esac
}
host_arch() {
	case "$(uname -m)" in
	arm64 | aarch64) echo arm64 ;;
	x86_64 | amd64) echo amd64 ;;
	*) echo unsupported ;;
	esac
}
[ -n "$target_os" ] || target_os=$(host_os)
[ -n "$target_arch" ] || target_arch=$(host_arch)
case "$target_os" in darwin | linux) ;; *) die "unsupported --os '$target_os' (darwin|linux)" 2 ;; esac
case "$target_arch" in arm64 | amd64) ;; *) die "unsupported --arch '$target_arch' (arm64|amd64)" 2 ;; esac

case "$out" in *'
'*) die "--out must not contain a newline" 2 ;; esac
[ ! -L "$out" ] || die "--out must not be a symlink: $out" 2
if [ -e "$out" ] && [ ! -d "$out" ]; then
	die "--out exists and is not a directory: $out" 2
fi

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
repo=$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)
[ -f "$repo/go.mod" ] && [ -d "$repo/cmd/luuna-atlas" ] && [ -d "$repo/vendor" ] ||
	die "cannot locate the luuna-atlas checkout from $script_dir"

command -v go >/dev/null 2>&1 || die "go is not on PATH (Go 1.27.x required; nothing is downloaded)"
command -v tar >/dev/null 2>&1 || die "tar is required"
command -v gzip >/dev/null 2>&1 || die "gzip is required"

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "sha256sum or shasum is required"
	fi
}

# JSON string literal for metadata values (escapes \ and ", drops control chars).
json_str() {
	printf '"%s"' "$(printf '%s' "$1" | tr -d '\000-\037' | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')"
}
json_or_null() {
	if [ -n "$1" ]; then json_str "$1"; else printf 'null'; fi
}

internal_version=$(sed -n 's/^const Version = "\([^"]*\)"$/\1/p' "$repo/internal/atlas/app.go")
[ -n "$internal_version" ] || die "could not read const Version from internal/atlas/app.go"

archive_name="luuna-atlas_${version}_${target_os}_${target_arch}.tar.gz"

[ -f "$script_dir/worktree-state.sh" ] || die "missing $script_dir/worktree-state.sh"
# Recorded before the build so that nothing this script writes can affect it.
dirty=$(sh "$script_dir/worktree-state.sh" "$repo")
case "$dirty" in true | false | null) ;; *) die "unexpected work-tree state: $dirty" ;; esac
if $require_clean && [ "$dirty" != false ]; then
	die "--require-clean: work tree is not clean (worktree_dirty=$dirty); commit, stash or remove untracked files"
fi

mkdir -p "$out"
out=$(CDPATH='' cd -- "$out" && pwd -P)
[ ! -e "$out/$archive_name" ] || die "refusing to overwrite existing $out/$archive_name"

umask 022
# touch -t reads local time; a fixed zone keeps the archive mtimes host-independent.
TZ=UTC0
export TZ
stage=$(mktemp -d "${TMPDIR:-/tmp}/luuna-atlas-package.XXXXXX")
trap 'rm -rf "$stage"' EXIT HUP INT TERM
mkdir "$stage/bin" "$stage/gocache"

export GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=vendor GOWORK=off
export CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch"
ldflags="-s -w -buildid="
go_version=$(go env GOVERSION)

printf 'Building luuna-atlas %s for %s/%s with %s\n' "$version" "$target_os" "$target_arch" "$go_version"
(cd "$repo" && go build -mod=vendor -trimpath -ldflags "$ldflags" -o "$stage/bin/luuna-atlas" ./cmd/luuna-atlas)
# The CLI reads only os.Args[1:] and never branches on argv[0], so the public
# alias is a byte-identical copy that preserves every argument.
cp "$stage/bin/luuna-atlas" "$stage/bin/lat"
chmod 0755 "$stage/bin/luuna-atlas" "$stage/bin/lat"
# Fixed timestamps (2000-01-01T00:00:00Z under TZ=UTC0) keep the archive
# reproducible for identical binaries.
touch -t 200001010000 "$stage/bin/luuna-atlas" "$stage/bin/lat"

# Only the two files are archived; no directory, link or metadata entries.
# One invocation per tar flavour, errors visible; there is no fallback that
# could drop the owner or metadata flags.
if tar --version 2>/dev/null | grep -q 'GNU tar'; then
	(cd "$stage" && tar --format=ustar --owner=0 --group=0 --numeric-owner \
		-cf "$stage/archive.tar" bin/lat bin/luuna-atlas) ||
		die "tar failed to create the archive"
else
	(cd "$stage" && COPYFILE_DISABLE=1 tar --format=ustar --uid 0 --gid 0 --uname root --gname wheel \
		--no-xattrs --no-acls --no-fflags -cf "$stage/archive.tar" bin/lat bin/luuna-atlas) ||
		die "tar failed to create the archive (bsdtar with --uname/--gname/--no-xattrs/--no-acls/--no-fflags is required)"
fi
gzip -n -9 -c "$stage/archive.tar" >"$stage/$archive_name" || die "gzip failed"

listing=$(tar -tzf "$stage/$archive_name") || die "cannot list the built archive"
entries=$(printf '%s\n' "$listing" | LC_ALL=C sort | tr '\n' ' ')
[ "$entries" = "bin/lat bin/luuna-atlas " ] || die "unexpected archive entries: $entries"

# Both entries must be 0755 regular files owned by uid/gid 0 (root). GNU tar
# lists "mode 0/0 size ..."; bsdtar lists "mode links owner group size ...".
verbose=$(tar -tzvf "$stage/$archive_name") || die "cannot list the built archive"
printf '%s\n' "$verbose" | awk '
	{
		n++
		if (index($2, "/") > 0) { split($2, og, "/"); u = og[1]; g = og[2] } else { u = $3; g = $4 }
		if ($1 != "-rwxr-xr-x") bad = 1
		if (u != "0" && u != "root") bad = 1
		if (g != "0" && g != "root" && g != "wheel") bad = 1
	}
	END { exit (bad || n != 2) ? 1 : 0 }
' || die "built archive entries are not 0755 regular files owned by root: $(printf '%s' "$verbose" | tr '\n' ';')"

mv "$stage/$archive_name" "$out/$archive_name"
cp "$repo/LICENSE" "$out/LICENSE"
cp "$repo/UPSTREAM.md" "$out/UPSTREAM.md"

archive_sha=$(sha256_of "$out/$archive_name")
license_sha=$(sha256_of "$out/LICENSE")
upstream_sha=$(sha256_of "$out/UPSTREAM.md")
{
	printf '%s  %s\n' "$archive_sha" "$archive_name"
	printf '%s  %s\n' "$license_sha" "LICENSE"
	printf '%s  %s\n' "$upstream_sha" "UPSTREAM.md"
} >"$out/SHA256SUMS"

commit=""
ref=""
if command -v git >/dev/null 2>&1 && git -C "$repo" rev-parse --git-dir >/dev/null 2>&1; then
	commit=$(git -C "$repo" rev-parse HEAD 2>/dev/null || true)
	ref=$(git -C "$repo" symbolic-ref -q HEAD 2>/dev/null || true)
fi
[ -z "${GITHUB_SHA:-}" ] || commit=$GITHUB_SHA
[ -z "${GITHUB_REF:-}" ] || ref=$GITHUB_REF

{
	printf '{\n'
	printf '  "schema": "luuna-component-provenance/1",\n'
	printf '  "component": "atlas",\n'
	printf '  "archive_version": %s,\n' "$(json_str "$version")"
	printf '  "internal_version": %s,\n' "$(json_str "$internal_version")"
	printf '  "internal_version_source": "internal/atlas/app.go const Version (not set by ldflags)",\n'
	printf '  "repository": %s,\n' "$(json_or_null "${GITHUB_REPOSITORY:-}")"
	printf '  "commit": %s,\n' "$(json_or_null "$commit")"
	printf '  "ref": %s,\n' "$(json_or_null "$ref")"
	printf '  "worktree_dirty": %s,\n' "$dirty"
	printf '  "workflow_run_id": %s,\n' "$(json_or_null "${GITHUB_RUN_ID:-}")"
	printf '  "workflow_run_attempt": %s,\n' "$(json_or_null "${GITHUB_RUN_ATTEMPT:-}")"
	printf '  "runner": {"os": %s, "arch": %s, "uname": %s},\n' \
		"$(json_or_null "${RUNNER_OS:-}")" "$(json_or_null "${RUNNER_ARCH:-}")" "$(json_str "$(uname -sm)")"
	printf '  "target": {"os": %s, "arch": %s},\n' "$(json_str "$target_os")" "$(json_str "$target_arch")"
	printf '  "toolchain": {"go": %s, "go_mod_directive": %s},\n' \
		"$(json_str "$go_version")" "$(json_str "$(sed -n 's/^go //p' "$repo/go.mod")")"
	printf '  "build": {"package": "./cmd/luuna-atlas", "env": "CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=vendor", "flags": %s},\n' \
		"$(json_str "-mod=vendor -trimpath -ldflags '$ldflags'")"
	printf '  "executables": ["luuna-atlas", "lat"],\n'
	printf '  "alias_strategy": "lat is a byte-identical copy; the CLI does not branch on argv[0]",\n'
	printf '  "archive": {"name": %s, "sha256": %s},\n' "$(json_str "$archive_name")" "$(json_str "$archive_sha")"
	printf '  "attribution": {"LICENSE": %s, "UPSTREAM.md": %s},\n' "$(json_str "$license_sha")" "$(json_str "$upstream_sha")"
	printf '  "attestation": null,\n'
	printf '  "note": "Self-reported metadata. Not signed, not attested, not published."\n'
	printf '}\n'
} >"$out/provenance.json"

printf 'Wrote %s\n' "$out/$archive_name"
printf 'sha256 %s\n' "$archive_sha"
