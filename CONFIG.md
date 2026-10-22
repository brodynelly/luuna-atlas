# Atlas Configuration Guide

This guide covers `config.yaml` and the surrounding `.luuna/atlas/` surface for **v0.10.7**. It is the reference doc for orgs running Atlas across many repos and many ICs.

If you're new to Atlas, start with the [README](./README.md) — it covers what Atlas is and the day-one commands. This document assumes you already know `luuna-atlas record` / `luuna-atlas prime` and want to tune them.

Conventions in this doc:

- Every config key states its **default** next to its name.
- YAML examples are minimal and copy-pasteable.
- Cross-references look like `(see §Hooks)` and link to a section below.
- Two audiences: **Part 1** for platform engineers rolling Atlas out; **Part 2** for ICs working inside a rollout.

---

## Why customize Atlas

The defaults work fine for a solo repo or a five-person team. At 50+ ICs writing into the same expertise store, four kinds of entropy start to bite:

**Vocabulary drift.** Three different teams independently invent `incident`, `postmortem`, and `outage` records. None of them validate against each other, none of them surface to `luuna-atlas prime` consistently, and reviewers can't tell which kind of record they're reading. The fix is `custom_types` plus `disabled_types` to retire the losing names without breaking peer repos (see §Custom record types, §Retiring types).

**Ungoverned writes.** Without per-domain rules, anyone can drop a `pattern` into a `security` domain without naming an owner. Six months later, no one knows who to ask about the rule. The fix is `domains.<name>.required_fields` — say "every record in `security` must carry `owner`" and validation enforces it (see §Per-domain rules).

**Stale records crowding `luuna-atlas prime`.** Tactical records are 14-day by default. On a monolith, that's still hundreds of records on day 15. Two levers: drop `luuna-atlas prime` into `manifest` mode for large repos (see §Prime), and tighten `classification_defaults.shelf_life` so prune walks records out faster (see §Classification defaults).

**Integration sprawl.** Slack wants a webhook on every new failure. Compliance wants an audit log on every record. Each repo solving this independently produces six implementations and three of them break silently. The fix is `hooks` — a single shell script per repo, deployed from a template, doing the integration work in one place (see §Hooks).

**Provider fragmentation.** Your org standardized on an internal coding agent runtime three months ago. The built-in `claude` / `cursor` / `codex` recipes don't know about it. Filesystem recipes let you ship a custom recipe through repo scaffolding without forking Atlas (see §Provider recipes).

---

## Quick tour: scenarios for large orgs

Eight motivating scenarios. Each one is short on purpose — the section it points to has the working example.

**Slack-notify the team on every new failure record.** Drop a `post-record` hook that reads `payload.type === "failure"` from stdin and POSTs to a webhook. Non-zero exit warns but doesn't block, so a Slack outage can't take down `luuna-atlas record`. → §Hooks.

**Gate the `infra` domain to only `decision` and `runbook` types.** Set `domains.infra.allowed_types: [decision, runbook]`. ICs writing `failure` records into `infra` get a validation error pointing at the violating field, plus a copy-paste retry hint. → §Per-domain rules.

**Add a custom `incident` type that extends `failure`.** Declare it under `custom_types.incident` with `extends: failure` and add `pager_link` / `severity` to `required`. Inherits dedup, summary, and compact strategy from `failure`. → §Custom record types.

**Retire the `reference` type org-wide without breaking peer repos.** Add `reference` to `disabled_types`. New writes emit a deprecation warning; existing records still read; CLI `--type` choices still include it so peer repos that haven't synced the config aren't surprised mid-command. → §Retiring types.

**Ship a custom Cursor recipe to every repo in the org.** Drop a `.luuna/atlas/recipes/cursor.ts` shipping with your repo scaffolding. Filesystem recipes win over built-ins, so the org variant takes precedence without anyone running `npm install`. → §Provider recipes.

**Drop `luuna-atlas prime` output size on a monolith from 12k tokens to 800.** Set `prime.default_mode: manifest`. Agents see a domain index and per-domain counts up front, then call `luuna-atlas prime <domain>` or `luuna-atlas prime --files <path>` to scope-load. → §Prime output tuning.

**Force every record in `security` to carry an `owner` field.** Set `domains.security.required_fields: [owner]`. ICs running `luuna-atlas record security ...` without `--owner` get a targeted error before the write hits disk. → §Per-domain rules.

**Auto-archive observational records older than 14 days, not 30.** Set `classification_defaults.shelf_life.observational: 14`. The demotion ladder is unchanged — records still walk one tier per `luuna-atlas prune` pass — but the threshold to enter the walk shrinks. → §Classification defaults.

---

# Part 1 — Admin guide

## The `.luuna/atlas/` directory layout

```
.luuna/atlas/
├── config.yaml         # config (this doc); commit
├── expertise/
│   └── <domain>.jsonl        # append-only records; commit
├── archive/
│   └── <domain>.jsonl        # soft-archived records; commit
├── recipes/                  # optional; commit if shared org-wide
│   ├── <name>.ts
│   └── <name>.sh
├── hooks/                    # optional; commit
│   └── *.sh
└── README.md                 # auto-generated; commit
```

**Commit everything.** `.luuna/atlas/` is intentionally checked in — expertise is a team artifact, not a per-developer cache. The auto-generated `README.md` describes the layout for first-time visitors to the repo.

**`.gitattributes` setup.** At the repo root (not inside `.luuna/atlas/`):

```
.luuna/atlas/expertise/*.jsonl merge=union
.luuna/atlas/archive/*.jsonl   merge=union
```

`merge=union` makes concurrent record writes from two branches concatenate instead of conflicting on the merge. Combined with atomic writes and file locking inside Atlas itself, two agents writing to the same domain on different worktrees never collide.

**What not to read directly.** `.luuna/atlas/archive/<domain>.jsonl` is stale by definition. `luuna-atlas prime` and `luuna-atlas search` (without `--archived`) both skip it. Don't `cat` it during a coding session; use `luuna-atlas search --archived <query>` instead.

---

## `config.yaml` — top-level shape

A fully-populated config, with comments:

```yaml
version: "1"

# Per-domain write rules. Empty map (the default) means any registered type
# is accepted in any domain.
domains:
  security:
    allowed_types: [decision, failure]
    required_fields: [owner]
  infra:
    allowed_types: [decision, runbook]
  frontend: {}                   # no extra constraints

# Soft / warn / hard caps per domain. Defaults shown.
governance:
  max_entries: 100
  warn_entries: 150
  hard_limit: 200

# Days before a record demotes one tier on `luuna-atlas prune`. foundational is
# permanent and not configurable here.
classification_defaults:
  shelf_life:
    tactical: 14
    observational: 30

# Output mode for `luuna-atlas prime`. "full" emits every record; "manifest" emits
# a domain index. Switch to manifest on monoliths.
prime:
  default_mode: full

# Search reranker. boost_factor multiplies confirmation outcomes onto BM25
# scores. 0 disables; --no-boost disables at call time.
search:
  boost_factor: 0.1

# Custom record types. See §Custom record types.
custom_types:
  runbook:
    extends: guide
    required: [name, description, steps]
    section_title: "Runbooks"

# Types retired org-wide. Emits a deprecation warning on write.
disabled_types: []

# Lifecycle hooks. See §Hooks.
hooks:
  post-record: [.luuna/atlas/hooks/slack-failures.sh]
hook_settings:
  timeout_ms: 5000

# Decay knobs (only active with `luuna-atlas prune --check-anchors`).
decay:
  anchor_validity:
    threshold: 0.5
    grace_days: 7
```

**Required keys** (omit and Atlas refuses to start): `version`, `domains`, `governance`, `classification_defaults`. Atlas ships sensible defaults for the latter three; the typical real-world config overrides governance caps and adds domains.

**Defaults applied automatically.** Any of the four top-level required keys missing in your file is filled in from `DEFAULT_CONFIG` (see `src/schemas/config.ts`). You only need to spell out the keys you're customizing.

---

## Per-domain rules (`domains`)

`domains` is a map from domain name to per-domain config. Keys:

| Key | Type | Default | Effect |
|-----|------|---------|--------|
| `allowed_types` | `string[]` | `[]` (all allowed) | Gates `--type` on write. Mismatch fails with a hint listing the allowed types. |
| `required_fields` | `string[]` | `[]` (no extras) | Top-level fields every record in the domain must carry, on top of per-type requirements. |

**Worked example.** Lock down `security` to decisions and failures, and require `owner`:

```yaml
domains:
  security:
    allowed_types: [decision, failure]
    required_fields: [owner]
```

After this lands, `luuna-atlas record security --type pattern ...` fails before writing. So does `luuna-atlas record security --type decision --title "..." --rationale "..."` without `--owner`.

**Overlap with `disabled_types`.** Disabled wins. If `security.allowed_types` contains `failure` and `disabled_types` also contains `failure`, writing a failure record still emits the deprecation warning. This is deliberate — disabling a type at the org level shouldn't be silently re-enabled by a stale per-domain config.

**Escape hatch.** Pass `--allow-domain-mismatch` to `luuna-atlas record` or `luuna-atlas validate` to bypass the domain check. This is for worktree/CI lag where the JSONL has landed (via `merge=union`) before the config update has propagated. `luuna-atlas sync` intentionally ignores `--allow-domain-mismatch` — sync is the gate that catches misconfiguration before push.

**Doctor checks.** `luuna-atlas doctor` surfaces two related checks:

- `domain-conformance` (informational) — records that violate per-domain rules but still parse.
- `domain-violations` (failing) — same set, but as a hard fail when run with `--strict`.
- `domain-rules-compatibility` — catches misconfiguration where `required_fields` lists a field no `allowed_types` entry can hold.

---

## Custom record types (`custom_types`)

Declare additional types beyond the six built-ins. Each entry under `custom_types` is keyed by type name.

Fields:

| Field | Type | Required | Effect |
|-------|------|----------|--------|
| `extends` | string | recommended | Name of a built-in type to inherit `required` / `optional` / `dedup_key` / `id_key` / `summary` / `compact` / `section_title` / `extracts_files` / `files_field` from. |
| `required` | `string[]` | optional | Additional required fields. Merges as a union with the parent's `required`. |
| `optional` | `string[]` | optional | Additional optional fields. Union with parent. |
| `dedup_key` | string | optional | Field used for dedup. Overrides parent. |
| `id_key` | string | optional | Field used as the human-readable id stem. Overrides parent. |
| `summary` | string | optional | Summary template. `"{field}"` tokens interpolate record fields. |
| `compact` | enum | optional | One of `concat`, `merge_outcomes`, `keep_latest`, `manual`. |
| `section_title` | string | optional | Heading used in `luuna-atlas prime` output. |
| `extracts_files` | bool | optional | Whether `files_field` should be parsed as file anchors. |
| `files_field` | string | optional | Field name holding file paths. |
| `aliases` | `Record<string, string[]>` | optional | Map canonical field name to legacy names. Disk records carrying a legacy field are rewritten to canonical at read time. |

**Worked example: `release_decision` extending `decision`.**

```yaml
custom_types:
  release_decision:
    extends: decision
    required: [release_tag]
    optional: [rollback_plan]
    summary: "{title} ({release_tag})"
    section_title: "Release decisions"
```

Use it:

```bash
luuna-atlas record releases \
  --type release_decision \
  --title "Cut v2.4 from main-stable" \
  --rationale "Hotfix for CVE-2026-1234 lands today" \
  --release-tag v2.4.0
```

Validation requires `title`, `rationale` (inherited from `decision`), and `release_tag` (added). `--rollback-plan` is accepted but optional.

**Worked example: `incident` extending `failure`.**

```yaml
custom_types:
  incident:
    extends: failure
    required: [severity, pager_link]
    optional: [postmortem_url]
    summary: "[{severity}] {description}"
    section_title: "Incidents"
    aliases:
      pager_link: [pagerduty_url, page_url]   # accept legacy names on read
```

Records written months ago with `pagerduty_url` get rewritten to `pager_link` on every read — no migration script needed.

**Constraints.**

- **Custom-from-custom is not supported in v0.9.0.** `extends` must name one of the six built-ins (`convention`, `pattern`, `failure`, `decision`, `reference`, `guide`). The schema validator rejects custom-from-custom at config load.
- **AJV preserves `additionalProperties: false`.** A custom type cannot accept arbitrary extra fields; everything must be declared in `required` or `optional` (or inherited).
- **Extending a disabled type is rejected.** If `failure` is in `disabled_types`, `custom_types.incident.extends: failure` fails config validation. Retire the parent only after migrating children.

---

## Retiring types (`disabled_types`)

A list of registered type names (built-in or custom) that are being retired. Writes still succeed but emit a deprecation warning. Reads always succeed. `--type` choices in CLI help still include the type so peer repos that haven't synced the new config aren't surprised mid-command.

```yaml
disabled_types: [reference, runbook_v1]
```

**When to use this vs deleting the custom type entirely.**

- **Use `disabled_types`** when:
  - Peer repos in your org still write the type and haven't pulled the config update yet.
  - Existing records on disk need to remain readable.
  - You want a soft migration window where new writes flag but old code keeps working.
- **Delete the `custom_types` entry entirely** when:
  - All historical records have been migrated to the replacement type.
  - No peer repo writes the type anymore.
  - You're confident `luuna-atlas doctor` reports zero records of this type across all repos.

**Conflict semantics.** `disabled_types` wins over `allowed_types` on overlap (see §Per-domain rules). This is deliberate so a stale `allowed_types` entry doesn't silently un-retire a type.

---

## Lifecycle hooks (`hooks`, `hook_settings`)

Hooks run external scripts at lifecycle events. Four events, two semantics each:

| Event | Blocks on non-zero? | Mutable via stdout? | Stdin payload |
|-------|---------------------|---------------------|---------------|
| `pre-record` | yes | yes | `{ event, payload: <ExpertiseRecord> }` |
| `post-record` | no (warns) | no (stdout ignored) | `{ event, payload: <ExpertiseRecord> }` |
| `pre-prime` | yes | yes | `{ event, payload: { domains: [{ domain, records }] } }` |
| `pre-prune` | yes | no (stdout ignored) | `{ event, payload: { candidates: [{ domain, stale, demote, anchor_decay }] } }` |

**Execution contract.**

- **Working directory**: project root (the repo containing `.luuna/atlas/`).
- **Environment**: parent env preserved, plus `LUUNA_ATLAS_HOOK=1`.
- **Process group**: detached, so SIGKILL on timeout cleans up children.
- **stdin**: payload JSON.
- **stdout**: read only on mutable events (`pre-record`, `pre-prime`). Accepts both `{ event, payload }` and bare-payload shapes — pick whichever is convenient.
- **stderr**: forwarded to the calling terminal.
- **Exit code**:
  - `0`: continue.
  - non-zero from `pre-*`: block the operation, print the script's stderr.
  - non-zero from `post-*`: warn, continue.

**Settings.**

```yaml
hook_settings:
  timeout_ms: 5000     # default 5000ms; SIGKILL on timeout
```

**Dry-run.** `luuna-atlas record --dry-run` and `luuna-atlas prune --dry-run` skip hooks. Preview commands never trigger Slack posts or external confirmation flows.

### Payload examples

`pre-record` / `post-record`:

```json
{
  "event": "pre-record",
  "payload": {
    "id": "mx-2026-05-13-1",
    "type": "failure",
    "classification": "tactical",
    "description": "AJV strict mode rejects schema missing type: object",
    "resolution": "Always include type: object alongside required/properties",
    "recorded_at": "2026-05-13T10:24:00Z",
    "evidence": { "commit": "abc1234", "file": "src/schemas/foo.ts" }
  }
}
```

`pre-prime`:

```json
{
  "event": "pre-prime",
  "payload": {
    "domains": [
      { "domain": "schemas", "records": [ { "id": "mx-...", "...": "..." } ] }
    ]
  }
}
```

`pre-prune`:

```json
{
  "event": "pre-prune",
  "payload": {
    "candidates": [
      {
        "domain": "frontend",
        "stale":       [ { "id": "mx-...", "reason": "tactical age 16d" } ],
        "demote":      [ { "id": "mx-...", "from": "tactical", "to": "observational" } ],
        "anchor_decay":[ { "id": "mx-...", "valid_fraction": 0.2 } ]
      }
    ]
  }
}
```

### Worked example: Slack-notify on new failures

`.luuna/atlas/hooks/slack-failures.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Read payload from stdin.
payload=$(cat)

# Only react to failure records.
type=$(echo "$payload" | jq -r '.payload.type // empty')
[ "$type" = "failure" ] || exit 0

desc=$(echo "$payload" | jq -r '.payload.description')
id=$(echo "$payload"   | jq -r '.payload.id')
repo=$(basename "$PWD")

# Fire-and-forget. Non-zero exit warns but does not block the record.
curl -fsS -X POST "$SLACK_WEBHOOK_URL" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg t "[$repo] new failure: $desc ($id)" '{text: $t}')" \
  > /dev/null

exit 0
```

Wire it up:

```yaml
hooks:
  post-record: [.luuna/atlas/hooks/slack-failures.sh]
```

`SLACK_WEBHOOK_URL` is read from the parent env, which is preserved into the hook.

---

## Prune, decay, and archive (`decay`, `--check-anchors`)

`luuna-atlas prune` is the demotion engine. Default behavior is **soft-archive**, not delete.

**The demotion ladder (hardcoded):**

```
foundational  →  tactical  →  observational  →  archived
```

One tier per `luuna-atlas prune` pass. `--aggressive` collapses straight to archived in a single pass.

**Soft-archive.** Records demoted past `observational` move to `.luuna/atlas/archive/<domain>.jsonl` with `status: "archived"` and `archived_at: <iso-date>`. Each archive file begins with a `# ARCHIVED — ...` banner. `readExpertiseFile` skips `#`-prefixed lines, so banners never confuse parsers.

**Hard delete.** `luuna-atlas prune --hard` skips the archive and removes records outright. Reserve this for sensitive data that shouldn't persist anywhere.

**Restore.** `luuna-atlas restore <id>` brings a soft-archived record back to live expertise. The classification resets to `observational`.

**Read paths.** `luuna-atlas prime` and the default `luuna-atlas search` never read archive files. `luuna-atlas search --archived <query>` is the only built-in read path that walks `.luuna/atlas/archive/`.

### Supersession decay

When record B has `supersedes: [A]`, A walks one tier per `luuna-atlas prune` pass. Cross-domain by design — if a `frontend` decision supersedes an `infra` decision, the infra record demotes regardless. Each step stamps `supersession_demoted_at: <iso-date>` on A. `--aggressive` collapses A to archived in a single pass.

### Anchor-validity decay

Opt in with `luuna-atlas prune --check-anchors`. Records whose declared file or directory anchors stop resolving (the underlying path was deleted, moved, etc.) demote one tier per pass when **both** of these are true:

- The fraction of valid anchors is below `decay.anchor_validity.threshold`.
- The record's age exceeds `decay.anchor_validity.grace_days`.

Records with zero anchors are exempt — absence is interpreted as "applies globally."

Config:

```yaml
decay:
  anchor_validity:
    threshold: 0.5     # default 0.5; demote when valid_fraction < this. Range [0, 1].
    grace_days: 7      # default 7; records younger than this are exempt.
    weight: 0.3        # reserved for a future fitness blend; currently unused.
```

`threshold` outside `[0, 1]` and negative `grace_days` are validated at command time and produce a human-readable error before any record is touched.

Demotions stamp `anchor_decay_demoted_at: <iso-date>`.

### Conflict resolution

When supersession decay and anchor-validity decay both want to demote the same record, **staleness wins**. Atlas picks the tier walk caused by classification age and skips the intermediate demotion stamp. `--explain` prints per-record reasons:

```bash
luuna-atlas prune --check-anchors --explain --dry-run
```

emits a line per candidate showing which decay path fired and why.

---

## Prime output tuning (`prime`)

```yaml
prime:
  default_mode: full       # "full" (default) or "manifest"
  session_close:
    style: conditional     # directive | conditional (default) | minimal | none
    # custom: |
    #   Verbatim prose to emit instead of the preset. Wins over `style`.
  tier_weights:            # full-mode trust ordering
    star: 100
    foundational: 50
    tactical: 20
    observational: 10
```

**`full`** (the default) emits the record index: one line per record, failures first, capped by `--budget` (default 4000 tokens). `luuna-atlas show <id>` prints a full record.

**`manifest`** emits a domain index and per-domain record counts plus a quick reference of common commands. Agents then scope-load with `luuna-atlas prime <domain>` or `luuna-atlas prime --files <path>`. On a 12k-token expertise store, manifest mode typically drops session-priming output to under 1k tokens.

Override per invocation:

- `luuna-atlas prime --manifest` — manifest mode regardless of config.
- `luuna-atlas prime --full` — full record bodies regardless of config.
- `luuna-atlas prime --files src/foo.ts` — load only records relevant to the listed paths, with per-file framing, classification age, and confirmation scores.

### Session-close footer (`prime.session_close`)

`luuna-atlas prime` (and the snippet `luuna-atlas setup` / `luuna-atlas onboard` writes into CLAUDE.md / AGENTS.md / `.cursor/rules/luuna-atlas.mdc`) ends with a footer reminding the agent to record insights before closing the session. The footer is configurable so each project can dial in the right voice without forking:

- **`style: directive`** — numbered imperative + inline type glossary + dedup nudge + anti-filler examples. Loudest. Use when the team needs reminding that `luuna-atlas record` exists at all.
- **`style: conditional`** (default) — "if you discovered insights worth preserving, record them; skip if no insight surfaced." The v0.10.0 reframing. Use for teams already recording consistently.
- **`style: minimal`** — one-line nudge.
- **`style: none`** — suppress the footer entirely.
- **`custom: "<prose>"`** — verbatim override. Wins over `style`. Use for org-specific phrasing or non-English voices.

The 🚨 marker is preserved on `directive` for `markdown` / `plain` prime footers (memory anchor in long sessions) and dropped from the embedded variant (which lives at the top of CLAUDE.md, not at end-of-session). Re-run `luuna-atlas setup <provider>` and `luuna-atlas onboard` after changing this knob — the existing files check stale until you do.

---

## Search tuning (`search`)

```yaml
search:
  boost_factor: 0.1        # default 0.1; multiplier on confirmation outcomes
```

`luuna-atlas search` ranks results with BM25, then applies a confirmation boost: records with positive `outcomes` (an IC marked the record as having helped) rerank higher. `boost_factor` scales the multiplier. Setting it to `0` disables boost. `luuna-atlas search --no-boost` disables it for a single call.

Tune higher if your team is diligent about marking outcomes and you want confirmed records to dominate ranking. Tune lower (or zero) if outcomes are inconsistently recorded and you want raw text relevance.

---

## Governance and classifications

### Governance caps

```yaml
governance:
  max_entries: 100         # soft target; luuna-atlas status flags above this
  warn_entries: 150        # luuna-atlas record prints a "consider compacting" hint
  hard_limit: 200          # writes past this are refused
```

All three are per-domain, not global. The flow is:

1. Below `max_entries`: silent.
2. Between `max_entries` and `warn_entries`: `luuna-atlas status` warns, writes proceed.
3. Between `warn_entries` and `hard_limit`: every `luuna-atlas record` write prints a "consider compacting" hint.
4. At `hard_limit`: writes are refused with a hint to run `luuna-atlas compact <domain>`.

Tune `hard_limit` up only after auditing prime output cost — every extra record is tokens in the agent's session start.

### Classification shelf life

```yaml
classification_defaults:
  shelf_life:
    tactical: 14           # days
    observational: 30      # days
```

Records older than the shelf life become demotion candidates on the next `luuna-atlas prune` pass. **Foundational is permanent and not configurable here** — by definition, foundational is "load-bearing forever." If a record should expire, classify it tactical or observational at record time.

The ladder (`foundational → tactical → observational → archived`) is hardcoded. You can change the speed of the walk via shelf life, but not the steps.

---

## Provider recipes

`luuna-atlas setup <name>` wires Atlas into a coding agent runtime. Recipes are pluggable.

### Discovery order

1. **Filesystem**: `.luuna/atlas/recipes/<name>.ts` or `.luuna/atlas/recipes/<name>.sh` (TypeScript wins on tie).
2. **npm**: a package named `luuna-atlas-recipe-<name>` resolvable from the repo.
3. **Built-in**: one of the recipes shipped in `src/commands/setup.ts`.

Filesystem wins so an org can shadow built-ins without forking Atlas.

### Built-ins as of v0.10.2

- `claude` — writes `SessionStart` hook into `.claude/`. `PreCompact` is intentionally not registered because its stdout is discarded across compaction.
- `cursor` — writes Cursor's project rules surface.
- `codex` — writes both an `AGENTS.md` luuna-atlas section (fallback prose) and a `[[hooks.SessionStart]]` block in `.codex/config.toml` fenced by `# luuna-atlas:start` / `# luuna-atlas:end` line comments for idempotency.

**Removed in v0.9.0**: `aider`, `gemini`, `windsurf`. **Removed in v0.10.2**: `pi` (experiment rolled back). Users who relied on any of these can re-create the same behavior as a filesystem recipe under `.luuna/atlas/recipes/<name>.{ts,sh}`.

### TypeScript recipe shape

```typescript
import type { ProviderRecipe, RecipeResult } from "@luuna/atlas-cli";

const recipe: ProviderRecipe = {
  async install(cwd: string): Promise<RecipeResult> {
    // idempotent install
    return { success: true, message: "installed" };
  },
  async check(cwd: string): Promise<RecipeResult> {
    return { success: true, message: "ok" };
  },
  async remove(cwd: string): Promise<RecipeResult> {
    return { success: true, message: "removed" };
  },
};

export default recipe;
```

`install` / `check` / `remove` must each be idempotent. Bun loads `.ts` recipes directly — no build step. The default export shape is validated at load; bad shapes fail before `install` runs.

### Shell recipe contract

```
<script> install|check|remove
```

- **cwd**: project root.
- **env**: `LUUNA_ATLAS_RECIPE_NAME=<name>`, `LUUNA_ATLAS_RECIPE_ACTION=<install|check|remove>`, plus all parent env.
- **stdout/stderr**: forwarded to the calling terminal.
- **Exit code**: `0` for success, non-zero for failure (the action is reported as failed and the message is the script's stderr).

### Worked example: an internal `windsurf-org` recipe

`.luuna/atlas/recipes/windsurf-org.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

action="${1:?action required}"
target_dir=".windsurf-org"
target_file="$target_dir/agent_context.md"
fence_start="<!-- luuna-atlas:start -->"
fence_end="<!-- luuna-atlas:end -->"

case "$action" in
  install)
    mkdir -p "$target_dir"
    touch "$target_file"
    tmp=$(mktemp)
    awk -v s="$fence_start" -v e="$fence_end" '
      $0==s {skip=1; next}
      $0==e {skip=0; next}
      !skip {print}
    ' "$target_file" > "$tmp"
    {
      cat "$tmp"
      echo "$fence_start"
      echo "Run: luuna-atlas prime"
      echo "$fence_end"
    } > "$target_file"
    rm "$tmp"
    echo "windsurf-org installed at $target_file"
    ;;
  check)
    grep -q "$fence_start" "$target_file" 2>/dev/null \
      && { echo "windsurf-org installed"; exit 0; } \
      || { echo "windsurf-org not installed"; exit 1; }
    ;;
  remove)
    [ -f "$target_file" ] || { echo "nothing to remove"; exit 0; }
    tmp=$(mktemp)
    awk -v s="$fence_start" -v e="$fence_end" '
      $0==s {skip=1; next}
      $0==e {skip=0; next}
      !skip {print}
    ' "$target_file" > "$tmp"
    mv "$tmp" "$target_file"
    echo "windsurf-org removed"
    ;;
  *)
    echo "unknown action: $action" >&2
    exit 2
    ;;
esac
```

Make it executable (`chmod +x`), commit it under `.luuna/atlas/recipes/`, and `luuna-atlas setup windsurf-org` Just Works in every repo that pulls the scaffolding.

`luuna-atlas setup --list` enumerates everything found with source + shadow markers, so it's easy to verify the filesystem recipe is winning over a built-in of the same name.

---

## Distributing config across an org

The recommended pattern:

1. **Commit `config.yaml` in every repo.** Don't try to centralize at the org level — repos move at different speeds, and a config out of sync with on-disk records is worse than a stale-but-consistent config.
2. **Seed identical config via repo scaffolding.** A cookiecutter template, an internal `degit` source, or a tool like Backstage that scaffolds new repos with a known `.luuna/atlas/` baseline.
3. **Roll out config changes via PRs.** Every change to `config.yaml` is a code review just like any other. `luuna-atlas sync` is the safety net that catches misconfiguration before the change merges.
4. **Use `disabled_types` and `--allow-unknown-types` to absorb rollout lag.** Two repos sharing expertise (via copy, fork, or external sync) will occasionally disagree on whether a custom type exists. `disabled_types` preserves CLI choices through the transition. `--allow-unknown-types` is the per-invocation escape hatch for reads against records of an unknown type.

### `.gitattributes` setup

```
.luuna/atlas/expertise/*.jsonl merge=union
.luuna/atlas/archive/*.jsonl   merge=union
```

`merge=union` is critical at scale. Two ICs working on different branches both running `luuna-atlas record` produces append-only writes that merge cleanly without conflicts. Combined with file locking (advisory, 50ms retry, 5s timeout, 30s stale detection) and atomic writes (temp file + rename), concurrent agents on the same repo never produce partial JSONL.

### Worktree behavior

`getAtlasDir()` resolves `.luuna/atlas/` to the main repo when invoked from a `git worktree`. Expertise survives worktree cleanup — a record written from `repo.worktrees/feature-x/` lands in `repo/.luuna/atlas/expertise/<domain>.jsonl`, not in the throwaway worktree.

`isInsideWorktree()` guards against a submodule false positive: `git rev-parse --git-common-dir` returns `/parent/.git/modules/<name>` for submodules (without a `.git` suffix), which the naive worktree check would treat as a worktree. The guard correctly identifies the submodule case and uses the local `.git`.

### `luuna-atlas sync` is the gate

`luuna-atlas sync` re-validates every staged record against the on-disk config before committing. It **ignores** `--allow-unknown-types` and `--allow-domain-mismatch` by design — those flags are for unblocking writes during rollout lag, not for bypassing the validation gate that protects the canonical history.

If `luuna-atlas sync` rejects something, the fix is one of:

- Update the record to match current config.
- Update config to accept the record (then commit the config change first).
- If a custom type is being retired, add it to `disabled_types` rather than removing it from `custom_types`.

---

# Part 2 — IC guide

## Base record fields (available on every type)

Every record carries these fields regardless of type. Most are set automatically; a few are worth knowing about.

| Field | Type | Set by | Notes |
|-------|------|--------|-------|
| `id` | string | Atlas | Human-readable id stem + date. Stable across edits. |
| `type` | string | you (`--type`) | One of the registered types (built-in or custom). |
| `classification` | enum | you (`--classification`) | `foundational` / `tactical` / `observational`. Default `tactical`. |
| `recorded_at` | ISO 8601 string | Atlas | Set at write time. |
| `evidence` | object | mixed | Sub-fields: `commit`, `date`, `issue`, `file`, `bead`, `missions`, `gh`, `linear`. Commit + changed files auto-populate from git. |
| `tags` | `string[]` | you (`--tags`) | Free-form. Searchable. |
| `relates_to` | `string[]` | you (`--relates-to`) | mx-ids of related records. |
| `supersedes` | `string[]` | you (`--supersedes`) | mx-ids this record replaces. Triggers supersession decay on `luuna-atlas prune`. |
| `outcomes` | object | `luuna-atlas outcome` | Confirmation counts. Drives search boost. |
| `dir_anchors` | `string[]` | mixed | Directory paths the record applies to. Drives `luuna-atlas prime --files` filtering. |
| `owner` | string | you (`--owner`) | Reserved, opaque to Atlas — use it however your org wants. |
| `status` | enum | Atlas | `draft` / `active` / `deprecated` / `archived`. |
| `archived_at` | ISO 8601 string | Atlas | Set when prune archives the record. |
| `supersession_demoted_at` | ISO 8601 string | Atlas | Set when supersession decay demoted this record. |
| `anchor_decay_demoted_at` | ISO 8601 string | Atlas | Set when anchor-validity decay demoted this record. |

---

## Built-in record types (the six)

| Type | Required | Optional | Dedup key | Summary template |
|------|----------|----------|-----------|------------------|
| `convention` | `content` | — | `content` | `{content}` |
| `pattern` | `name`, `description` | `files` | `name` | `{name}` |
| `failure` | `description`, `resolution` | — | `description` | `{description}` |
| `decision` | `title`, `rationale` | `date` | `title` | `{title}` |
| `reference` | `name`, `description` | `files` | `name` | `{name}` |
| `guide` | `name`, `description` | `content`, `files` | `name` | `{name}` |

Each example below is the minimum to make `luuna-atlas record` succeed.

```bash
# convention
luuna-atlas record style --type convention \
  --content "Use .ts extensions on all relative imports"

# pattern
luuna-atlas record adapters --type pattern \
  --name "Atomic write via temp + rename" \
  --description "Write to <path>.tmp.<pid>, then rename to <path>" \
  --files src/utils/expertise.ts

# failure
luuna-atlas record schemas --type failure \
  --description "AJV strict mode rejects schema missing type: object" \
  --resolution "Include type: object alongside required/properties"

# decision
luuna-atlas record architecture --type decision \
  --title "Soft-archive over hard-delete on prune" \
  --rationale "Archived records remain searchable via luuna-atlas search --archived"

# reference
luuna-atlas record vendors --type reference \
  --name "Bun runtime docs" \
  --description "Bun handles ESM/CJS interop transparently for AJV"

# guide
luuna-atlas record onboarding --type guide \
  --name "First-day-on-team setup" \
  --description "Steps for new ICs to get a working dev environment" \
  --content "1. Install bun. 2. bun install. 3. bun test."
```

---

## Global CLI flags

| Flag | When to use |
|------|-------------|
| `--allow-unknown-types` | Reading expertise that references a type not registered in the local config. Common during org rollout lag. |
| `--allow-domain-mismatch` | Writing a record where per-domain rules don't yet match an on-disk update. Same lag scenario. `luuna-atlas sync` ignores this. |
| `--quiet` / `-q` | Suppress informational output. Useful in scripts. |
| `--json` | Emit JSON instead of formatted text. For piping into `jq` or other tools. |
| `--verbose` | Extra diagnostic output. Use when something looks off. |
| `--format <markdown\|compact\|xml\|plain>` | Output format for read commands. `markdown` is default; `compact` is the IC-friendly TUI mode; `xml` and `plain` are for tool integration. |

---

## Environment variables

| Variable | Set by | Available where |
|----------|--------|-----------------|
| `LUUNA_ATLAS_HOOK` | Atlas (set to `1`) | Inside hook scripts. Detect with `[ "${LUUNA_ATLAS_HOOK:-}" = "1" ]`. |
| `LUUNA_ATLAS_RECIPE_NAME` | Atlas | Inside recipe scripts (shell only). |
| `LUUNA_ATLAS_RECIPE_ACTION` | Atlas | Inside recipe scripts. One of `install` / `check` / `remove`. |
| `LUUNA_ATLAS_DEBUG` | You (operator) | Read by `luuna-atlas` at startup. Set to any non-empty value to raise the diagnostic logger to `debug` (hook traces, registry-init failures). Diagnostics go to **stderr** only and never touch stdout / `--json` output. |
| `LUUNA_ATLAS_LOG_LEVEL` | You (operator) | Read by `luuna-atlas` at startup. Authoritative log level (`trace`/`debug`/`info`/`warn`/`error`/`fatal`); overrides `LUUNA_ATLAS_DEBUG`. Default `info` (silent for routine internals). |
| `LUUNA_ATLAS_LOG_JSON` | You (operator) | Read by `luuna-atlas` at startup. Set to `1` to force newline-delimited JSON logs even in a TTY (the default pretty-renders to stderr in an interactive terminal). |

Parent env is preserved into both hooks and recipes. `SLACK_WEBHOOK_URL`, `OPS_TOKEN`, whatever you have exported in the agent's session is visible to the script.

The `LUUNA_ATLAS_*` *logging* variables (`LUUNA_ATLAS_DEBUG`, `LUUNA_ATLAS_LOG_LEVEL`, `LUUNA_ATLAS_LOG_JSON`) control luuna-atlas's diagnostic channel only — they never alter product output. Atlas's user-facing output (the markdown `luuna-atlas prime` emits, status tables, `--json` results) always goes to stdout; the logger writes exclusively to stderr, so turning on `LUUNA_ATLAS_DEBUG` is safe even when piping `--json` to a machine consumer.

---

## Working in worktrees and across branches

**Worktree resolution.** `.luuna/atlas/` always resolves to the main repo, even when you invoke `luuna-atlas` from a worktree directory. A record written from `~/repo.worktrees/feat-x/` lands in `~/repo/.luuna/atlas/expertise/<domain>.jsonl`. Worktree cleanup never loses expertise.

**Concurrent writes.** Two ICs writing to the same domain on different branches produce two appends to the same JSONL file. The combination of:

- `.gitattributes` `merge=union` on expertise files,
- Advisory file locking (`withFileLock`),
- Atomic writes (temp + rename),

means concurrent writes never produce partial or corrupt JSONL, and post-merge JSONL is the union of both branches' records.

If two branches write a record with the same `dedup_key`, both records are preserved on disk after merge. Run `luuna-atlas compact <domain>` to collapse them according to the type's compact strategy.

---

## When your org has custom types

Discovery from the command line:

- `luuna-atlas --help` lists registered types in the description of the `--type` flag.
- `luuna-atlas record --help` shows the allowed `--type` values, including custom types.
- `luuna-atlas record <domain> --type <name> --help` shows required/optional flags for that combination — including any extra `required_fields` from the domain config.

**How required fields surface as CLI flags.** A `required_fields: [owner]` entry in `domains.security` means `luuna-atlas record security ...` accepts (and demands) `--owner`. The flag name follows the field name with `_` replaced by `-`.

**Validate your local records against org rules.** Run `luuna-atlas doctor` to check that everything you've written is conformant. Common checks:

- `domain-conformance` / `domain-violations` — per-domain rule conformance.
- `type-registry` — built-in vs custom types in use; counts; disabled markers.
- `unknown-types` — fails if records reference a type the local config doesn't know about. Pass `--allow-unknown-types` to relax.

`luuna-atlas doctor --fix` strips broken file anchors but does not move records between domains or rewrite fields. Bigger fixes need an explicit `luuna-atlas edit`.

---

# Appendix

## A. Full config-key index (alphabetical)

| Key path | Type | Default | Notes |
|----------|------|---------|-------|
| `classification_defaults.shelf_life.observational` | int | `30` | Days before observational records demote. |
| `classification_defaults.shelf_life.tactical` | int | `14` | Days before tactical records demote. |
| `custom_types.<name>.aliases` | `Record<string, string[]>` | `{}` | Legacy field name map; rewritten on read. |
| `custom_types.<name>.compact` | enum | inherited | `concat` / `merge_outcomes` / `keep_latest` / `manual`. |
| `custom_types.<name>.dedup_key` | string | inherited | Field used for dedup. |
| `custom_types.<name>.extends` | string | — | Built-in type to inherit from. Custom-from-custom not supported. |
| `custom_types.<name>.extracts_files` | bool | inherited | Whether to extract file anchors from `files_field`. |
| `custom_types.<name>.files_field` | string | inherited | Field holding file paths. |
| `custom_types.<name>.id_key` | string | inherited | Field used as id stem. |
| `custom_types.<name>.optional` | `string[]` | inherited | Optional fields. Union with parent. |
| `custom_types.<name>.required` | `string[]` | inherited | Required fields. Union with parent. |
| `custom_types.<name>.section_title` | string | inherited | Heading in prime output. |
| `custom_types.<name>.summary` | string | inherited | Template with `{field}` tokens. |
| `decay.anchor_validity.grace_days` | int | `7` | Records younger than this are exempt from anchor decay. |
| `decay.anchor_validity.threshold` | number | `0.5` | Demote when `valid_fraction < threshold`. Range `[0, 1]`. |
| `decay.anchor_validity.weight` | number | — | Reserved; unused in v0.9.0. |
| `disabled_types` | `string[]` | `[]` | Types that emit deprecation warning on write. Wins over `allowed_types`. |
| `domains.<name>.allowed_types` | `string[]` | `[]` (all) | Gates `--type` on write. |
| `domains.<name>.required_fields` | `string[]` | `[]` | Extra required fields per domain. |
| `governance.hard_limit` | int | `200` | Writes past this are refused. |
| `governance.max_entries` | int | `100` | Soft target; status flags above this. |
| `governance.warn_entries` | int | `150` | `luuna-atlas record` prints compaction hint. |
| `hook_settings.timeout_ms` | int | `5000` | Per-hook timeout, ms. SIGKILL on timeout. |
| `hooks.post-record` | `string[]` | — | Scripts run after a successful record. Non-zero warns. |
| `hooks.pre-compact` | `string[]` | — | Scripts run before `luuna-atlas compact`. Mutable (replacement record). Non-zero blocks. |
| `hooks.pre-prime` | `string[]` | — | Scripts run before `luuna-atlas prime`. Mutable. Non-zero blocks. |
| `hooks.pre-prune` | `string[]` | — | Scripts run before `luuna-atlas prune`. Non-zero blocks. Stdout ignored. |
| `hooks.pre-record` | `string[]` | — | Scripts run before a record is written. Mutable. Non-zero blocks. |
| `prime.default_mode` | enum | `full` | `full` or `manifest`. |
| `prime.session_close.custom` | string | — | Verbatim footer prose. Wins over `style`. |
| `prime.session_close.style` | enum | `conditional` | `directive` / `conditional` / `minimal` / `none`. |
| `prime.tier_weights.foundational` | int | `50` | Trust-tier weight for foundational records in full-mode ordering. |
| `prime.tier_weights.observational` | int | `10` | Trust-tier weight for observational records. |
| `prime.tier_weights.star` | int | `100` | Weight applied per confirmation outcome (★ count). |
| `prime.tier_weights.tactical` | int | `20` | Trust-tier weight for tactical records. |
| `search.boost_factor` | number | `0.1` | Outcome confirmation boost multiplier. `0` disables. |
| `version` | string | `"1"` | Config schema version. |

All keys are defined in `src/schemas/config.ts`.

## B. Hook event quick reference

| Event | Blocks on non-zero? | Mutable via stdout? | Payload root |
|-------|---------------------|---------------------|--------------|
| `pre-record` | yes | yes | `{ event, payload: ExpertiseRecord }` |
| `post-record` | no (warns) | no | `{ event, payload: ExpertiseRecord }` |
| `pre-prime` | yes | yes | `{ event, payload: { domains: [...] } }` |
| `pre-prune` | yes | no | `{ event, payload: { candidates: [...] } }` |
| `pre-compact` | yes | yes (`{ replacement: <Record> }`) | `{ event, payload: { domain, candidates, merged } }` |

Common contract for all five:

- `cwd` = project root.
- `LUUNA_ATLAS_HOOK=1` plus all parent env.
- stdin = JSON payload.
- stderr forwarded to caller.
- SIGKILL on `hook_settings.timeout_ms` (default 5000).
- Dry-run skips hooks.

## C. All CLI flags

Global, available on every command:

| Flag | Description |
|------|-------------|
| `--allow-unknown-types` | Relax `unknown-types` error when reading. |
| `--allow-domain-mismatch` | Relax per-domain validation on `luuna-atlas record` / `luuna-atlas validate`. Ignored by `luuna-atlas sync`. |
| `--quiet`, `-q` | Suppress informational output. |
| `--json` | Emit JSON. |
| `--verbose` | Extra diagnostic output. |
| `--format <markdown\|compact\|xml\|plain>` | Output format for read commands. |

Notable command-specific flags referenced in this doc:

| Flag | Command | Effect |
|------|---------|--------|
| `--manifest` | `luuna-atlas prime` | Force manifest mode for this call. |
| `--full` | `luuna-atlas prime` | Force full mode for this call. |
| `--files <path...>` | `luuna-atlas prime` | Scope-load records by file relevance. |
| `--no-boost` | `luuna-atlas search` | Disable confirmation boost. |
| `--archived` | `luuna-atlas search` | Walk `.luuna/atlas/archive/` too. |
| `--check-anchors` | `luuna-atlas prune` | Enable anchor-validity decay. |
| `--aggressive` | `luuna-atlas prune` | Collapse straight to archived in one pass. |
| `--hard` | `luuna-atlas prune` | True delete instead of soft archive. |
| `--explain` | `luuna-atlas prune` | Per-record demotion reasons. |
| `--dry-run` | `luuna-atlas record`, `luuna-atlas prune` | Preview; skips hooks. |
| `--fix` | `luuna-atlas doctor` | Strip broken file anchors. |
| `--strict` | `luuna-atlas doctor` | Fail on informational checks too. |
| `--list` | `luuna-atlas setup` | Enumerate recipes with source + shadow markers. |

## D. File path reference

Every path Atlas reads or writes under the repo root:

| Path | Purpose | Written by | Read by |
|------|---------|------------|---------|
| `.luuna/atlas/config.yaml` | Configuration | you / `luuna-atlas init` | every command |
| `.luuna/atlas/expertise/<domain>.jsonl` | Live records | `luuna-atlas record`, `luuna-atlas edit`, `luuna-atlas compact`, `luuna-atlas restore`, `luuna-atlas doctor --fix` | `luuna-atlas prime`, `luuna-atlas search`, `luuna-atlas query`, `luuna-atlas rank`, `luuna-atlas status`, `luuna-atlas validate`, `luuna-atlas learn`, `luuna-atlas ready`, `luuna-atlas sync` |
| `.luuna/atlas/archive/<domain>.jsonl` | Soft-archived records | `luuna-atlas prune` | `luuna-atlas search --archived`, `luuna-atlas restore` |
| `.luuna/atlas/recipes/<name>.ts` | TypeScript provider recipe | you | `luuna-atlas setup`, `luuna-atlas setup --list` |
| `.luuna/atlas/recipes/<name>.sh` | Shell provider recipe | you | `luuna-atlas setup`, `luuna-atlas setup --list` |
| `.luuna/atlas/hooks/*.sh` (or anywhere) | Lifecycle hook scripts | you | `luuna-atlas record`, `luuna-atlas prime`, `luuna-atlas prune` |
| `.luuna/atlas/README.md` | Auto-generated layout doc | `luuna-atlas init` | humans |
| `.gitattributes` (repo root) | `merge=union` rules | you | git |

Lock files (`.luuna/atlas/expertise/<domain>.jsonl.lock` and similar) are created and removed by `withFileLock` during writes. They should not persist between commands; if you find a stale one, it will self-clear after the 30s stale-lock window.
