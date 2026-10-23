# Go rewrite compatibility plan

Baseline: Atlas commit 27477c313a3049b1579dfdc32c39da600726721f, version 0.11.0. User requires an entire Go CLI rewrite with unchanged commands, functionality, interactions, record/config formats, aliases and integrations. Existing local mapping documentation is preserved. No commits, pushes, publication, issues, component-pin changes or live-store changes are authorized.

The TypeScript runtime remains the reference until the Go replacement passes differential checks. No runtime delegation from Go back to the legacy implementation is permitted. A small entry-point compatibility launcher may invoke the native Go executable for existing source/Bun consumers. Historical source may be retained solely as a test oracle, excluded from production execution.

## Work ownership

| Area | Owner | Files |
|---|---|---|
| Data/config/types/storage/search/Git/retention algorithms | core agent | internal/atlas/model.go, config.go, store.go, registry.go, schema.go, search.go, git.go, retention.go and corresponding tests |
| Formatters/provider setup/onboard/recipe/lifecycle hooks/hook handler/completions; native development tooling, CI and current documentation | integrations agent | internal/atlas/format*.go, setup*.go, hooks*.go, onboard.go, completions.go and tests; scripts/go-tool.ts, verify-native.ts, build-native.ts, version-bump.ts; package.json, .github/workflows/ci.yml, AGENTS.md, README.md, CLAUDE.md |
| Write commands and lifecycle maintenance | commands agent | internal/atlas/record.go, edit.go, outcome.go, move.go, archive_commands.go, delete.go, prune.go, compact.go and tests |
| CLI/parser/read/health/config commands, compatibility transport and contract tests | parent | app.go, cli.go, read_commands.go, prime.go, health.go, audit.go, config_commands.go, git_commands.go, cmd/, src/, scripts/contract tooling |

Agents share the checkout with disjoint file ownership. No worktree/branch/commit operations; this adapts the orchestration workflow to the explicit no-commit instruction and the existing uncommitted documentation. The user authorized unattended completion, so no build checkpoint is required.

## Shared Go contract

All implementation lives in package `internal/atlas` initially; executable at `cmd/luuna-atlas`. Data objects use `type Record map[string]any` and `type Config map[string]any` to preserve unknown/optional data without zero-value mutation. Core agent owns these types, `TypeDefinition`, `Registry`, and common conversion helpers.

Core functions (coordinate any changes before using):

```go
func AtlasDir(cwd string) (string, error)
func ExpertisePath(cwd, domain string) (string, error)
func ReadConfig(cwd string) (Config, error)
func WriteConfig(cwd string, cfg Config) error
func InitStore(cwd string) (created bool, path string, err error)
func AddDomain(cwd, domain string) error
func NewRegistry(cfg Config) (*Registry, error)
func ReadRecords(path string, registry *Registry, allowUnknown bool) ([]Record, error)
func WriteRecords(path string, records []Record) error
func AppendRecord(path string, record Record, registry *Registry) error
func WithFileLock(path string, fn func() error) error
func GenerateRecordID(record Record, registry *Registry) string
func FindDuplicate(records []Record, candidate Record, registry *Registry) int
func ResolveRecordID(records []Record, id string) (int, error)
func ValidateRecord(record Record, registry *Registry) []ValidationIssue
```

Helpers: `Text(any) string`, `Obj(any) map[string]any`, `List(any) []string`, `Number(any) float64`, `Bool(any) bool`, `NowISO() string`, `CloneRecord(Record) Record`, `SplitCSV(string) []string`. Core registry: `Types map[string]*TypeDefinition`, `Order []string`; definition fields `Name`, `Required`, `Optional`, `DedupKey`, `IDKey`, `Summary`, `SectionTitle`, `Compact`, `FilesField`, `ExtractsFiles`, `Aliases`, `Disabled`, `Builtin`. `ValidationIssue` has `Path` and `Message` strings.

Parent owns:

```go
type Invocation struct { Positionals []string; Flags map[string][]string }
func (i Invocation) String(name string) string
func (i Invocation) Strings(name string) []string
func (i Invocation) Bool(name string) bool
func (i Invocation) Has(name string) bool
type App struct {
    Cwd string
    In io.Reader
    Out, Err io.Writer
    JSONMode, Quiet, Verbose, AllowUnknownTypes, AllowDomainMismatch bool
    Format string
    Registry *Registry
    ExitCode int
    Command string
}
```

Handlers: `func (a *App) runRecord(i Invocation) error`, equivalently runEdit/runOutcome/runMove/runArchive/runRestore/runDelete/runDeleteDomain/runPrune/runCompact/runSetup/runOnboard/runHook/runCompletions. Return errors for shared rendering, or call `a.Error(command, message)` to render/set ExitCode and return nil. `a.Emit(any)` writes pretty JSON; `a.OK(command string, fields map[string]any)` adds standard envelope; `a.Human(string)` honors quiet. Do not invoke Bun/TypeScript for business logic.

Formatting agent should expose source-equivalent Go helpers named `FormatRecordIndex`, `FormatDomain`, `FormatPrime`, `FormatStatus`, `RecordSummary`, with signatures communicated to parent before implementation consumers are written. Lifecycle function contract: `RunHooks(cwd string, cfg Config, event string, payload any, stderr io.Writer) (HookResult,error)`, HookResult fields Payload any, Blocked bool, BlockReason string, Warnings []string, RanAny bool.

## Completion gates

- All 33 commands and four config children implemented in Go; all flags/positionals/defaults/choices/help/exit behavior retained.
- Differential CLI cases compare reference vs Go outputs, errors, persisted record/config state, hooks and Git effects using disposable projects. Existing quirks are compatibility behavior, not opportunities for unrelated fixes.
- Core Go tests cover real filesystem concurrency/atomicity, records/schema/custom types, search/ranking/decay and migrations. Race detection and native build where toolchain supports them.
- Existing Bun public-boundary tests continue to exercise compatibility entry points; pure reference tests are clearly distinguished from Go evidence.
- Native executable works without Bun/Node/legacy source. Existing aliases and source entry point continue to invoke native Go.
- Go build/vet/tests and supported legacy validation pass, with any platform skips stated. Cross-compile other supported targets, without claiming execution on untested OSes.
- Current README/development/CI guidance reflects Go runtime; provenance/MIT and private/unpublished release policy preserved.

## Current state

Completed. All production command, library, data and integration behavior has been ported. The former source is frozen as a test oracle; strict TypeScript entry points are transports to Go. Independent review findings for Unicode lowercasing, YAML merges/duplicates and non-finite API values have regression coverage and are resolved. Six-target final cross-builds and isolated Windows/Linux execution passed. The supported `bun run verify` exited 0: Go passed 627 cases and Bun passed 1,605 tests with 3 platform skips and no failures. Final evidence and platform boundaries are recorded in [GO-REWRITE-REPORT.md](GO-REWRITE-REPORT.md). No commit, push or consumer-pin change was made.

The user's subsequent request to push everything authorizes a Git commit and push of all completed source, tests, vendored dependencies and documentation in this repository. Delivery uses `feat/go-cli-rewrite`; generated binaries/local run data stay ignored. Package publication, merging and sibling consumer-pin changes are outside this shipping request.
