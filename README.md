# Luuna Atlas

Structured expertise and project memory. This is Luuna's maintained MIT-licensed component. The Luuna package identity is **@luuna/atlas-cli**. It is maintained in the independent public repository [brodynelly/luuna-atlas](https://github.com/brodynelly/luuna-atlas). The npm package remains private and unpublished.

The CLI and library behavior are implemented in Go. For development, install Go 1.27.0 and Bun 1.4.3, then run from this directory:

```bash
bun install --frozen-lockfile --ignore-scripts
bun run build:native
bun src/cli.ts --help
bun run verify
```

The standalone executable is `luuna-atlas`; `lat` is the Luuna suite shortcut and `la` remains a compatibility alias. The build writes all three executables to `.atlas-bin/` (with `.exe` on Windows). Run the native binary directly, or continue using `bun src/cli.ts <args>`; the compatibility entry point builds Go when needed and forwards arguments, input, output and exit status. `luuna atlas <args>` remains the entry through Luuna's installed launcher. Registry publication is disabled; use the Luuna CLI installer from the main checkout to make the commands available on PATH.

The native executable needs neither Bun nor Node. To build it directly with the pinned Go toolchain, use `go build -mod=vendor -trimpath -o luuna-atlas ./cmd/luuna-atlas`. Go dependencies are included in `vendor/`, so building does not fetch modules. Bun remains necessary for the source launcher, TypeScript library transport, development compatibility checks and user-provided TypeScript/npm provider recipes. Set `LUUNA_ATLAS_GO` to a Go executable when it is installed outside PATH, or `LUUNA_ATLAS_BINARY` to an existing native executable for the compatibility transport.

Project data lives in `.luuna/atlas/` (configuration in `.luuna/atlas/config.yaml`). A project that still has the pre-Luuna data directory is moved there automatically the first time a command needs the store, with one notice on stderr; record formats are unchanged. Luuna consumes a pinned Git submodule and integrates through the CLI/JSON boundary. The existing TypeScript library exports call the native Go implementation. Runtime policy and approvals remain Luuna's responsibility.

Self-upgrade is disabled while the package is private. Update through a reviewed commit in this repository; never replace this fork with a similarly named registry package. Dependency installs do not change Git hooks.

Read [UPSTREAM.md](UPSTREAM.md) for attribution and provenance, [AGENTS.md](AGENTS.md) for development rules and [MIGRATION.md](MIGRATION.md) for layout changes. Upstream release/runbook documents are historical references and do not authorize publication.

For the command, record, configuration and integration tour used as the rewrite baseline, read the [CLI and system map](CLI-SYSTEM-MAP.md). The companion [CLI reference](CLI-REFERENCE.md) captures help for every command and config subcommand. The [Go rewrite report](GO-REWRITE-REPORT.md) maps the current architecture, command coverage and verification. Baseline source links resolve to the frozen TypeScript oracle in `compat/reference/`; current Go source is in `cmd/luuna-atlas/` and `internal/atlas/`.

## Origin / attribution

Luuna Atlas is derived from Mulch 0.11.0 by Jaymin West (MIT), https://github.com/jayminwest/mulch. The original license notice is preserved in [LICENSE](LICENSE); see [UPSTREAM.md](UPSTREAM.md) for provenance.

## Maintenance and releases

The repository owner and release maintainer is `brodynelly`. CI is configured to run the supported `bun run verify` surface on Linux and Windows with Go 1.27.0 and Bun 1.4.3. It checks Go formatting, builds native executable aliases, runs go vet and Go tests, then type checks the compatibility surface, validates repository instructions and runs Bun tests. Differential tests compare Go outputs and persisted state with the frozen oracle; historical internal Bun unit tests validate that reference, while public CLI/API tests exercise Go. Race detection is available separately through `bun run go:test:race` on a host with a supported C compiler.

Registry publishing is disabled and requires no npm token. Luuna pins a reviewed commit, never a floating branch; update its submodule only after CI and the consumer smoke pass. See [MIGRATION.md](MIGRATION.md) for the verified source baseline. Dependency installation does not build executables or change Git hooks.
