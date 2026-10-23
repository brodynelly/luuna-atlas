# Patch: native release review blockers (C-atlas)

Source: final read-only review of `scripts/release/*`, `.github/workflows/release.yml`, `docs/RELEASING.md` against `specs/chore-native-release.md`.

Environment note: `bun run verify` was **not** run to completion. The host has Go 1.27.3 and the repository pins exactly 1.27.0. That check is missing, not passing. Do not change pins or download toolchains to resolve it.

## Blockers

### B1. SHA256SUMS is optional in check-archive.sh
`scripts/release/check-archive.sh:66-73` prints a note and continues when `SHA256SUMS` is absent. `test-release.sh:144-146` asserts that behaviour (exit 0). If the sums file is deleted, nothing checks the archive integrity, which defeats the checksum gate. The docs (step 1, "the file name and `SHA256SUMS`") assume it is mandatory.

Fix:
- Fail with `die "missing SHA256SUMS beside $name"` when the file is absent, missing, or is a symlink or non-regular file.
- Reject an `SHA256SUMS` with more than one line for `$name`. Use exact two-field matching; the existing awk is fine apart from the duplicate check.
- Change the `nosums` test to expect exit 1. Add a duplicate-entry fixture that expects 1. Keep the symlinked-archive case with a fixture that still has its sums file, so it continues to exercise the symlink rejection at exit 2.

### B2. Provenance hides untracked source files
`scripts/release/package.sh:196` uses `git status --porcelain --untracked-files=no`. An untracked `*.go` file under `cmd/` or `internal/` is compiled into the binary, yet `worktree_dirty` reports `false`. Provenance then claims a clean build of `commit` that the commit cannot reproduce.

Fix:
- Compute `dirty` **before** `go build`, using `git status --porcelain --untracked-files=normal` (ignored files such as `dist/` remain excluded).
- If git is unavailable or the directory is not a work tree, emit `"worktree_dirty": null` rather than `false`.
- Optional but recommended: add a `--require-clean` flag and pass it in `release.yml`.
- Add a test, guarded by a git work tree being present, that creates a temporary untracked `internal/atlas/zz_untracked_probe.go`, builds, asserts `"worktree_dirty": true` and removes the file in a trap. Alternatively, factor the dirty check into a function and test it against a scratch `git init` repository in `$T`. The scratch repository is preferred because it does not touch the real tree.

### B3. Archive timestamps depend on the host time zone
`package.sh:160` runs `touch -t 200001010000` in local time. This was verified on this host: under `TZ=UTC` the mtime is 946684800, and under `TZ=America/Los_Angeles` it is 946713600. As a result the tar headers, and therefore the archive SHA, differ between hosts and between a local dry run and CI. That contradicts the "identical inputs produce an identical archive" claim in the script comment and in `docs/RELEASING.md`.

Fix: `TZ=UTC0 touch -t 200001010000 ...` (or export `TZ=UTC0` near `umask`). Add a test: build twice with different `TZ` values (for example `TZ=UTC0` and `TZ=Pacific/Kiritimati`) into two output dirs and assert byte-identical archives. A cheaper option is to assert the mtime of `tar -tzvf` entries, or to package with `--skip`-style stub binaries if a build hook is added.

### B4. tar failure masking and a non-deterministic fallback
`package.sh:167-170`: the BSD branch silences stderr (`2>/dev/null`) and on any failure silently retries without `--uname/--gname/--no-xattrs/--no-acls/--no-fflags`. A real error (disk full, unreadable file) is hidden. A flag-support failure silently produces an archive whose owner names come from the runner's user and whose xattrs/ACL metadata may be present, so it is non-reproducible. The post-check only validates entry names.

Fix:
- Remove the silent fallback. Run one BSD tar invocation without `2>/dev/null` and fail loudly. macOS bsdtar 3.x supports all of those flags.
- If a fallback is retained, it must print a warning to stderr and fail unless `--allow-nondeterministic` is given.
- After building, assert via `tar -tzvf` that both entries are `-rwxr-xr-x` with owner `0`/`root`. This reuses check-archive's mode check: run `check-archive.sh` on `$stage` output before moving, or simply rely on the workflow step but also run the mode check in package.sh.
- Also in `package.sh:174`, `tar -tzf … | sort` masks a tar failure in the pipeline. Capture `tar -tzf` output to a variable first (as check-archive.sh does) and `die` on failure.

## Non-blocking findings (recommended)

- **N1.** Pin the workflow actions to full commit SHAs, with the version in a comment: `actions/checkout`, `actions/setup-go`, `actions/upload-artifact`. Tags are mutable, and this job produces release candidates.
- **N2.** Add a workflow step that asserts `go env GOVERSION` = `go1.27.0` and that `uname -sm` matches `matrix.os/arch` before packaging. The `--require-exec` check already catches an architecture mismatch, but only after the build.
- **N3.** Have `check-archive.sh` also verify the `LICENSE` and `UPSTREAM.md` lines of `SHA256SUMS` when those files are present, and require them, because redistribution depends on them.
- **N4.** `docs/RELEASING.md` should document that SHA256SUMS is mandatory (after B1), the `TZ` handling (after B3), and that `worktree_dirty` includes untracked files (after B2).

## Verified OK (no change)

- The workflow injection surface: `inputs.version` reaches shell only through `env` and quoted `"$ARCHIVE_VERSION"`. `package.sh` validates the strict grammar before the check-archive and upload steps run. Uses of `${{ inputs.version }}` in `with: name:` are not shell. `contents: read`, `persist-credentials: false`, `cache: false`, `workflow_dispatch` only, and the attestation step is commented out.
- The runner matrix is native for all four targets (`macos-14` arm64, `macos-15-intel` amd64, `ubuntu-latest` amd64, `ubuntu-24.04-arm` arm64), and `--require-exec` enforces the host match.
- The check-archive entry allowlist, mode, symlink, hardlink, traversal and absolute-path rejection, the private extraction directory and the sandboxed HOME are all present.
- The build flags match the spec. No `-X` is applied to the constant `Version`.

## Validation after the patch

```sh
GOTOOLCHAIN=local go test -mod=vendor ./...
sh scripts/release/test-release.sh
out=$(mktemp -d); sh scripts/release/package.sh --version 0.0.0-local --out "$out"
sh scripts/release/check-archive.sh --require-exec "$out"/luuna-atlas_0.0.0-local_*.tar.gz
rm "$out/SHA256SUMS"; ! sh scripts/release/check-archive.sh "$out"/luuna-atlas_0.0.0-local_*.tar.gz
```
Report `bun run verify` as blocked by the Go 1.27.3 vs 1.27.0 pin mismatch unless it is run on a matching toolchain.
