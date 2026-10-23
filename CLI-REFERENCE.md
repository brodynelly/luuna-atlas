# Luuna Atlas CLI reference

Generated from the local CLI help at commit 27477c313a3049b1579dfdc32c39da600726721f, using Bun 1.4.3. This is the built-in command surface; project custom types add record and edit flags at startup. See [the system map](CLI-SYSTEM-MAP.md) for behavior, workflows, integration boundaries, and limitations.

Global options are inherited even when a command help block does not repeat them. The custom help formatter prints generic usage and omits positional argument sections; consult the system map command tables for exact invocation syntax. This transcript preserves the actual help text, including its documented limitations.

## Global help

```text
Luuna Atlas v0.11.0 — Structured expertise management

Usage: luuna-atlas <command> [options]

Commands:
  init                Initialize .luuna/atlas/ in the current project
  add <domain>        Add a new expertise domain
  record [options] <domain> [content]  Record an expertise record
  edit [options] <domain> <id>  Edit an existing expertise record
  query [options] [domain]  Query expertise records
  setup [options] [provider]  Set up luuna-atlas integration for a specific agent provider
  prime [options] [domains...]  Generate a priming prompt from expertise records
  hook                Claude Code PreToolUse handler (installed by `luuna-atlas setup claude`): injects file-anchored records, denies hand edits of .luuna/atlas/expertise/*.jsonl; reads hook JSON on stdin
  onboard [options]   Generate or update a CLAUDE.md/AGENTS.md snippet pointing to luuna-atlas prime
  status              Show status of expertise records
  validate            Validate expertise records against schemas
  prune [options]     Soft-archive (default) or hard-delete stale records, plus tier-demote superseded ones
  archive [options] <domain> [id]  Soft-archive one or more live records (symmetric to `luuna-atlas restore`; pairs with `luuna-atlas prune` for bulk decay)
  restore <id>        Restore a soft-archived record from .luuna/atlas/archive/ back to live expertise
  search [options] [query]  Search expertise records across domains
  show <ids...>       Show full record bodies by ID (the detail behind `luuna-atlas prime`'s index)
  stale [domains...]  List records that look stale: anchors changed since the evidence commit, missing anchors, or past shelf life unconfirmed
  rank [options] [domain]  Rank records by confirmation-frequency score (highest first)
  outcome [options] <domain> <id>  Append an outcome to an existing expertise record
  doctor [options]    Run health checks on expertise records
  ready [options]     Show recently added or updated expertise records
  sync [options]      Validate, stage, and commit .luuna/atlas/ changes
  delete [options] <domain> [id]  Delete an expertise record or multiple records in bulk
  delete-domain [options] <domain>  Delete an expertise domain and its expertise file
  move [options] <source-domain> <id> <target-domain>  Move a record from one domain to another, preserving its ID and metadata
  learn [options]     Show changed files and suggest domains for recording learnings
  compact [options] [domain]  Compact records: analyze candidates or apply a compaction
  config              Read and write .luuna/atlas/config.yaml
  diff [options]      Show expertise record changes since a git ref
  upgrade [options]   Upgrade Luuna Atlas after publication
  completions <shell>  Output shell completion script (bash, zsh, fish)
  audit [options]     Audit corpus health (evidence coverage, rule density, floaters, per-domain mix)

Options:
  -v, --version       Print version
  --json              Output as structured JSON
  -q, --quiet         Suppress non-error output
  --verbose           Show full details in output
  --timing            Print execution time to stderr
  --allow-unknown-types  tolerate on-disk records of unregistered types (worktree/CI lag escape hatch)
  --allow-domain-mismatch  tolerate records that violate per-domain allowed_types/required_fields (escape hatch for record/validate; ignored by sync)
  --format <format>   output format for record-rendering commands (prime, query, search) (choices: "markdown", "compact", "xml", "plain")
  -h, --help          display help for command

Run 'luuna-atlas <command> --help' for command-specific help.
```

## add

```text
add v0.11.0 — Structured expertise management

Usage: add <command> [options]

Options:
  -h, --help          display help for command

Run 'add <command> --help' for command-specific help.
```

## archive

```text
archive v0.11.0 — Structured expertise management

Usage: archive <command> [options]

Options:
  --records <ids>     comma-separated list of record IDs to archive
  --reason <text>     why this record is being archived (required, free-text)
  --dry-run           preview what would be archived without making changes (default: false)
  -h, --help          display help for command

Run 'archive <command> --help' for command-specific help.
```

## audit

```text
audit v0.11.0 — Structured expertise management

Usage: audit <command> [options]

Options:
  --ci                Exit 1 if any signal is in FAIL band; output JSON
  --suggest           Emit specific record IDs to archive/revise/attribute
  --domain <name>     Scope all metrics to one domain
  --ignore-domains <list>  Comma-separated domain names excluded from audit (merged with audit.ignore_domains from config)
  -h, --help          display help for command

Run 'audit <command> --help' for command-specific help.
```

## compact

```text
compact v0.11.0 — Structured expertise management

Usage: compact <command> [options]

Options:
  --analyze           show compaction candidates
  --apply             apply a compaction (replace records with summary)
  --auto              automatically compact all candidates
  --dry-run           preview what --auto would do without writing (use with --auto)
  --min-group <size>  minimum group size for auto-compaction (default: 5) (default: "5")
  --max-records <count>  maximum records to compact in one run (default: 50) (default: "50")
  --yes               skip confirmation prompts (use with --auto)
  --records <ids>     comma-separated record IDs to compact
  --type <type>       record type for the replacement
  --name <name>       name for replacement (pattern/reference/guide)
  --title <title>     title for replacement (decision)
  --description <description>  description for replacement
  --content <content>  content for replacement (convention)
  --resolution <resolution>  resolution for replacement (failure)
  --rationale <rationale>  rationale for replacement (decision)
  -h, --help          display help for command

Run 'compact <command> --help' for command-specific help.
```

## completions

```text
completions v0.11.0 — Structured expertise management

Usage: completions <command> [options]

Options:
  -h, --help          display help for command

Run 'completions <command> --help' for command-specific help.
```

## config

```text
config v0.11.0 — Structured expertise management

Usage: config <command> [options]

Commands:
  schema              Emit AtlasConfig JSON Schema for schema-driven configuration UIs
  show [options]      Emit the effective AtlasConfig as JSON. Pass --path to read a single knob; falls back to the schema default when the knob is unset.
  set <path> <value>  Set a config knob via dot-notation path. <value> is YAML-parsed (so booleans, numbers, lists, and objects all work uniformly). The full resulting config is validated against the schema before atomic write under a file lock; invalid values are rejected with a schema-referenced error. Last-writer-wins semantics under concurrent writes — re-read via `luuna-atlas config show` after every write.
  unset <path>        Remove a config knob via dot-notation path so subsequent reads fall back to the schema default. Empty parent objects along the unset path are pruned when the schema allows the parent to be omitted; required-field removals are rejected with a schema-referenced error (use `luuna-atlas config set` to override). Idempotent — unsetting a never-set path is a silent no-op. Atomic write under a file lock.
  help [command]      display help for command

Options:
  -h, --help          display help for command

Run 'config <command> --help' for command-specific help.
```

## delete

```text
delete v0.11.0 — Structured expertise management

Usage: delete <command> [options]

Options:
  --records <ids>     comma-separated list of record IDs to delete
  --all-except <ids>  delete all records except these comma-separated IDs
  --dry-run           preview what would be deleted without making changes (default: false)
  -h, --help          display help for command

Run 'delete <command> --help' for command-specific help.
```

## delete-domain

```text
delete-domain v0.11.0 — Structured expertise management

Usage: delete-domain <command> [options]

Options:
  --yes               skip confirmation prompt
  --dry-run           preview what would be deleted without making changes (default: false)
  -h, --help          display help for command

Run 'delete-domain <command> --help' for command-specific help.
```

## diff

```text
diff v0.11.0 — Structured expertise management

Usage: diff <command> [options]

Options:
  --since <ref>       git ref to diff against (default: "HEAD~1")
  -h, --help          display help for command

Run 'diff <command> --help' for command-specific help.
```

## doctor

```text
doctor v0.11.0 — Structured expertise management

Usage: doctor <command> [options]

Options:
  --fix               auto-fix fixable issues
  -h, --help          display help for command

Run 'doctor <command> --help' for command-specific help.
```

## edit

```text
edit v0.11.0 — Structured expertise management

Usage: edit <command> [options]

Options:
  --classification <classification>  update classification (choices: "foundational", "tactical", "observational")
  --content <content>  update content (convention)
  --name <name>       update name (pattern)
  --description <description>  update description
  --resolution <resolution>  update resolution (failure)
  --title <title>     update title (decision)
  --rationale <rationale>  update rationale (decision)
  --files <files>     update related files (comma-separated)
  --relates-to <ids>  update linked record IDs (comma-separated)
  --supersedes <ids>  update superseded record IDs (comma-separated)
  --outcome-status <status>  set outcome status (choices: "success", "failure", "partial")
  --outcome-duration <ms>  set outcome duration in milliseconds
  --outcome-test-results <text>  set outcome test results summary
  --outcome-agent <agent>  set outcome agent name
  -h, --help          display help for command

Run 'edit <command> --help' for command-specific help.
```

## hook

```text
hook v0.11.0 — Structured expertise management

Usage: hook <command> [options]

Options:
  -h, --help          display help for command

Run 'hook <command> --help' for command-specific help.
```

## init

```text
init v0.11.0 — Structured expertise management

Usage: init <command> [options]

Options:
  -h, --help          display help for command

Run 'init <command> --help' for command-specific help.
```

## learn

```text
learn v0.11.0 — Structured expertise management

Usage: learn <command> [options]

Options:
  --since <ref>       git ref to diff against (default: "HEAD~1")
  -h, --help          display help for command

Run 'learn <command> --help' for command-specific help.
```

## move

```text
move v0.11.0 — Structured expertise management

Usage: move <command> [options]

Options:
  --dry-run           preview the move without making changes (default: false)
  --force             bypass target domain's allowed_types gate (required_fields still enforced) (default: false)
  -h, --help          display help for command

Run 'move <command> --help' for command-specific help.
```

## onboard

```text
onboard v0.11.0 — Structured expertise management

Usage: onboard <command> [options]

Options:
  --stdout            print snippet to stdout instead of writing to file
  --provider <provider>  customize snippet for a specific provider (e.g. claude)
  --check             check if onboarding snippet is installed and up to date
  -h, --help          display help for command

Run 'onboard <command> --help' for command-specific help.
```

## outcome

```text
outcome v0.11.0 — Structured expertise management

Usage: outcome <command> [options]

Options:
  --status <status>   outcome status (choices: "success", "failure", "partial")
  --duration <ms>     duration in milliseconds
  --agent <name>      agent name
  --notes <text>      notes about the outcome
  --test-results <text>  test results summary
  -h, --help          display help for command

Run 'outcome <command> --help' for command-specific help.
```

## prime

```text
prime v0.11.0 — Structured expertise management

Usage: prime <command> [options]

Options:
  --compact           alias for --format compact (the default: one-line index per record)
  --full              alias for --format markdown (full record bodies instead of the index)
  --manifest          emit a domain index instead of full records (for monolith projects)
  --domain <domains...>  domain(s) to include
  --exclude-domain <domains...>  domain(s) to exclude
  --context           filter records to only those relevant to changed files
  --files <paths...>  filter records to only those relevant to specified files
  --all               opt out of auto-context-scope and emit the full corpus (full mode only; manifest mode is unaffected)
  --export <path>     export output to a file
  --budget <tokens>   token budget for output (default: 4000)
  --no-limit          disable token budget limit
  --records-only      emit only the record lines (no contract, quick reference, or session-close footer); for editor hooks
  --dry-run           emit JSON summary of records that would be primed (id, type, domain, tokens) without rendering content; respects --budget and skips pre-prime hooks
  -h, --help          display help for command

Run 'prime <command> --help' for command-specific help.
```

## prune

```text
prune v0.11.0 — Structured expertise management

Usage: prune <command> [options]

Options:
  --dry-run           Show what would be pruned without removing (default: false)
  --hard              Permanently delete stale records instead of moving them to .luuna/atlas/archive/ (default: false)
  --aggressive        Collapse superseded records straight to archived in one pass instead of one tier at a time (default: false)
  --check-anchors     Demote records whose file/dir anchors no longer resolve (R-05f) (default: false)
  --explain           Print per-record reasons for each demotion (anchor list + decision) (default: false)
  -h, --help          display help for command

Run 'prune <command> --help' for command-specific help.
```

## query

```text
query v0.11.0 — Structured expertise management

Usage: query <command> [options]

Options:
  --type <type>       filter by record type
  --classification <classification>  filter by classification (choices: "foundational", "tactical", "observational")
  --file <file>       filter by associated file path (substring match)
  --outcome-status <status>  filter by outcome status (choices: "success", "failure")
  --sort-by-score     sort results by confirmation-frequency score (highest first)
  --all               show all domains
  --format <format>   output format for records (choices: "markdown", "compact", "xml", "plain", "ids")
  -h, --help          display help for command

Run 'query <command> --help' for command-specific help.
```

## rank

```text
rank v0.11.0 — Structured expertise management

Usage: rank <command> [options]

Options:
  --type <type>       filter by record type (choices: "convention", "pattern", "failure", "decision", "reference", "guide")
  --limit <n>         return at most N records (default: "10")
  --min-score <n>     exclude records whose confirmation score is below N (default: 0, i.e. include all) (default: "0")
  -h, --help          display help for command

Run 'rank <command> --help' for command-specific help.
```

## ready

```text
ready v0.11.0 — Structured expertise management

Usage: ready <command> [options]

Options:
  --limit <n>         maximum number of records to show (default: "10")
  --domain <domain>   limit to a specific domain
  --since <duration>  show records from the last duration (e.g. 24h, 7d, 2w)
  -h, --help          display help for command

Run 'ready <command> --help' for command-specific help.
```

## record

```text
record v0.11.0 — Structured expertise management

Usage: record <command> [options]

Options:
  --type <type>       record type (choices: "convention", "pattern", "failure", "decision", "reference", "guide")
  --classification <classification>  classification level (choices: "foundational", "tactical", "observational", default: "tactical")
  --name <name>       name of the convention or pattern
  --content <content>  content for convention records (explicit flag wins over positional [content])
  --description <description>  description of the record
  --resolution <resolution>  resolution for failure records
  --title <title>     title for decision records
  --rationale <rationale>  rationale for decision records
  --files <files>     related files (comma-separated)
  --dir-anchor <path>  repo-relative directory the record applies to (repeatable; auto-populated from changed files when omitted) (default: [])
  --tags <tags>       comma-separated tags
  --evidence-commit <commit>  evidence: commit hash (auto-populated from git if omitted)
  --evidence-issue <issue>  evidence: issue reference
  --evidence-file <file>  evidence: file path
  --evidence-bead <bead>  evidence: bead ID
  --evidence-missions <id>  evidence: missions issue ID
  --evidence-gh <ref>  evidence: GitHub issue or PR reference
  --evidence-linear <ticket>  evidence: Linear ticket reference
  --relates-to <ids>  comma-separated record IDs this relates to
  --supersedes <ids>  comma-separated record IDs this supersedes
  --outcome-status <status>  outcome status (choices: "success", "failure", "partial")
  --outcome-duration <ms>  outcome duration in milliseconds
  --outcome-test-results <text>  outcome test results summary
  --outcome-agent <agent>  outcome agent name
  --force             force recording even if duplicate exists
  --stdin             read JSON record(s) from stdin (single object or array)
  --batch <file>      read JSON record(s) from file (single object or array)
  --dry-run           preview what would be recorded without writing
  -h, --help          display help for command

Run 'record <command> --help' for command-specific help.

Required fields per record type:
  convention   --content (or positional [content])
  pattern      --name, --description (or [content])
  failure      --description, --resolution
  decision     --title, --rationale
  reference    --name, --description (or [content])
  guide        --name, --description (or [content])

--description alone works for every type except failure (needs --resolution):
it fills convention content, a derived pattern/reference/guide name, and a
decision title (first sentence) + rationale. Explicit flags win.

Batch recording examples:
  luuna-atlas record cli --batch records.json
  luuna-atlas record cli --batch records.json --dry-run
  echo '[{"type":"convention","content":"test"}]' > batch.json && luuna-atlas record cli --batch batch.json

```

## restore

```text
restore v0.11.0 — Structured expertise management

Usage: restore <command> [options]

Options:
  -h, --help          display help for command

Run 'restore <command> --help' for command-specific help.
```

## search

```text
search v0.11.0 — Structured expertise management

Usage: search <command> [options]

Options:
  --domain <domain>   limit search to a specific domain
  --type <type>       filter by record type (choices: "convention", "pattern", "failure", "decision", "reference", "guide")
  --tag <tag>         filter by tag
  --classification <classification>  filter by classification (choices: "foundational", "tactical", "observational")
  --file <file>       filter by associated file path (substring match)
  --outcome-status <status>  filter by outcome status (choices: "success", "failure", "partial")
  --sort-by-score     sort results by confirmation-frequency score (highest first)
  --no-boost          disable confirmation-frequency boost on BM25 ranking (pure BM25)
  --archived          include soft-archived records from .luuna/atlas/archive/ (excluded by default)
  --format <format>   output format for records (choices: "markdown", "compact", "xml", "plain", "ids")
  -h, --help          display help for command

Run 'search <command> --help' for command-specific help.
```

## setup

```text
setup v0.11.0 — Structured expertise management

Usage: setup <command> [options]

Options:
  --check             verify provider integration is installed
  --remove            remove provider integration
  --hooks             install a pre-commit git hook running luuna-atlas validate
  --list              list discovered providers (built-in, .luuna/atlas/recipes/, luuna-atlas-recipe-* npm)
  -h, --help          display help for command

Run 'setup <command> --help' for command-specific help.
```

## show

```text
show v0.11.0 — Structured expertise management

Usage: show <command> [options]

Options:
  -h, --help          display help for command

Run 'show <command> --help' for command-specific help.
```

## stale

```text
stale v0.11.0 — Structured expertise management

Usage: stale <command> [options]

Options:
  -h, --help          display help for command

Run 'stale <command> --help' for command-specific help.
```

## status

```text
status v0.11.0 — Structured expertise management

Usage: status <command> [options]

Options:
  -h, --help          display help for command

Run 'status <command> --help' for command-specific help.
```

## sync

```text
sync v0.11.0 — Structured expertise management

Usage: sync <command> [options]

Options:
  --message <message>  custom commit message
  --no-validate       skip validation step
  -h, --help          display help for command

Run 'sync <command> --help' for command-specific help.
```

## update

```text
update v0.11.0 — Structured expertise management

Usage: update <command> [options]

Options:
  --check             only check for updates, do not install
  -h, --help          display help for command

Run 'update <command> --help' for command-specific help.
```

## upgrade

```text
upgrade v0.11.0 — Structured expertise management

Usage: upgrade <command> [options]

Options:
  --check             Check for updates without installing
  -h, --help          display help for command

Run 'upgrade <command> --help' for command-specific help.
```

## validate

```text
validate v0.11.0 — Structured expertise management

Usage: validate <command> [options]

Options:
  -h, --help          display help for command

Run 'validate <command> --help' for command-specific help.
```

## config schema

```text
schema v0.11.0 — Structured expertise management

Usage: schema <command> [options]

Options:
  -h, --help          display help for command

Run 'schema <command> --help' for command-specific help.
```

## config show

```text
show v0.11.0 — Structured expertise management

Usage: show <command> [options]

Options:
  --path <path>       Dot-notation path to a single knob (e.g. governance.max_entries, search.boost_factor)
  -h, --help          display help for command

Run 'show <command> --help' for command-specific help.
```

## config set

```text
set v0.11.0 — Structured expertise management

Usage: set <command> [options]

Options:
  -h, --help          display help for command

Run 'set <command> --help' for command-specific help.
```

## config unset

```text
unset v0.11.0 — Structured expertise management

Usage: unset <command> [options]

Options:
  -h, --help          display help for command

Run 'unset <command> --help' for command-specific help.
```
