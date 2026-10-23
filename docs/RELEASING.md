# Releasing native Atlas archives

Nothing in this repository publishes Atlas. The scripts and workflow below produce archives you can verify, and stop there. Tags, GitHub Releases, signing, notarization, attestations and npm packages are owner decisions that are deliberately not automated.

## What an archive contains

`luuna-atlas_<version>_<os>_<arch>.tar.gz` contains exactly two regular files with mode 0755:

- `bin/luuna-atlas`, the primary executable
- `bin/lat`, the public alias

There are no directories, links or other entries, so the suite installer's strict executable allowlist accepts it. The `la` compatibility alias stays available from source builds (`bun run build:native`) and is not part of the public archive.

`lat` is a byte-identical copy of `luuna-atlas`. `cmd/luuna-atlas/main.go` passes `os.Args[1:]` to the app and nothing branches on `argv[0]`, so both names behave the same and receive every argument unchanged. `check-archive.sh` verifies this on the host.

Licensing and attribution travel beside the archive rather than inside it, because the archive allowlist admits only executables. The output directory and the uploaded artifact contain `LICENSE` (MIT, upstream notice) and `UPSTREAM.md`, and both are listed in `SHA256SUMS`. Anyone redistributing an archive must ship those two files with it.

Platforms: `darwin/arm64`, `darwin/amd64`, `linux/arm64` and `linux/amd64`. Linux binaries are static (`CGO_ENABLED=0`). Windows archives are not produced.

## Versions

- **Archive version:** the `--version` you pass, for example `0.12.0`. It names the archive.
- **Internal version:** what `luuna-atlas --version` prints. It comes from `const Version` in `internal/atlas/app.go`.

The internal version is a Go constant, so `-ldflags -X` cannot override it, and the build does not try. `provenance.json` records both values. Keeping them equal is an owner release step: bump the constant (`bun run version:bump`) in a reviewed change before dispatching.

## Local dry run (host platform only)

```sh
GOTOOLCHAIN=local go test -p=1 -mod=vendor ./...
sh scripts/release/test-release.sh
out=$(mktemp -d)
sh scripts/release/package.sh --version 0.0.0-local --out "$out"
sh scripts/release/check-archive.sh --require-exec "$out"/luuna-atlas_0.0.0-local_*.tar.gz
```

`package.sh` uses only the local Go toolchain: `GOTOOLCHAIN=local`, `GOPROXY=off`, vendored modules and `CGO_ENABLED=0`. It never downloads a toolchain. It builds with `-trimpath -ldflags '-s -w -buildid='` and fixed file timestamps (2000-01-01T00:00:00Z; the script exports `TZ=UTC0`, so the host time zone does not change the tar headers), so identical inputs produce an identical archive. It creates the tar in one invocation with visible errors and no fallback, and it fails unless both entries are 0755 regular files owned by uid/gid 0 (root). It refuses to overwrite an existing archive and rejects a symlinked or non-directory `--out`.

`provenance.json` records `worktree_dirty`, measured before the build by `scripts/release/worktree-state.sh`: `true` when there are tracked changes **or untracked files** (an untracked `*.go` file would be compiled in), `false` for a clean work tree, and `null` when git is unavailable or the checkout is not a work tree. Ignored files such as `dist/` do not count. `--require-clean` makes any state other than `false` a failure; the workflow passes it.

`check-archive.sh` checks the archive before extracting it:

1. the file name and `SHA256SUMS`, which is mandatory: a missing, symlinked or non-regular `SHA256SUMS`, a missing entry, a duplicate entry or a mismatch all fail
2. that the entry list is exactly the two executables, with no absolute paths, `..`, links, devices or extra entries
3. that both entries are mode 0755

Only then does it extract into a private temporary directory. On a matching host it also runs `--version` and `--help` through both names, inside an empty sandbox `HOME`. `--require-exec` turns a host mismatch into a failure.

`test-release.sh` covers argument validation, hostile fixture archives (traversal, absolute path, symlink, hardlink, extra or missing entry, wrong mode, bad, missing or duplicate checksum, divergent alias) and the work-tree state against scratch Git repositories, without network access. The real-build section builds under two time zones and requires byte-identical archives, and uses `tar` shims to prove that a tar failure is reported and that an archive not owned by root is rejected. `--skip-build` omits the real builds.

Toolchain pin limitation: `go.mod` declares `go 1.27.0` and CI's `setup-go` installs 1.27.0. Locally, any installed Go 1.27.x satisfies `GOTOOLCHAIN=local`, so a local archive may come from a newer patch release. `provenance.json` records the exact `go` version used.

## Workflow (`.github/workflows/release.yml`)

- **Trigger and permissions:** manual `workflow_dispatch` only, with a `version` input and `permissions: contents: read`. There is no tag trigger.
- **Runners:** each matrix entry runs on a native runner: `macos-14` (darwin/arm64), `macos-15-intel` (darwin/amd64), `ubuntu-latest` (linux/amd64) and `ubuntu-24.04-arm` (linux/arm64). The retired `macos-13` label is not used. Because every runner is native, `check-archive.sh --require-exec` executes every archive on its own platform.
- **Steps:** Go tests, then the offline release-script tests, then `package.sh`, then `check-archive.sh`, then `actions/upload-artifact`. Artifacts are kept for 14 days.
- **Bun:** not required. The existing Bun-based `bun run verify` stays in `ci.yml`.
- **Not included:** release creation, registry publish steps and attestations. The attestation step is present only as a commented-out owner gate.

## Owner gates (not automated)

1. Bump `const Version` if needed, through a reviewed change.
2. Run the workflow with `workflow_dispatch`, download the four artifacts, and verify `SHA256SUMS`, `check-archive.sh` and `provenance.json` on each platform you claim.
3. Decide on signing, notarization (Apple Developer ID) and GitHub build attestations. Enabling attestations needs `id-token: write` and `attestations: write`.
4. Create the tag and GitHub Release and upload the archives, `SHA256SUMS`, `LICENSE` and `UPSTREAM.md`.
5. Record the released URLs and checksums in the suite hub's release manifest through its own reviewed process.

## What a passing run does not prove

- A local run proves only the host platform. The other platforms are proven only by an owner-dispatched workflow run.
- `provenance.json` is self-reported metadata, not a verifiable attestation.
- Unsigned macOS binaries downloaded through a browser may be quarantined by Gatekeeper. That case is not covered.
- musl/Alpine compatibility is not claimed, although Go static binaries usually run there.
