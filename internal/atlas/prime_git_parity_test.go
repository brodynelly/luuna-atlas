package atlas

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func parityPrimeFixture(t *testing.T, dir string) {
	t.Helper()
	if _, _, err := InitStore(dir); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg["domains"] = map[string]any{"zeta": map[string]any{}, "alpha": map[string]any{}}
	rememberOrder(Obj(cfg["domains"]), []string{"zeta", "alpha"})
	cfg["prime"] = map[string]any{"session_close": map[string]any{"style": "minimal"}}
	if err := WriteConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRegistry(cfg); err != nil {
		t.Fatal(err)
	}
	records := []Record{
		{"type": "convention", "content": "Avoid global state because concurrent workers need isolation 🐙", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"type": "pattern", "name": "Anchored pattern", "description": "Always keep anchored work close to implementation", "files": []string{"src/a.go"}, "classification": "tactical", "recorded_at": "2025-01-02T00:00:00.000Z", "outcomes": []any{map[string]any{"status": "success"}}},
		{"type": "failure", "description": "Shared writes can overwrite complete files", "resolution": "Use advisory locks and atomic writes", "dir_anchors": []string{"src"}, "classification": "foundational", "recorded_at": "2025-01-03T00:00:00.000Z"},
		{"type": "reference", "name": "Removed anchor", "description": "Reference material tied to a missing file", "files": []string{"missing.go"}, "classification": "observational", "recorded_at": "2025-01-04T00:00:00.000Z"},
	}
	path, _ := ExpertisePath(dir, "zeta")
	if err := WriteRecords(path, records); err != nil {
		t.Fatal(err)
	}
	path, _ = ExpertisePath(dir, "alpha")
	if err := WriteRecords(path, []Record{}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("initial\n"), 0666); err != nil {
		t.Fatal(err)
	}
}

func TestParityPrimeFormatsBudgetAndExport(t *testing.T) {
	steps := []parityStep{}
	for _, format := range []string{"markdown", "compact", "xml", "plain"} {
		steps = append(steps, parityStep{Args: []string{"prime", "--all", "--format", format}}, parityStep{Args: []string{"prime", "--all", "--records-only", "--format", format}}, parityStep{Args: []string{"prime", "--manifest", "--format", format}})
	}
	for _, args := range [][]string{
		{"prime", "--all", "--json"}, {"prime", "--manifest", "--json"},
		{"prime", "--all", "--dry-run", "--budget", "45"}, {"prime", "--all", "--dry-run", "--no-limit"},
		{"prime", "--files", "src/a.go", "--budget", "1", "--format", "plain"},
		{"prime", "--files", "src/a.go", "--budget", "1", "--dry-run"},
		{"prime", "--files", "unrelated.go", "--json"}, {"prime", "--files", "unrelated.go", "--records-only"},
		{"prime", "--all", "--no-limit", "--full"},
		{"prime", "--all", "--format", "plain", "--export", "primed.txt"},
		{"prime", "--dry-run", "--export", "preview.json"},
	} {
		steps = append(steps, parityStep{Args: args})
	}
	parityExecute(t, steps, func(dir string) { parityPrimeFixture(t, dir) })
}

func parityGitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "core.autocrlf", "false"}, {"config", "user.name", "Atlas Test"}, {"config", "user.email", "atlas-test@example.invalid"}} {
		if _, err := Git(dir, args...); err != nil {
			t.Fatal(err)
		}
	}
}
func parityGitEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_DATE", "2025-01-10T00:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2025-01-10T00:00:00Z")
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "0")
}
func TestParityGitLearnDiffSyncAndContext(t *testing.T) {
	parityGitEnvironment(t)
	steps := []parityStep{}
	for _, args := range [][]string{
		{"learn", "--since", "HEAD", "--json"}, {"learn", "--since", "HEAD"},
		{"diff", "--since", "HEAD", "--json"}, {"diff", "--since", "HEAD"},
		{"diff", "--since", "missing-ref", "--json"},
		{"prime", "--context", "--format", "plain"}, {"prime", "--format", "plain"},
		{"prime", "--all", "--format", "plain"}, {"prime", "--files", "src/a.go", "--json"},
		{"sync", "--message", "fixture sync", "--json"}, {"sync", "--json"},
		{"learn", "--since", "HEAD", "--json"}, {"diff", "--since", "HEAD~1", "--json"},
		{"diff", "--since", "HEAD~1", "--quiet"},
	} {
		steps = append(steps, parityStep{Args: args})
	}
	parityExecute(t, steps, func(dir string) {
		parityPrimeFixture(t, dir)
		parityGitInit(t, dir)
		if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("before\n"), 0666); err != nil {
			t.Fatal(err)
		}
		commitFixture(t, dir, "base")
		_ = os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("changed\n"), 0666)
		_ = os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("staged\n"), 0666)
		_, _ = Git(dir, "add", "tracked.txt")
		_ = os.WriteFile(filepath.Join(dir, "unmatched.txt"), []byte("untracked\n"), 0666)
		path, _ := ExpertisePath(dir, "zeta")
		if err := AppendRecord(path, Record{"type": "guide", "name": "Additional working guidance", "description": "A new record to diff and synchronize", "classification": "foundational", "recorded_at": "2025-01-05T00:00:00.000Z"}, GetRegistry()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestParityPrimeActiveWorkAndHooks(t *testing.T) {
	parityGitEnvironment(t)
	parityHookEnvironment(t)
	steps := []parityStep{{Args: []string{"prime", "--format", "plain"}}, {Args: []string{"prime", "--all", "--json"}}, {Args: []string{"prime", "--all", "--format", "markdown"}}, {Args: []string{"prime", "--all", "--dry-run"}}}
	parityExecute(t, steps, func(dir string) {
		parityPrimeFixture(t, dir)
		parityGitInit(t, dir)
		path, _ := ExpertisePath(dir, "alpha")
		if err := WriteRecords(path, []Record{{"type": "decision", "title": "Active mission", "rationale": "This applies to the active work", "classification": "foundational", "recorded_at": "2025-01-05T00:00:00.000Z", "evidence": map[string]any{"missions": "atlas-abcd"}}, {"type": "decision", "title": "Unrelated mission", "rationale": "This applies to unrelated work", "classification": "foundational", "recorded_at": "2025-01-05T00:00:00.000Z", "evidence": map[string]any{"missions": "atlas-dcba"}}}); err != nil {
			t.Fatal(err)
		}
		_ = os.MkdirAll(filepath.Join(dir, ".luuna", "missions"), 0755)
		_ = os.WriteFile(filepath.Join(dir, ".luuna", "missions", "issues.jsonl"), []byte("{\"id\":\"atlas-abcd\",\"status\":\"in_progress\"}\n"), 0666)
		hook := `const p = (await Bun.stdin.json()).payload; for (const d of p.domains) { d.records = d.records.filter(r => r.type !== "reference"); } console.log(JSON.stringify(p));`
		_ = os.WriteFile(filepath.Join(dir, "mutate-prime.ts"), []byte(hook), 0666)
		cfg, err := ReadConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		cfg["hooks"] = map[string]any{"pre-prime": []string{"bun mutate-prime.ts"}}
		if err = WriteConfig(dir, cfg); err != nil {
			t.Fatal(err)
		}
		commitFixture(t, dir, "base")
	})
}

func TestParitySyncWorktreeSharesMainStore(t *testing.T) {
	parityGitEnvironment(t)
	reference := parityReference(t)
	refDir, goDir := parityProjects(t)
	roots := []string{refDir, goDir}
	works := []string{}
	for _, dir := range roots {
		parityPrimeFixture(t, dir)
		parityGitInit(t, dir)
		commitFixture(t, dir, "base")
		work := filepath.Join(filepath.Dir(dir), "linked")
		if _, err := Git(dir, "worktree", "add", "-b", "worker", work); err != nil {
			t.Fatal(err)
		}
		works = append(works, work)
		t.Cleanup(func() { _, _ = Git(dir, "worktree", "remove", "--force", work) })
	}
	started := time.Now()
	for _, args := range [][]string{{"sync", "--json"}, {"sync"}, {"prime", "--all", "--json"}} {
		want := parityRun(t, reference, works[0], parityStep{Args: args}, false)
		got := parityRun(t, reference, works[1], parityStep{Args: args}, true)
		parityCompare(t, fmt.Sprint(args), want, got, works[0], works[1], started)
	}
	if !reflect.DeepEqual(parityTree(t, refDir, started), parityTree(t, goDir, started)) {
		t.Fatal("worktree command changed main store inconsistently")
	}
	for _, dir := range roots {
		status, err := Git(dir, "status", "--porcelain")
		if err != nil || strings.TrimSpace(status) != "" {
			t.Fatalf("unexpected main store mutation: %s %v", status, err)
		}
	}
}

func TestParitySyncStrictGateAndGitFailures(t *testing.T) {
	parityGitEnvironment(t)
	for _, variant := range []string{"unknown-type", "domain-type", "required-field", "invalid-json", "empty-message"} {
		t.Run(variant, func(t *testing.T) {
			steps := []parityStep{{Args: []string{"sync", "--json", "--allow-unknown-types", "--allow-domain-mismatch"}}, {Args: []string{"sync"}}, {Args: []string{"sync", "--no-validate", "--json"}}}
			if variant == "empty-message" {
				steps = []parityStep{{Args: []string{"sync", "--message", "", "--json"}}, {Args: []string{"sync", "--message", ""}}}
			}
			parityExecute(t, steps, func(dir string) {
				parityPrimeFixture(t, dir)
				parityGitInit(t, dir)
				commitFixture(t, dir, "base")
				cfg, err := ReadConfig(dir)
				if err != nil {
					t.Fatal(err)
				}
				path, _ := ExpertisePath(dir, "zeta")
				switch variant {
				case "unknown-type":
					_ = os.WriteFile(path, []byte("{\"id\":\"mx-aabbcc\",\"type\":\"missing\"}\n"), 0666)
				case "domain-type":
					cfg["domains"] = map[string]any{"zeta": map[string]any{"allowed_types": []string{"pattern"}}}
					_ = WriteConfig(dir, cfg)
				case "required-field":
					cfg["domains"] = map[string]any{"zeta": map[string]any{"required_fields": []string{"owner"}}}
					_ = WriteConfig(dir, cfg)
				case "invalid-json":
					_ = os.WriteFile(path, []byte("{malformed json}\n"), 0666)
				case "empty-message":
					r := Record{"type": "guide", "name": "Changed", "description": "Changed", "classification": "foundational", "recorded_at": "2025-01-05T00:00:00.000Z"}
					if err = AppendRecord(path, r, GetRegistry()); err != nil {
						t.Fatal(err)
					}
				}
			})
		})
	}
}

func TestParityGitCommandsOutsideRepository(t *testing.T) {
	steps := []parityStep{}
	for _, command := range []string{"sync", "learn", "diff"} {
		steps = append(steps, parityStep{Args: []string{command}}, parityStep{Args: []string{command, "--json"}})
	}
	parityExecute(t, steps, nil)
}

func parityHookEnvironment(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		git, err := exec.LookPath("git")
		if err != nil {
			t.Fatal(err)
		}
		shellDir := filepath.Join(filepath.Dir(filepath.Dir(git)), "bin")
		if !exists(filepath.Join(shellDir, "sh.exe")) {
			t.Skip("a POSIX shell is needed to exercise lifecycle hook execution")
		}
		t.Setenv("PATH", shellDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	t.Setenv("LUUNA_ATLAS_LOG_LEVEL", "silent")
}

func TestParityPrimeHookMutationBoundary(t *testing.T) {
	parityHookEnvironment(t)
	for _, script := range []string{
		`console.log(JSON.stringify({domains:[{domain:"zeta",records:null},{domain:"unasked",records:[]}]}));`,
		`console.log("{bad");`,
		`console.log(JSON.stringify({domains:[{domain:"zeta",records:[]}]}));`,
		`process.exit(3);`,
	} {
		t.Run(fmt.Sprint(script), func(t *testing.T) {
			parityExecute(t, []parityStep{{Args: []string{"prime", "--all", "--format", "plain"}}, {Args: []string{"prime", "--all", "--json"}}, {Args: []string{"prime", "--all", "--dry-run"}}}, func(dir string) {
				parityPrimeFixture(t, dir)
				if err := os.WriteFile(filepath.Join(dir, "prime-hook.ts"), []byte(script), 0666); err != nil {
					t.Fatal(err)
				}
				cfg, err := ReadConfig(dir)
				if err != nil {
					t.Fatal(err)
				}
				cfg["hooks"] = map[string]any{"pre-prime": []string{"bun prime-hook.ts"}}
				if err = WriteConfig(dir, cfg); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestParityValidationArbitraryJSONValues(t *testing.T) {
	parityGitEnvironment(t)
	parityExecute(t, []parityStep{{Args: []string{"validate", "--json"}}, {Args: []string{"validate"}}, {Args: []string{"sync", "--json"}}, {Args: []string{"sync"}}}, func(dir string) {
		parityPrimeFixture(t, dir)
		parityGitInit(t, dir)
		commitFixture(t, dir, "base")
		path, _ := ExpertisePath(dir, "zeta")
		if err := os.WriteFile(path, []byte("null\ntrue\n123\n\"text\"\n[]\n[{}]\n{}\n"), 0666); err != nil {
			t.Fatal(err)
		}
	})
}

func TestParityCanonicalNumericDomainEnumeration(t *testing.T) {
	parityExecute(t, []parityStep{{Args: []string{"config", "show"}}, {Args: []string{"prime", "--manifest", "--json"}}, {Args: []string{"status", "--json"}}, {Args: []string{"config", "set", "search.boost_factor", "0.2"}}, {Args: []string{"config", "show"}}}, func(dir string) {
		parityPrimeFixture(t, dir)
		path, _ := ConfigPath(dir)
		text := "version: '1'\ndomains:\n  '10': {}\n  '2': {}\n  zebra: {}\n"
		if err := os.WriteFile(path, []byte(text), 0666); err != nil {
			t.Fatal(err)
		}
	})
}

func TestParityQueryPreservesArrayRecordRows(t *testing.T) {
	parityExecute(t, []parityStep{{Args: []string{"query", "zeta", "--json"}}, {Args: []string{"query", "--all", "--json"}}}, func(dir string) {
		parityPrimeFixture(t, dir)
		path, _ := ExpertisePath(dir, "zeta")
		if err := os.WriteFile(path, []byte("[]\n[{\"nested\":true}]\n"), 0666); err != nil {
			t.Fatal(err)
		}
	})
}

func TestParityYAMLMergeAndDuplicateKeys(t *testing.T) {
	for _, body := range []string{
		"version: '1'\ndomains: {}\nbase: &base\n  max_entries: 42\n  warn_entries: 50\ngovernance:\n  <<: *base\n  hard_limit: 80\n",
		"version: '1'\ndomains: {}\none: &one {max_entries: 42, warn_entries: 50}\ntwo: &two {max_entries: 24, hard_limit: 80}\ngovernance:\n  hard_limit: 81\n  <<: [*one, *two]\n  warn_entries: 51\n",
		"version: '1'\ndomains: {}\ngovernance:\n  max_entries: 42\n  max_entries: 55\n",
	} {
		t.Run(body, func(t *testing.T) {
			parityExecute(t, []parityStep{{Args: []string{"config", "show"}}, {Args: []string{"status", "--json"}}}, func(dir string) {
				parityPrimeFixture(t, dir)
				path, _ := ConfigPath(dir)
				if err := os.WriteFile(path, []byte(body), 0666); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}
