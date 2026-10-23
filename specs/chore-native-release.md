# Chore: native release artifacts for Luuna Atlas (C-atlas)

Scoped from the suite plan "Public native CLI distribution for the Luuna suite" (hub spec `specs/feature-native-cli-distribution.md` in `luuna-ai-os`). Only task **C-atlas** applies here. Hub tasks (H0–H7) and the other component tasks belong to other repositories and agents.

## Goal

Build, verify and upload (as workflow artifacts only) host-native archives of `luuna-atlas` and its public alias `lat`. Publish nothing.

## Facts verified in this repository

- Entrypoint: `cmd/luuna-atlas/main.go` calls `atlas.NewApp().Run(os.Args[1:])`. Nothing branches on `argv[0]` (`format_color.go` also reads only `os.Args[1:]`). An alias copy therefore behaves the same and preserves its arguments.
- Version symbol: `internal/atlas/app.go` has `const Version = "0.11.0"`. A constant cannot be set with `-ldflags -X`, so no `-X` flag is used and no symbol is invented. The archive version is recorded separately from the internal version.
- Go modules are vendored (`vendor/`). `go.mod` declares `go 1.27.0`.
- `.tac/runs/` is already in `.gitignore`.
- Bun bins (`luuna-atlas`, `lat`, `la`) and the existing `ci.yml` stay unchanged.

## Deliverables

1. `scripts/release/package.sh`: POSIX script. It validates `--version`, `--out`, `--os` and `--arch`, then builds with `CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off go build -mod=vendor -trimpath -ldflags "-s -w -buildid="`. It copies `bin/lat` and writes a deterministic tar.gz containing only `bin/lat` and `bin/luuna-atlas` (mode 0755). It also writes `SHA256SUMS`, `provenance.json` and the attribution files `LICENSE` and `UPSTREAM.md`, which accompany the archive rather than sitting inside it.
2. `scripts/release/check-archive.sh`: checks the name, checksum, exact entry allowlist and regular 0755 modes before extraction. After extracting into a private temporary directory it checks file types again, confirms the alias is identical, and on a matching host runs `--version` and `--help` in a sandbox `HOME`. `--require-exec` is available.
3. `scripts/release/test-release.sh`: offline argument and hostile-fixture tests, plus an optional real host build.
4. `.github/workflows/release.yml`:
   - `workflow_dispatch` only, `contents: read`
   - native runner matrix: `macos-14` arm64, `macos-15-intel` amd64, `ubuntu-latest` amd64, `ubuntu-24.04-arm` arm64
   - steps: tests → package → check → `upload-artifact`
   - attestation present only as a commented owner gate
5. `docs/RELEASING.md`: local dry run, owner gates and limits.

## Deviations from the suite plan

- Runner `macos-13` (retired) is replaced by `macos-15-intel` for darwin/amd64.
- No `-X …version` ldflag, because the version is a constant (see above).
- `LICENSE` and `UPSTREAM.md` are shipped beside the archive, not inside it, to keep the strict executable allowlist.

## Validation

```sh
GOTOOLCHAIN=local go test -mod=vendor ./...
sh scripts/release/test-release.sh
out=$(mktemp -d); sh scripts/release/package.sh --version 0.0.0-local --out "$out"
sh scripts/release/check-archive.sh --require-exec "$out"/luuna-atlas_0.0.0-local_*.tar.gz
grep -nE 'gh release|action-gh-release|npm publish|cargo publish|attest-build-provenance' .github/workflows/release.yml  # commented lines only
```

`bun run verify` remains the component contract. It needs Bun 1.4.3, which this task does not install.
