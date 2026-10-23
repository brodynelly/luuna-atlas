# Patch: canonical parity project roots (test harness only)

## Problem

`bun run verify` fails on macOS under the default `TMPDIR` (`/var/folders/...`) when it runs with the local Go 1.27.0 SDK and Bun 1.4.3. The cause is in the parity harness, not in the product:

- `internal/atlas/parity_test.go:192-202` `parityProjects` builds `refDir`/`goDir` from `t.TempDir()`. On macOS that path starts with `/var/...`, and `/var` is a symlink to `/private/var`.
- The native run (`parityRun`, `native=true`) passes this path unchanged as `App.Cwd`, so Go output contains `/var/...`.
- The reference run sets `cmd.Dir` to the same path. Bun reports `process.cwd()` as the resolved path, so reference output contains `/private/var/...`.
- `parityNormalize` (lines 81-83) replaces only the literal `dir` string with `<PROJECT>`. The `/private/var/...` strings in reference stdout, stderr and the persisted tree are not replaced, so the comparisons in `parityCompare`/`parityExecute` fail.

Setting `TMPDIR` to its realpath makes verify pass, but it hides the defect and is **not** an acceptable fix. Verify must pass under the default `TMPDIR` with no override.

## Scope and constraints

- Change only `internal/atlas/parity_test.go`, which is a test file.
- Do not change CLI path semantics in `internal/atlas` non-test code, `cmd/luuna-atlas/`, or `src/`.
- Do not modify `compat/reference/` (the frozen oracle), its outputs, `contract` fixtures, or expected JSON/schema contracts.
- Do not change the normalization rules: add no `/private` stripping and no prefix aliasing. The fix gives both runs the same canonical input path.
- Do not change toolchain pins, `TMPDIR` handling in scripts, or CI config.
- No commit, push or release.

## Design

Canonicalize the temporary root once, before either project directory is derived. Both the Go and reference invocations, and the `dir` argument passed to `parityNormalize`, `parityOutput` and `parityTree`, then use the same `/private/var/...` path. This works because the same `refDir`/`goDir` values flow through `parityExecute` and `TestParityHelpAndParser`.

```go
func parityCanonicalRoot(t *testing.T, root string) string {
	t.Helper()
	canonical, e := filepath.EvalSymlinks(root)
	if e != nil {
		t.Fatal(e)
	}
	return canonical
}

func parityProjects(t *testing.T) (string, string) {
	t.Helper()
	root := parityCanonicalRoot(t, t.TempDir())
	// unchanged below
}
```

`EvalSymlinks` returns a cleaned absolute path when its input is absolute, as `t.TempDir()` is. On Linux, where there is no symlink, the result is the same path, so behaviour there does not change. Windows short-name expansion is also harmless here.

## Step by Step Tasks

1. **Reproduce first, with no code change.** Use the default `TMPDIR` (confirm `echo "$TMPDIR"` starts with `/var/folders/`, and that `TMPDIR` is not exported in the current shell's rc overrides). Run:
   `GOTOOLCHAIN=local go test -mod=vendor -run 'TestParity' ./internal/atlas/`
   Record the failing mismatch, which shows `/private/var/...` in the REFERENCE output and `/var/...` in GO.
2. **Add the `parityCanonicalRoot` helper** to `internal/atlas/parity_test.go`, next to `parityProjects`. It calls `filepath.EvalSymlinks` and calls `t.Fatal(e)` on error.
3. **Use it in `parityProjects`:** `root := parityCanonicalRoot(t, t.TempDir())`. Leave the rest of the function, `parityRun`, `parityNormalize`, `parityOutput`, `parityTree`, `parityCompare` and every step list unchanged.
4. **Add a focused regression test** in the same file, `TestParityProjectsCanonicalRoot`:
   - Under `base := t.TempDir()`, create `real := filepath.Join(base, "real")` (`os.MkdirAll`) and `link := filepath.Join(base, "link")` with `os.Symlink(real, link)`. If `os.Symlink` fails, call `t.Skip` because symlinks are unsupported, for example on Windows without privilege.
   - Assert that `parityCanonicalRoot(t, link)` equals `filepath.EvalSymlinks(real)`, which covers the macOS `/var` to `/private/var` case as well, and that it does not equal `link`.
   - Assert that the harness yields canonical directories. Call `parityProjects(t)`, then for each returned dir assert `filepath.EvalSymlinks(dir) == dir`.
   - Assert normalization round-trips the canonical path. `parityNormalize(filepath.Join(canon, "x"), canon, time.Now())` must equal `"<PROJECT>" + string(filepath.Separator) + "x"`. On Windows, use `filepath.Join("<PROJECT>", "x")` to avoid separator assumptions.
   - The test must not require Bun. It does not call `parityReference`.
5. **Format and vet:** `gofmt -l internal/atlas/parity_test.go` must print nothing, and `GOTOOLCHAIN=local go vet -mod=vendor ./internal/atlas/` must pass.
6. **Run the Validation Commands below** under the default `TMPDIR`. Do not export or prefix `TMPDIR` anywhere.
7. **Confirm the diff is limited to the test file:** `git diff --stat` must show only `internal/atlas/parity_test.go`, apart from the pre-existing uncommitted release work. `git diff -- compat/ src/ cmd/ internal/atlas/*[!_test].go` must be empty with respect to this patch.

## Validation Commands

Run all commands from the repository root in a fresh shell. Do **not** set, prefix or export `TMPDIR`. Use the already-installed local Go 1.27.0 SDK and the local Bun 1.4.3, and download nothing. Put the existing SDK's `bin` first on `PATH`. Adjust the path below to the location where 1.27.0 is already installed, for example `$HOME/sdk/go1.27.0`.

```sh
# Toolchain and environment preconditions
export PATH="$HOME/sdk/go1.27.0/bin:$PATH"   # existing local SDK; no download
export GOTOOLCHAIN=local
test "$(go env GOVERSION)" = go1.27.0
test "$(bun --version)" = 1.4.3
case "$TMPDIR" in /var/folders/*) : ;; *) echo "TMPDIR not default: $TMPDIR" >&2; exit 1 ;; esac

# Focused: harness regression + all parity suites against the Bun oracle
go test -mod=vendor -count=1 -run 'TestParity' -v ./internal/atlas/

# Full Go suite
go test -mod=vendor -count=1 ./...

# Full supported component verification under default TMPDIR (no override)
bun run verify

# Release script tests (expect all 72 to pass, 0 failures)
sh scripts/release/test-release.sh

# Native build check
bun run build:native
out=$(mktemp -d)
sh scripts/release/package.sh --version 0.0.0-local --out "$out"
sh scripts/release/check-archive.sh --require-exec "$out"/luuna-atlas_0.0.0-local_*.tar.gz

# Scope check
git diff --stat -- internal/atlas/parity_test.go compat/ src/ cmd/
```

Pass criteria:
- Every command exits 0.
- `TestParityProjectsCanonicalRoot` runs and passes on macOS; it is not skipped.
- `test-release.sh` reports 72 passed and 0 failed.
- The scope check shows changes only in `internal/atlas/parity_test.go`.

If any command passes only with a `TMPDIR` override, treat that as a failure of this patch.
