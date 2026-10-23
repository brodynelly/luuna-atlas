# Luuna Atlas

Read [AGENTS.md](AGENTS.md) for current development instructions and [UPSTREAM.md](UPSTREAM.md) for provenance. This repository owns the component; Luuna owns runtime policy. Archived upstream workflows are reference material.

The implementation is native Go in `cmd/luuna-atlas/` and `internal/atlas/`. `src/` preserves the Bun CLI and TypeScript API as transports to that executable. `compat/reference/` is a frozen test oracle; production code must not execute or import its business logic. Use Go 1.27.0 and Bun 1.4.3, and run `bun run verify` for the supported component checks. Preserve the CLI interactions, JSON contracts, stored schemas and existing aliases. No commit, push, release or root Git hook change is implied by development work.
