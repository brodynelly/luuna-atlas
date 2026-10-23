# Luuna Atlas development

This component is a maintained fork of an upstream project, with provenance in `UPSTREAM.md` and the original MIT notice in `LICENSE`.

Use Go 1.27.0 and Bun 1.4.3. Go dependencies are vendored; verification disables toolchain downloads and module network access. From this directory:

```bash
bun install --frozen-lockfile --ignore-scripts
bun run build:native
bun run go:test
bun run typecheck
bun run check:agents
bun test --timeout 30000
bun run verify
```

The native executable starts in `cmd/luuna-atlas/`; business logic lives in `internal/atlas/`. The strict TypeScript in `src/` is the compatibility launcher and library transport to Go. It must not reimplement business logic. The frozen pre-rewrite implementation in `compat/reference/` is a development oracle, never a production dependency. Bun tests of the CLI and exported library use the native implementation; historical internal unit tests import the reference explicitly. Go tests use real temporary projects and compare observable results against the oracle. Keep advisory locks, atomic writes, JSON output contracts, terminal interactions, command/flag compatibility and secret redaction. Test changes at the appropriate public boundary.

Primary executable: luuna-atlas (lat; compatibility alias la). This component owns structured expertise and project memory. Missions owns tickets/plans; Beacon owns reusable prompts. Project data lives under .luuna/atlas in the user project (config in config.yaml); the one-way migration and legacy names live only in `internal/atlas/legacy_layout.go`. Preserve configuration keys and record schemas unless an explicit migration is designed and tested. Runtime policy and approval enforcement belong in Luuna, not in a rewrite of this component.

Do not load imported dogfooding stores, session histories, or upstream agent skills as Luuna work records. Do not create them as a development ritual. Installations must not alter the root Git hook configuration. This is an independent public repository maintained by brodynelly. Packages remain unpublished; registry publication and upstream self-upgrades require a separate reviewed release process.

`bun run verify` is the supported component validation surface: Go formatting check, native executable/alias build, go vet, Go tests including differential cases, TypeScript type check, instruction check and the Bun suite. `bun run go:test:race` adds race detection when the host has a C compiler supported by Go. The built native executable runs without Bun or Node; user-provided TypeScript/npm recipe extensions still need Bun. Dependency installation never builds the executable or installs hooks. Inherited check:all and quality-budget scripts remain available for upstream maintenance comparison; they reference standalone upstream CI and are not the supported CI contract. Luuna consumes exact Git commits through submodules; changes here do not authorize changes to Luuna runtime policy.

Do not commit or push unless explicitly requested. Report remaining work without automatically creating issues or synchronizing component state.
