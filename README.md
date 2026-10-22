# Luuna Atlas

Structured expertise and project memory. This is Luuna's maintained MIT-licensed component. The Luuna package identity is **@luuna/atlas-cli**. It is maintained in the independent public repository [brodynelly/luuna-atlas](https://github.com/brodynelly/luuna-atlas). The npm package remains private and unpublished.

Run from this directory with Bun 1.4.3:

```bash
bun install --frozen-lockfile --ignore-scripts
bun src/cli.ts --help
bun run verify
```

The standalone executable is `luuna-atlas`; `lat` is the Luuna suite shortcut and `la` remains a compatibility alias. `luuna atlas <args>` is the equivalent entry through Luuna's installed launcher. In a checkout, install dependencies with the command above and run `bun src/cli.ts <args>`. Registry publication is disabled; use the Luuna CLI installer from the main checkout to make the commands available on PATH.

Project data lives in `.luuna/atlas/` (configuration in `.luuna/atlas/config.yaml`). A project that still has the pre-Luuna data directory is moved there automatically the first time a command needs the store, with one notice on stderr; record formats are unchanged. Luuna consumes a pinned Git submodule and integrates through the CLI/JSON boundary (Atlas also exports a TypeScript library); runtime policy and approvals remain Luuna's responsibility.

Self-upgrade is disabled while the package is private. Update through a reviewed commit in this repository; never replace this fork with a similarly named registry package. Dependency installs do not change Git hooks.

Read [UPSTREAM.md](UPSTREAM.md) for attribution and provenance, [AGENTS.md](AGENTS.md) for development rules and [MIGRATION.md](MIGRATION.md) for layout changes. Upstream release/runbook documents are historical references and do not authorize publication.

## Origin / attribution

Luuna Atlas is derived from Mulch 0.11.0 by Jaymin West (MIT), https://github.com/jayminwest/mulch. The original license notice is preserved in [LICENSE](LICENSE); see [UPSTREAM.md](UPSTREAM.md) for provenance.

## Maintenance and releases

The repository owner and release maintainer is `brodynelly`. CI runs the frozen Bun 1.4.3 dependency installation, type checks, instruction checks and full suite on Linux. Registry publishing is disabled and requires no npm token. Luuna pins a reviewed commit, never a floating branch; update its submodule only after CI and the consumer smoke pass. See [MIGRATION.md](MIGRATION.md) for the verified source baseline.
