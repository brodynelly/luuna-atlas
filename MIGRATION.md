# Repository extraction, October 21, 2026

Maintainer and release owner: brodynelly. Destination: https://github.com/brodynelly/luuna-atlas.

- Source: brodynelly/luuna at `fd42d59c5f608b9120d9cdc2e2354b66f945a5ed`, path `components/atlas`.
- Component-scoped history: `git subtree split --prefix=components/atlas fd42d59c5f608b9120d9cdc2e2354b66f945a5ed` produced `9e85c58e38009e02bf0f241b1db0f562970ec03e`.
- The extracted baseline tree exactly matches the source subtree before independent-repository metadata changes.
- Original upstream provenance and pre-branding import revision remain in UPSTREAM.md; the complete earlier monorepo history stays in Luuna.
- Source, data formats, aliases, dependencies and lockfile are unchanged by extraction. LICENSE SHA-256: `fe1956a36381a8ac9e13e229247eb1f05f951bb6a96153773025d03a0fd62519`.
- This commit adds standalone ownership, documentation and CI. Luuna consumes the exact reviewed Git commit as a submodule at the existing path.
- No registry package is published. GitHub access supplies source retrieval; npm release credentials are intentionally unnecessary.

Visibility: public, explicitly authorized by the owner on October 21, 2026. Luuna remains private. No private authoring records or credential files are included in the extracted history.

The owner requested the explicit luuna-* repository names on October 21, 2026. GitHub repository identity, history and CI are preserved by rename; the canonical destination is brodynelly/luuna-atlas.

# Layout move to .luuna/, October 22, 2026

- Project data: `.mulch/` becomes `.luuna/atlas/`; `.mulch/mulch.config.yaml` becomes `.luuna/atlas/config.yaml`. Expertise, archive, recipes, state and the JSONL record format are unchanged.
- Automatic: the first command that needs the store (including `init`) renames a legacy `.mulch/` into place and prints one notice on stderr. It refuses when `.luuna` or `.mulch` is a symlink, and fails with an explanatory error when both `.mulch/` and `.luuna/atlas/` exist. Matching `.gitattributes`/`.gitignore` lines are rewritten. Commit the move.
- Sibling data: Missions is read from `.luuna/missions/` (legacy `.seeds/` read-only fallback).
- Removed aliases `ml` and `mulch`; environment variables `MULCH_*` are `LUUNA_ATLAS_*`.
- Re-run `luuna-atlas setup claude` to replace old hook commands; re-run `luuna-atlas onboard` to refresh instruction snippets (old marker sections are replaced in place).

# Native Go implementation, October 22, 2026

The CLI and library operations were rewritten in Go against Atlas commit `27477c313a3049b1579dfdc32c39da600726721f`, compatibility version `0.11.0`. No additional data migration is introduced. Existing commands, aliases, configuration keys, record schemas and the layout migration above remain the compatibility boundary.

Development now requires Go 1.27.0 and Bun 1.4.3. Go dependencies are vendored. `bun run build:native` produces the standalone executable and aliases; `bun run verify` checks the native implementation and compatibility surfaces. Existing TypeScript entry points call Go. The former implementation is a frozen test oracle under `compat/reference/`, excluded from production packaging and execution.

See [GO-REWRITE-REPORT.md](GO-REWRITE-REPORT.md) for architecture, command coverage and verification. This rewrite does not update a Luuna consumer pin, publish a package or change runtime policy. Those steps retain their separate review requirements.
