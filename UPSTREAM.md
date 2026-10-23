# Upstream provenance

- Original project: Mulch
- Original package: @os-eco/mulch-cli
- Upstream repository: https://github.com/jayminwest/mulch
- Imported upstream version: 0.11.0
- Import source: the tracked mulch/ tree in brodynelly/luuna at commit 532048d1839d003d724e55e8a47dbfdc44145004 (see repository history for the full tree).
- Luuna identity: @luuna/atlas-cli, in components/atlas/.
- Upstream copyright holder and author: Jaymin West, as retained in LICENSE and package authorship.

The original MIT LICENSE is preserved verbatim. Luuna modifications are also licensed under MIT by Luuna contributors. Original version numbers remain the compatibility baseline; a distinct Luuna release version must be chosen before publication.

Luuna changes include branding, private package metadata, safe installation without Git-hook mutation, current agent guidance, and removal of imported authoring/session records. Storage formats, historical command aliases, exported type names, environment variables, schema fields, and integration markers retain upstream compatibility.

The current implementation was rewritten in Go against the Luuna Atlas `0.11.0` compatibility baseline at commit `27477c313a3049b1579dfdc32c39da600726721f`. The former TypeScript implementation is retained only as a frozen development oracle in `compat/reference/`; it is excluded from production execution and packaging. Existing TypeScript entry points transport operations to Go. This rewrite does not change the original MIT attribution or authorize registry publication. See [GO-REWRITE-REPORT.md](GO-REWRITE-REPORT.md).

This maintained distribution now lives in https://github.com/brodynelly/luuna-atlas. Luuna consumes it as an exact-commit Git submodule. The package remains private and unpublished. The component-scoped history was extracted with git subtree split; the original pre-branding history remains in brodynelly/luuna at the import source recorded above. See MIGRATION.md.

Imported agent instructions and authoring metadata were preserved locally before removal from the working tree. They must not be loaded as Luuna project state. Historical documentation remains reference material; current component development starts with this repository's AGENTS.md. Luuna runtime policy remains in the Luuna repository.
