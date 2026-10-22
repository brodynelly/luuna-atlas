# Atlas Operations Runbook

This runbook covers luuna-atlas's operational procedures only:

1. Cutting a release of `@luuna/atlas-cli` to npm.
2. Triaging a failed publish.
3. Rolling back a bad release.
4. Debugging a broken `luuna-atlas prime` invocation.

For day-to-day development conventions, see `AGENTS.md` and `CLAUDE.md`.
For configuration reference, see `CONFIG.md`.

## Pre-flight (do once per machine)

- `bun --version` ≥ the version in `package.json` `engines.bun` (≥ 1.0).
- `gh auth status` → authenticated, with `repo` + `workflow` scopes.
- `git remote -v` shows the canonical origin
  (`github.com/jayminwest/luuna-atlas`).
- For npm publish: `npm whoami` → `jayminwest`; 2FA enabled.
- Local working tree on `main`, fully up to date, `git status` clean.

Atlas is a no-build Bun CLI — there is **no** compile/`dist` step. The
publish flow is fully automated via `.github/workflows/publish.yml`; you
should not need to invoke `npm publish` manually for a normal release.

## 1. Release procedure

Cut releases from `main` only. Never tag a feature branch.

### 1.1 Decide the version

Follow [SemVer](https://semver.org). Pick:

- **MAJOR** for any backward-incompatible change to the `luuna-atlas` public CLI
  surface or the on-disk JSONL / config schema.
- **MINOR** for new features or non-breaking additions (new subcommand,
  new optional flag, new record type).
- **PATCH** for bug fixes, doc-only changes, internal refactors, or
  dependency bumps that don't change luuna-atlas's surface.

While luuna-atlas is pre-1.0 (`package.json` `"version"` starts with `0.`),
breaking changes go in MINOR; additive changes go in PATCH.

### 1.2 Bump the version in every source of truth

Atlas's version lives in **two** places, kept in sync manually. The
publish workflow asserts they agree before pushing to npm:

- `package.json` — the `"version"` field.
- `src/cli.ts` — `export const VERSION = "X.Y.Z"`.

```bash
bun run version:bump              # scripts/version-bump.ts updates both
git diff package.json src/cli.ts  # confirm only the version moved
```

If you edit by hand instead, change both files to the identical
`X.Y.Z`. The `Verify version sync` step in `.github/workflows/publish.yml`
greps `src/cli.ts` and fails the release on any mismatch.

### 1.3 Update the changelog

`CHANGELOG.md` must have a new dated entry at the top under a
`## [X.Y.Z] - YYYY-MM-DD` heading. The publish workflow extracts this
section verbatim with an awk script and uses it as the GitHub release
body, so the heading format matters.

Group changes under standard Keep-a-Changelog headings (Added /
Changed / Fixed / Deprecated / Removed / Security). Link each entry to
its tracker id (`atlas-XXXX`, `mx-XXXX`), `#NNN` GitHub issue, or a URL.

### 1.4 Final gate check

```bash
bun install
bun run lint
bun run typecheck
bun test
bun run check:agents
```

All must exit 0. If any fails, **stop** — fix locally and re-run before
continuing.

### 1.5 Commit and push to `main`

```bash
git add package.json src/cli.ts CHANGELOG.md
git commit -m "release: luuna-atlas X.Y.Z"
git push origin main
```

Pushing triggers `.github/workflows/publish.yml`, which:

1. Re-runs `bun run lint`, `bun run typecheck`, and `bun test` in CI.
2. Compares `package.json` `"version"` against the npm registry's
   current `@luuna/atlas-cli` version. If they match, the workflow is a
   no-op (`publish=false`); otherwise it proceeds.
3. Asserts `package.json` and `src/cli.ts` agree on `X.Y.Z`.
4. Publishes `@luuna/atlas-cli@X.Y.Z` to npm with
   `--access public --provenance`.
5. Tags `vX.Y.Z` and pushes the tag to origin.
6. Extracts the matching `CHANGELOG.md` section and uses it as the
   GitHub release body; falls back to `--generate-notes` if empty.

Watch the run live:

```bash
gh run watch
```

### 1.6 Post-release sanity

```bash
git pull --tags
gh release view vX.Y.Z                       # confirm release page renders
npm view @luuna/atlas-cli version           # confirm published version
```

Smoke-install in a clean dir:

```bash
mkdir /tmp/atlas-smoke && cd /tmp/atlas-smoke
bunx @luuna/atlas-cli --version             # should print X.Y.Z
bunx @luuna/atlas-cli --help                # should list all subcommands
```

## 2. Triage of a failed publish

When `.github/workflows/publish.yml` exits non-zero:

### 2.1 Read the log

```bash
gh run list --workflow=publish.yml --limit 5
gh run view <run-id> --log-failed
```

Common failures and fixes:

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `Version mismatch! package.json=... src/cli.ts=...` | The "Verify version sync" step failed. | Sync the two files; push a fix commit. |
| `Version X.Y.Z already published, skipping.` | npm already has this version. | Not an error — the workflow short-circuits to a no-op. Bump if you meant to ship. |
| `npm publish ... 403` | Missing or **expired `NPM_TOKEN`** secret. | Repo → Settings → Secrets → update `NPM_TOKEN`; re-run the workflow. |
| `npm publish ... E409` / `cannot publish over existing version` | **Version conflict** — that version was already published from another commit. | Bump to the next patch; do **not** unpublish a live version. |
| `npm publish ... 429` | **Registry rate limit.** | Wait a few minutes, then re-run the workflow (no version change needed). |
| `gh release create ... already exists` | Tag `vX.Y.Z` exists but a prior run made an incomplete release. | Delete the orphan release in the GitHub UI, then re-run. |
| `tsc` / `biome` / `bun test` failure in publish.yml | Local greens diverged from CI (env / OS path / race). | Reproduce locally; do **not** force-push to `main`. |

### 2.2 Re-run the workflow

After the fix commit lands on `main`:

```bash
gh workflow run publish.yml --ref main
```

### 2.3 If the publish half-succeeded

If `npm publish` completed but `gh release create` failed (or vice
versa), **do not unpublish**. Recover the missing half manually:

- npm version exists but the GitHub release is missing:
  ```bash
  awk '/^## \[X.Y.Z\]/{found=1; next} found && /^## \[/{exit} found{print}' CHANGELOG.md > /tmp/notes.md
  gh release create vX.Y.Z --title "vX.Y.Z" --notes-file /tmp/notes.md
  ```
- npm has the version but the git tag is missing:
  ```bash
  git tag vX.Y.Z <release-sha>
  git push origin vX.Y.Z
  ```

Record the deviation in a `atlas-XXXX` tracker so future operators know
the half-step happened.

## 3. Rollback

A "rollback" never means unpublishing — npm versions and git tags are
immutable. Rollback means **publishing a corrective version**.

### 3.1 Decide the severity

- **Critical** (data loss, JSONL corruption on write, total CLI
  breakage): cut a patch reverting the change within 30 minutes.
- **High** (regression on a common path: `luuna-atlas record` or `luuna-atlas prime`
  breaks): cut a patch within the day.
- **Medium / Low**: fix forward on the next planned release.

### 3.2 Revert the offending commits

```bash
git checkout main
git pull
git log --oneline -10
git revert <bad-sha>           # creates a new commit, preserves history
```

The revert commit goes into the work for the next patch, `X.Y.(Z+1)`.

### 3.3 Cut a follow-up release

Follow §1.2–§1.5. In `CHANGELOG.md`, note the rollback explicitly under
`### Fixed`, naming the reverted commit and the symptom it caused.

### 3.4 Deprecate the bad version on npm

If `@luuna/atlas-cli@X.Y.Z` is dangerous to install:

```bash
npm deprecate @luuna/atlas-cli@X.Y.Z \
  "Critical bug; install X.Y.(Z+1) or later. See CHANGELOG.md."
```

`npm deprecate` does not remove the version (which would break
reproducible installs); it surfaces a warning at install time. Then
publish the patch with the previous behavior restored (§3.2–§3.3).

### 3.5 Communicate

- Edit the GitHub release notes for `vX.Y.Z` with a banner:
  `> ⚠️ This release contains a regression. Use vX.Y.(Z+1) or later.`
- File / update `atlas-XXXX` with root cause + remediation links.
- If a downstream consumer (downstream tools) pinned the bad
  version, open an issue against that repo recommending the upgrade.

## 4. Debugging a broken `luuna-atlas prime`

`luuna-atlas prime` is read-only — it never mutates `.luuna/atlas/`. When it errors,
prints nothing, or prints the wrong records, the problem is almost
always the on-disk store, not the command.

### 4.1 Confirm which store is being read

Atlas resolves its data dir via `getAtlasDir()` (worktree-aware: a
worktree resolves to the **main** repo's `.luuna/atlas/`). Inspect the
resolved store with:

```bash
luuna-atlas doctor
```

If you are inside a `git worktree` and expected local edits, remember
they are read from the main checkout's `.luuna/atlas/expertise/`, not the
worktree copy.

### 4.2 Run the integrity check

```bash
luuna-atlas doctor                         # reports record-integrity problems
luuna-atlas validate                       # re-validates every record against its schema
```

`luuna-atlas doctor` surfaces the failing checks; common causes of a broken
`luuna-atlas prime`:

- **JSONL corruption** — a hand-edited or partially-written line in
  `.luuna/atlas/expertise/<domain>.jsonl` that isn't valid JSON. Never
  hand-edit these files; use `luuna-atlas record` / `luuna-atlas edit` so the advisory
  lock and atomic write apply. Inspect the offending file and fix or
  remove the malformed line.
- **Unknown record type** — a record whose `type` isn't registered
  throws `Unknown record type "X" at <file>:<line>`. The type was likely
  added to `.luuna/atlas/config.yaml` `custom_types` on another branch
  that hasn't merged yet. Pass `luuna-atlas prime --allow-unknown-types` as a
  temporary escape hatch until config catches up.
- **Broken file anchors** — records anchored to files that no longer
  exist. Strip them with:
  ```bash
  luuna-atlas doctor --fix
  ```

### 4.3 Narrow the failure to one domain

`luuna-atlas prime <domain>` loads a single domain; `luuna-atlas prime --files <path>`
loads only records anchored to a path. Use them to isolate which domain
file is at fault, then `luuna-atlas status` to confirm record counts per domain
look sane.

## Appendix — Common commands

```bash
# Inspect recent releases
git tag --sort=-creatordate | head -5
gh release list --limit 5

# Inspect a failing workflow run
gh run list --workflow=publish.yml --limit 5
gh run view <run-id> --log-failed
gh run rerun <run-id> --failed

# Inspect what npm has published
npm view @luuna/atlas-cli versions --json
npm view @luuna/atlas-cli dist-tags
```
