#!/bin/sh
# Offline tests for scripts/release/{package,check-archive}.sh.
#
#   sh scripts/release/test-release.sh [--skip-build]
#
# Uses generated stub archives (tiny #!/bin/sh executables) for check-archive
# cases and validates package.sh arguments before any build. The final case
# runs a real host build with the local Go toolchain unless --skip-build is
# given or go is missing. No network, Bun, or project store is used.
set -eu

skip_build=false
[ "${1:-}" != "--skip-build" ] || skip_build=true

here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)
pkg=$here/package.sh
chk=$here/check-archive.sh
T=$(mktemp -d "${TMPDIR:-/tmp}/luuna-atlas-release-test.XXXXXX")
trap 'rm -rf "$T"' EXIT HUP INT TERM

pass=0
fail=0
ok() {
	pass=$((pass + 1))
	printf 'ok %s\n' "$1"
}
not_ok() {
	fail=$((fail + 1))
	printf 'not ok %s\n' "$1"
	[ ! -s "$T/err" ] || sed 's/^/#   /' "$T/err"
}

# expect CODE NAME CMD...: run CMD and require exit status CODE ("nz" = any non-zero).
expect() {
	want=$1
	label=$2
	shift 2
	set +e
	"$@" >"$T/out" 2>"$T/err"
	got=$?
	set -e
	if [ "$want" = nz ] && [ "$got" -ne 0 ]; then
		ok "$label (exit $got)"
	elif [ "$want" = "$got" ]; then
		ok "$label"
	else
		not_ok "$label: expected exit $want, got $got"
	fi
}

case "$(uname -s)" in Darwin) host_os=darwin ;; Linux) host_os=linux ;; *) host_os=unknown ;; esac
case "$(uname -m)" in arm64 | aarch64) host_arch=arm64 ;; x86_64 | amd64) host_arch=amd64 ;; *) host_arch=unknown ;; esac
if [ "$host_os" = darwin ]; then other_os=linux; else other_os=darwin; fi

sha() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'; else shasum -a 256 "$1" | awk '{print $1}'; fi
}

# make_fixture DIR OS ARCH [MUTATION]: write a stub archive plus SHA256SUMS into DIR.
make_fixture() {
	d=$1
	fos=$2
	farch=$3
	mut=${4:-none}
	name=luuna-atlas_1.2.3-test_${fos}_${farch}.tar.gz
	src=$d.src
	rm -rf "$src"
	mkdir -p "$d" "$src/a/b/bin"
	cat >"$src/a/b/bin/luuna-atlas" <<'EOF'
#!/bin/sh
printf 'stub'
for a in "$@"; do printf ' [%s]' "$a"; done
printf '\n'
EOF
	cp "$src/a/b/bin/luuna-atlas" "$src/a/b/bin/lat"
	chmod 0755 "$src/a/b/bin/luuna-atlas" "$src/a/b/bin/lat"
	echo evil >"$src/a/evil"
	echo extra >"$src/a/b/bin/extra"
	files="bin/lat bin/luuna-atlas"
	flags=""
	case "$mut" in
	extra) files="$files bin/extra" ;;
	missing) files="bin/luuna-atlas" ;;
	traversal) files="$files bin/../../evil" flags=-P ;;
	absolute) files="bin/luuna-atlas $src/a/b/bin/lat" flags=-P ;;
	symlink) rm "$src/a/b/bin/lat" && ln -s luuna-atlas "$src/a/b/bin/lat" ;;
	hardlink) rm "$src/a/b/bin/lat" && ln "$src/a/b/bin/luuna-atlas" "$src/a/b/bin/lat" ;;
	mode644) chmod 0644 "$src/a/b/bin/lat" ;;
	differs) echo 'echo different' >>"$src/a/b/bin/lat" ;;
	esac
	# shellcheck disable=SC2086 # word splitting of the fixed file list is intended
	(cd "$src/a/b" && COPYFILE_DISABLE=1 tar $flags -czf "$d/$name" $files 2>/dev/null)
	printf '%s  %s\n' "$(sha "$d/$name")" "$name" >"$d/SHA256SUMS"
	[ "$mut" != badsum ] || printf '%064d  %s\n' 0 "$name" >"$d/SHA256SUMS"
	printf '%s\n' "$d/$name"
}

echo '# package.sh argument validation (no build happens)'
expect 0 "package --help" sh "$pkg" --help
expect 2 "package without arguments" sh "$pkg"
expect 2 "package --version without value" sh "$pkg" --version
expect 2 "package missing --out" sh "$pkg" --version 1.0.0
expect 2 "package missing --version" sh "$pkg" --out "$T/o"
for bad in v1.0.0 1.0 1.0.0- ../1.0.0 '1.0.0;id' '1.0.0 x' '1.0.0/..' '-1.0.0' 1.0.0-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa; do
	expect 2 "package rejects version '$bad'" sh "$pkg" --version "$bad" --out "$T/o"
done
expect 2 "package rejects --os windows" sh "$pkg" --version 1.0.0 --out "$T/o" --os windows
expect 2 "package rejects --arch riscv64" sh "$pkg" --version 1.0.0 --out "$T/o" --arch riscv64
expect 2 "package rejects unknown flag" sh "$pkg" --version 1.0.0 --out "$T/o" --publish
[ ! -e "$T/o" ] && ok "rejected runs created no output dir" || not_ok "rejected runs created $T/o"
mkdir "$T/real"
ln -s "$T/real" "$T/link"
expect 2 "package rejects symlink --out" sh "$pkg" --version 1.0.0 --out "$T/link"
: >"$T/file"
expect 2 "package rejects file --out" sh "$pkg" --version 1.0.0 --out "$T/file"
mkdir "$T/existing"
echo keep >"$T/existing/luuna-atlas_1.0.0_${host_os}_${host_arch}.tar.gz"
expect nz "package refuses to overwrite an existing archive" sh "$pkg" --version 1.0.0 --out "$T/existing"
[ "$(cat "$T/existing/luuna-atlas_1.0.0_${host_os}_${host_arch}.tar.gz")" = keep ] &&
	ok "existing archive left unchanged" || not_ok "existing archive was modified"
[ -z "$(ls -A "$T/real")" ] && ok "symlink target left empty" || not_ok "symlink target was written"

echo '# check-archive.sh fixtures'
expect 2 "check without archive" sh "$chk"
expect 2 "check unknown option" sh "$chk" --force x
a=$(make_fixture "$T/static" "$other_os" "$host_arch")
expect 0 "valid foreign-platform fixture passes static checks" sh "$chk" "$a"
expect 1 "--require-exec fails for a foreign platform" sh "$chk" --require-exec "$a"
if [ "$host_os" != unknown ] && [ "$host_arch" != unknown ]; then
	a=$(make_fixture "$T/host" "$host_os" "$host_arch")
	expect 0 "valid host fixture executes --version/--help via both names" sh "$chk" --require-exec "$a"
	a=$(make_fixture "$T/differs" "$host_os" "$host_arch" differs)
	expect 1 "alias that differs from primary is rejected" sh "$chk" "$a"
fi
for m in extra missing traversal absolute symlink hardlink mode644 badsum; do
	a=$(make_fixture "$T/bad-$m" "$other_os" "$host_arch" "$m")
	expect 1 "rejects $m archive" sh "$chk" "$a"
done
[ ! -e "$T/bad-traversal/evil" ] && [ ! -e "$T/evil" ] && ok "traversal fixture extracted nothing" ||
	not_ok "traversal fixture wrote outside its sandbox"
a=$(make_fixture "$T/renamed" "$other_os" "$host_arch")
mv "$a" "$T/renamed/luuna-atlas_1.2.3_windows_amd64.tar.gz"
expect 1 "rejects unsupported platform in name" sh "$chk" "$T/renamed/luuna-atlas_1.2.3_windows_amd64.tar.gz"
echo '# SHA256SUMS is mandatory'
a=$(make_fixture "$T/nosums" "$other_os" "$host_arch")
rm "$T/nosums/SHA256SUMS"
expect 1 "rejects an archive without SHA256SUMS" sh "$chk" "$a"
grep -q 'missing SHA256SUMS' "$T/err" && ok "missing SHA256SUMS is reported" || not_ok "missing SHA256SUMS message absent"
a=$(make_fixture "$T/sumslink" "$other_os" "$host_arch")
mv "$T/sumslink/SHA256SUMS" "$T/sumslink.real-sums"
ln -s "$T/sumslink.real-sums" "$T/sumslink/SHA256SUMS"
expect 1 "rejects a symlinked SHA256SUMS" sh "$chk" "$a"
a=$(make_fixture "$T/sumsdir" "$other_os" "$host_arch")
rm "$T/sumsdir/SHA256SUMS"
mkdir "$T/sumsdir/SHA256SUMS"
expect 1 "rejects a non-regular SHA256SUMS" sh "$chk" "$a"
a=$(make_fixture "$T/dupsum" "$other_os" "$host_arch")
sed -n 1p "$T/dupsum/SHA256SUMS" >>"$T/dupsum/SHA256SUMS"
expect 1 "rejects SHA256SUMS with a duplicate entry" sh "$chk" "$a"
a=$(make_fixture "$T/nosumentry" "$other_os" "$host_arch")
printf '%s  %s\n' "$(sha "$a")" "other.tar.gz" >"$T/nosumentry/SHA256SUMS"
expect 1 "rejects SHA256SUMS without an entry for the archive" sh "$chk" "$a"
a=$(make_fixture "$T/symarchive" "$other_os" "$host_arch")
ln -s "$a" "$T/symarchive/luuna-atlas_9.9.9_${other_os}_${host_arch}.tar.gz"
[ -f "$T/symarchive/SHA256SUMS" ] || not_ok "symlink fixture lost its SHA256SUMS"
expect 2 "rejects a symlinked archive path" sh "$chk" "$T/symarchive/luuna-atlas_9.9.9_${other_os}_${host_arch}.tar.gz"

echo '# worktree-state.sh (scratch repositories only)'
wts=$here/worktree-state.sh
expect 2 "worktree-state without a directory" sh "$wts"
mkdir -p "$T/emptybin" "$T/notrepo"
state() { GIT_CEILING_DIRECTORIES=$T sh "$wts" "$1"; }
[ "$(state "$T/notrepo")" = null ] && ok "non-repository reports null" || not_ok "non-repository did not report null"
[ "$(PATH=$T/emptybin /bin/sh "$wts" "$T/notrepo")" = null ] && ok "missing git reports null" ||
	not_ok "missing git did not report null"
if command -v git >/dev/null 2>&1; then
	r=$T/scratch
	g() { git -C "$r" -c user.name=release-test -c user.email=release-test@example.invalid -c core.hooksPath=/dev/null -c commit.gpgsign=false "$@"; }
	mkdir -p "$r/internal/atlas"
	git init -q "$r"
	echo 'package atlas' >"$r/internal/atlas/app.go"
	echo 'dist/' >"$r/.gitignore"
	g add -A && g commit -q -m init
	[ "$(state "$r")" = false ] && ok "clean scratch repo reports false" || not_ok "clean scratch repo not false"
	mkdir "$r/dist" && echo x >"$r/dist/out"
	[ "$(state "$r")" = false ] && ok "ignored output keeps the tree clean" || not_ok "ignored output made the tree dirty"
	echo 'package atlas' >"$r/internal/atlas/zz_untracked_probe.go"
	[ "$(state "$r")" = true ] && ok "untracked source file reports true" || not_ok "untracked source file not reported dirty"
	[ "$(state "$r/internal")" = true ] && ok "subdirectory reports the repository state" || not_ok "subdirectory state wrong"
	rm "$r/internal/atlas/zz_untracked_probe.go"
	echo '// changed' >>"$r/internal/atlas/app.go"
	[ "$(state "$r")" = true ] && ok "tracked modification reports true" || not_ok "tracked modification not reported dirty"

	# --require-clean against a minimal scratch checkout; it must stop before any build.
	c=$T/clean-co
	mkdir -p "$c/scripts/release" "$c/cmd/luuna-atlas" "$c/vendor" "$c/internal/atlas"
	cp "$pkg" "$wts" "$c/scripts/release/"
	printf 'module example.invalid/x\n\ngo 1.27.0\n' >"$c/go.mod"
	echo 'const Version = "0.0.0"' >"$c/internal/atlas/app.go"
	echo keep >"$c/vendor/modules.txt"
	echo 'package main' >"$c/cmd/luuna-atlas/main.go"
	git init -q "$c"
	echo 'out/' >"$c/.gitignore"
	g() { git -C "$c" -c user.name=release-test -c user.email=release-test@example.invalid -c core.hooksPath=/dev/null -c commit.gpgsign=false "$@"; }
	g add -A && g commit -q -m init
	echo 'package atlas' >"$c/internal/atlas/zz_untracked_probe.go"
	expect 1 "--require-clean rejects an untracked source file" sh "$c/scripts/release/package.sh" --version 1.0.0 --out "$c/out" --require-clean
	grep -q 'worktree_dirty=true' "$T/err" && ok "--require-clean reports the dirty state" || not_ok "--require-clean message missing"
	[ ! -e "$c/out" ] && ok "--require-clean failure created no output" || not_ok "--require-clean failure wrote output"
	rm -rf "$c/.git"
	expect 1 "--require-clean rejects an unknown (null) state" env GIT_CEILING_DIRECTORIES="$T" sh "$c/scripts/release/package.sh" --version 1.0.0 --out "$c/out" --require-clean
	grep -q 'worktree_dirty=null' "$T/err" && ok "--require-clean reports the null state" || not_ok "--require-clean null message missing"
else
	echo 'skip scratch git repositories (git not on PATH)'
fi

echo '# real host build'
if $skip_build; then
	echo 'skip real build (--skip-build)'
elif ! command -v go >/dev/null 2>&1; then
	echo 'skip real build (go not on PATH)'
else
	archive=luuna-atlas_0.0.0-test_${host_os}_${host_arch}.tar.gz
	expect 0 "package.sh builds the host archive (TZ=Pacific/Kiritimati)" env TZ=Pacific/Kiritimati sh "$pkg" --version 0.0.0-test --out "$T/build"
	built=$T/build/$archive
	expect 0 "check-archive.sh accepts the built archive" sh "$chk" --require-exec "$built"
	grep -q '"archive_version": "0.0.0-test"' "$T/build/provenance.json" &&
		grep -q '"internal_version": "' "$T/build/provenance.json" &&
		ok "provenance records archive and internal versions separately" || not_ok "provenance versions missing"
	want_dirty=$(sh "$here/worktree-state.sh" "$here/../..")
	grep -q "\"worktree_dirty\": $want_dirty," "$T/build/provenance.json" &&
		ok "provenance worktree_dirty matches worktree-state.sh ($want_dirty, untracked files included)" ||
		not_ok "provenance worktree_dirty does not match $want_dirty"

	expect 0 "package.sh builds again under TZ=UTC0" env TZ=UTC0 sh "$pkg" --version 0.0.0-test --out "$T/build-utc"
	cmp -s "$built" "$T/build-utc/$archive" && ok "archives are byte-identical across host time zones" ||
		not_ok "archive differs between TZ=Pacific/Kiritimati and TZ=UTC0"
	mtime=$(gzip -dc "$built" | dd bs=1 skip=136 count=11 2>/dev/null)
	[ "$mtime" = 07033241600 ] && ok "entry mtime is 2000-01-01T00:00:00Z (946684800)" ||
		not_ok "entry mtime is octal '$mtime', expected 07033241600"

	# tar shims: a create failure must surface, and dropped owner flags must be caught.
	real_tar=$(command -v tar)
	mkdir -p "$T/failtar" "$T/striptar"
	cat >"$T/failtar/tar" <<EOF
#!/bin/sh
for a in "\$@"; do
	case "\$a" in -cf | -c*) echo 'shim: simulated tar create failure' >&2; exit 1 ;; esac
done
exec "$real_tar" "\$@"
EOF
	cat >"$T/striptar/tar" <<EOF
#!/bin/sh
create=false
for a in "\$@"; do case "\$a" in -cf | -c*) create=true ;; esac; done
\$create || exec "$real_tar" "\$@"
n=\$#
i=0
skip=false
while [ "\$i" -lt "\$n" ]; do
	a=\$1
	shift
	i=\$((i + 1))
	if \$skip; then skip=false; continue; fi
	case "\$a" in
	--uid | --gid | --uname | --gname) skip=true; continue ;;
	--owner=* | --group=* | --numeric-owner) continue ;;
	esac
	set -- "\$@" "\$a"
done
exec "$real_tar" "\$@"
EOF
	chmod 0755 "$T/failtar/tar" "$T/striptar/tar"
	expect 1 "package.sh fails loudly when tar cannot create the archive" env PATH="$T/failtar:$PATH" sh "$pkg" --version 0.0.0-test --out "$T/build-failtar"
	grep -q 'simulated tar create failure' "$T/err" && grep -q 'tar failed to create the archive' "$T/err" &&
		ok "tar failure is visible, not masked" || not_ok "tar failure was masked"
	[ ! -e "$T/build-failtar/$archive" ] && ok "no archive after tar failure (no fallback)" || not_ok "fallback archive was produced"
	if [ "$(id -u)" != 0 ]; then
		expect 1 "package.sh rejects an archive not owned by root" env PATH="$T/striptar:$PATH" sh "$pkg" --version 0.0.0-test --out "$T/build-striptar"
		grep -q 'not 0755 regular files owned by root' "$T/err" && ok "owner check names the problem" || not_ok "owner check message missing"
		[ ! -e "$T/build-striptar/$archive" ] && ok "non-root-owned archive was not published to --out" || not_ok "non-root-owned archive reached --out"
	else
		echo 'skip owner-flag shim (running as root)'
	fi
fi

printf '# %d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
