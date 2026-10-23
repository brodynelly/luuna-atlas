# Contributing to Luuna Atlas

Atlas is an independent MIT-licensed Luuna component maintained by brodynelly. Read [AGENTS.md](AGENTS.md) before changing it. Provenance and the original notice are in [UPSTREAM.md](UPSTREAM.md) and [LICENSE](LICENSE).

## Development

Use Go 1.27.0 and Bun 1.4.3. Go dependencies are vendored. From the checkout:

```sh
bun install --frozen-lockfile --ignore-scripts
bun run build:native
bun run verify
```

The build produces `luuna-atlas`, `lat` and `la` under `.atlas-bin/` (`.exe` on Windows). The executable runs without Bun or Node. Existing source consumers can use `bun src/cli.ts <args>`; it builds and launches Go when necessary. `LUUNA_ATLAS_GO` selects a Go executable outside PATH.

Useful focused checks:

```sh
bun run go:test
bun run go:vet
bun run go:fmt:check
bun run typecheck
bun run check:agents
bun test test/native-api.test.ts --timeout 30000
```

`bun run verify` is the supported validation contract: native formatting/build/vet/tests, compatibility TypeScript type checking, instruction validation and Bun tests. Race detection is available through `bun run go:test:race` on a host with a supported C compiler. The devcontainer pins the same Go/Bun versions; container startup was not part of the local rewrite verification.

## Implementation conventions

- Native entry point: `cmd/luuna-atlas/`; business logic: `internal/atlas/`; public Go API: `atlas.go`.
- Format Go changes with `gofmt`. Preserve advisory locks, atomic writes, secret redaction and schemas.
- `src/` contains strict TypeScript types, the CLI launcher and public API transport. Keep business operations in Go. Relative TypeScript imports use `.ts`.
- Public JSON, flags, aliases, help, terminal styling, streams and exit status are compatibility boundaries. Review existing behavior before changing it.
- Keep legacy names and the one-way store migration centralized in `internal/atlas/legacy_layout.go`.
- Configuration/data live in the user's project under `.luuna/atlas/`. Tests must use disposable projects rather than this checkout or imported authoring stores.

For current source and command coverage, read [GO-REWRITE-REPORT.md](GO-REWRITE-REPORT.md). The broader [system map](CLI-SYSTEM-MAP.md) documents the compatibility baseline and Luuna integrations.

## Testing

Go tests use real temporary files and processes. Differential tests execute the frozen TypeScript reference in a separate temporary project and compare outputs and persisted state with Go. The oracle in `compat/reference/` is immutable; do not edit it to satisfy new behavior. Its integrity manifest is checked by native tests.

Historical internal Bun unit tests explicitly import that reference. Public CLI subprocess tests and public API tests exercise Go through the compatibility entry points. A passing reference test alone is not native implementation evidence. Add behavior tests at the affected public boundary and include expected error/data effects.

Keep generated binaries, caches, test stores and credentials out of Git. Dependency installs must not change root Git hooks. Ordinary installation does not build or install hooks; explicit development builds and explicit setup commands do their documented work.

## Review and release

Keep each change focused and describe the resulting behavior and meaningful validation. Use the repository's supported verification before requesting review. Do not commit, push, create issues, update sibling component pins or synchronize component state unless requested.

The package remains private and unpublished. Registry publication and upstream/self-upgrade require a separate reviewed release process. Luuna consumes an exact reviewed Git commit through its submodule; runtime policy and approval enforcement belong in Luuna.

Use [GitHub Issues](https://github.com/brodynelly/luuna-atlas/issues) when you choose to report a bug. Security guidance is in [SECURITY.md](SECURITY.md). Contributions retain the [MIT License](LICENSE).
