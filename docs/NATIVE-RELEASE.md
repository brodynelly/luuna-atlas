# Native release artifacts: status summary

This page summarizes the native archive work for Atlas (task C-atlas) and where it stands now. The full procedure is in [RELEASING.md](RELEASING.md). The scope comes from [specs/chore-native-release.md](../specs/chore-native-release.md) and the review fixes from [specs/patch-native-release-review.md](../specs/patch-native-release-review.md).

## Scope: manual artifacts only

- `.github/workflows/release.yml` runs only on `workflow_dispatch` with `permissions: contents: read`. It builds on four native runners (`macos-14`, `macos-15-intel`, `ubuntu-latest`, `ubuntu-24.04-arm`) and uploads the results with `actions/upload-artifact`, kept for 14 days.
- There is no tag trigger, GitHub Release, registry publish, signing or notarization. The attestation step is present only as a commented-out owner gate. Publishing is an owner decision (see "Owner gates" in RELEASING.md).
- Each archive, `luuna-atlas_<version>_<os>_<arch>.tar.gz`, contains exactly `bin/luuna-atlas` and `bin/lat`. Both are regular files with mode 0755, owned by uid/gid 0. `LICENSE`, `UPSTREAM.md`, `SHA256SUMS` and `provenance.json` are placed beside the archive, not inside it.

## Checksums

`package.sh` writes `SHA256SUMS` with entries for the archive, `LICENSE` and `UPSTREAM.md`. In `check-archive.sh`, the checksum check is **mandatory**. Each of these fails before anything is extracted:

- `SHA256SUMS` is missing, is a symlink or is not a regular file
- the archive has no entry, or more than one entry
- the checksum does not match

Open item (N3, not implemented): `check-archive.sh` checks only the archive line, not the `LICENSE` or `UPSTREAM.md` lines.

## Provenance

`provenance.json` records the archive version and the internal `const Version` separately, because that constant is not set by `-ldflags -X`. It also records the commit, ref, runner, target, the exact `go env GOVERSION`, build flags and checksums.

`worktree_dirty` is measured **before** the build by `scripts/release/worktree-state.sh`:

| Value | Meaning |
| --- | --- |
| `true` | tracked changes or untracked files (ignored files such as `dist/` do not count) |
| `false` | clean work tree |
| `null` | git is unavailable, or the directory is not a work tree |

This is self-reported metadata (`"attestation": null`), not a verifiable attestation.

## `--require-clean`

`package.sh --require-clean` exits with an error unless `worktree_dirty` is `false`, which means `null` also fails. The workflow passes this flag. Local changes must be committed or removed before `--require-clean` succeeds.

## Time zone and reproducibility

`package.sh` exports `TZ=UTC0` before running `touch -t 200001010000`, so the entry timestamps are always 2000-01-01T00:00:00Z whatever the host time zone. Other measures that keep the build reproducible:

- tar runs in a single invocation with visible errors and no silent fallback, and its owner and mode are checked after building
- `gzip -n`
- `-trimpath -ldflags '-s -w -buildid='`

`test-release.sh` builds under two different `TZ` values and requires byte-identical archives.

## Verification status (honest)

| Check | Result |
| --- | --- |
| Final read-only review after the patch | passed |
| Native release regression (`test-release.sh` and the related Go tests) | 72 passed |
| Bun test suite | 1608 passed |
| `bun run verify` (full component contract) | passed with Go 1.27.0 and Bun 1.4.3 |

**Shipping validation:** a checksum-verified Go **1.27.0** SDK was installed repo-locally under `.tac/tools/go1.27.0/go`; the system Go installation and version pins were not changed. The default macOS `TMPDIR` exposed a symlink-path mismatch in the parity test harness. [The test-only patch](../specs/patch-native-release-parity-paths.md) canonicalizes fixture roots without changing product path semantics, reference outputs or normalization rules. Full `bun run verify` then passed independently with the original `TMPDIR` unchanged.

Also not proven:

- A local run covers only the host platform. The other three targets are proven only by an owner-dispatched workflow run, and none has been dispatched as part of this work.
- Review items N1 (pinning workflow actions to commit SHAs) and N2 (a pre-build step asserting the Go version and `uname`) are still open. The workflow uses the tags `@v4`/`@v5`.
- Gatekeeper behaviour for unsigned macOS binaries, and musl/Alpine compatibility, are not covered.
