# Luuna Atlas

Structured expertise and project memory. This is Luuna's maintained MIT-licensed fork of [Mulch](https://github.com/jayminwest/mulch), imported from upstream version **0.11.0**. The Luuna package identity is **@luuna/atlas-cli**. It is maintained in the independent public repository [brodynelly/luuna-atlas](https://github.com/brodynelly/luuna-atlas). The npm package remains private and unpublished.

Run from this directory with Bun 1.4.3:

```bash
bun install --frozen-lockfile --ignore-scripts
bun src/cli.ts --help
bun run verify
```

The primary executable is `luuna-atlas` with `la` as its short alias. Legacy aliases `ml`, `mulch` remain available when the package is linked or installed, so existing integrations continue to work. Direct source execution uses the command above. Some inherited subcommand output and generated integration snippets retain legacy command names; they resolve through these compatibility aliases.

Existing data stays in .mulch/ and preserves its upstream record types, configuration, markers, and environment variables. Branding does not migrate or rewrite user data. Luuna consumes a pinned Git submodule and integrates through the CLI/JSON boundary (Atlas also exports a TypeScript library); runtime policy and approvals remain Luuna's responsibility.

Self-upgrade is disabled while the package is private. Update through a reviewed commit in this repository; never replace this fork with a similarly named registry package. Dependency installs do not change Git hooks.

Read [UPSTREAM.md](UPSTREAM.md) for attribution and provenance, [AGENTS.md](AGENTS.md) for development rules, and [the original README](docs/UPSTREAM-README.md) for inherited command examples. Upstream release/runbook documents are historical references and do not authorize publication.

## Maintenance and releases

The repository owner and release maintainer is `brodynelly`. CI runs the frozen Bun 1.4.3 dependency installation, type checks, instruction checks and full suite on Linux. Registry publishing is disabled and requires no npm token. Luuna pins a reviewed commit, never a floating branch; update its submodule only after CI and the consumer smoke pass. See [MIGRATION.md](MIGRATION.md) for the verified source baseline.
