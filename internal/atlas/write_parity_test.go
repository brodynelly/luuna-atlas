package atlas

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func parityWritePrepare(t *testing.T, rows []Record, extra func(string)) func(string) {
	return func(dir string) {
		if _, _, e := InitStore(dir); e != nil {
			t.Fatal(e)
		}
		for _, d := range []string{"knowledge", "target", "links"} {
			if e := AddDomain(dir, d); e != nil {
				t.Fatal(e)
			}
		}
		cfg, e := ReadConfig(dir)
		if e != nil {
			t.Fatal(e)
		}
		cfg["quality"] = map[string]any{"level": "off"}
		if e = WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
		if _, e = NewRegistry(cfg); e != nil {
			t.Fatal(e)
		}
		copy := []Record{}
		for _, r := range rows {
			copy = append(copy, CloneRecord(r))
		}
		path, _ := ExpertisePath(dir, "knowledge")
		if e = WriteRecords(path, copy); e != nil {
			t.Fatal(e)
		}
		if extra != nil {
			extra(dir)
		}
	}
}
func TestParityWriteLifecycle(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	r := Record{"type": "decision", "title": "Atomic writes", "rationale": "A complete snapshot", "classification": "foundational", "recorded_at": "2026-01-01T00:00:00.000Z", "outcomes": []Record{{"status": "success"}}}
	reg := BuiltinRegistry()
	id := GenerateRecordID(r, reg)
	r["id"] = id
	f := Record{"type": "failure", "description": "Shared fixture", "resolution": "Unique directory", "classification": "foundational", "recorded_at": "2026-01-01T00:00:00.000Z"}
	fid := GenerateRecordID(f, reg)
	f["id"] = fid
	steps := []parityStep{
		{Args: []string{"record", "knowledge", "--type", "decision", "--title", "Atomic writes", "--rationale", "A complete snapshot for readers", "--outcome-status", "partial", "--json"}},
		{Args: []string{"outcome", "knowledge", id, "--json"}},
		{Args: []string{"outcome", "knowledge", id, "--status", "success", "--duration", "12.5", "--agent", "test-agent", "--notes", "Verified locally", "--test-results", "Passed", "--json"}},
		{Args: []string{"edit", "knowledge", id, "--rationale", "Updated rationale", "--outcome-status", "failure", "--outcome-duration", "10", "--json"}},
		{Args: []string{"outcome", "knowledge", id}},
		{Args: []string{"record", "knowledge", "--type", "failure", "--description", "Shared fixture", "--resolution", "Unique directory", "--force", "--json"}},
		{Args: []string{"move", "knowledge", id, "target", "--dry-run", "--json"}},
		{Args: []string{"move", "knowledge", id, "target", "--json"}},
		{Args: []string{"archive", "target", id, "--reason", "Superseded by new guidance", "--dry-run", "--json"}},
		{Args: []string{"archive", "target", id, "--reason", "Superseded by new guidance", "--json"}},
		{Args: []string{"restore", id, "--json"}},
		{Args: []string{"delete", "target", id, "--dry-run", "--json"}},
		{Args: []string{"delete", "target", id, "--json"}},
		{Args: []string{"record", "brand-new", "--type", "guide", "--description", "A guide derived from description", "--dry-run", "--json", "--quiet"}},
		{Args: []string{"record", "knowledge", "--type", "pattern", "--description", "Use separate fixtures for concurrent jobs", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "decision", "--description", "Prefer domain locks. They isolate writers.", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "convention", "Prefer one lock per file because independent writers need concurrency", "--json"}},
		{Args: []string{"edit", "knowledge", fid, "--resolution", "New resolution"}},
		{Args: []string{"archive", "knowledge", fid, "--reason", "Corrected guidance"}},
		{Args: []string{"restore", fid}},
		{Args: []string{"delete", "knowledge", fid}},
		{Args: []string{"delete-domain", "target", "--dry-run", "--json"}},
		{Args: []string{"delete-domain", "target", "--json"}},
	}
	parityExecute(t, steps, parityWritePrepare(t, []Record{r, f}, nil))
}
func TestParityWriteBulkAndErrors(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	valid := `[{"type":"decision","title":"A decision","rationale":"A rationale"},{"type":"convention","content":"Avoid shared state because jobs overlap"},{"type":"convention","content":"Avoid shared state because jobs overlap"}]`
	partial := `[{"type":"failure","description":"Good failure","resolution":"Fix it"},{"type":"failure","description":"Missing resolution"}]`
	steps := []parityStep{
		{Args: []string{"record", "knowledge", "--stdin", "--dry-run", "--json"}, Input: valid},
		{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: valid},
		{Args: []string{"record", "knowledge", "--stdin", "--force", "--json"}, Input: valid},
		{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: partial},
		{Args: []string{"record", "new-domain", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "failure", "--description", "No resolution", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "failure", "--description", "No resolution"}},
		{Args: []string{"archive", "knowledge", "--reason", "No ID", "--json"}},
		{Args: []string{"archive", "knowledge", "mx-abc123", "--records", "mx-def456", "--reason", "Wrong modes", "--json"}},
		{Args: []string{"archive", "knowledge", "mx-abc123", "--reason", "   ", "--json"}},
		{Args: []string{"delete", "knowledge", "--json"}},
		{Args: []string{"delete", "knowledge", "mx-abc123", "--records", "mx-def456", "--json"}},
		{Args: []string{"delete", "knowledge", "--records", ",", "--json"}},
		{Args: []string{"delete", "knowledge", "--all-except", ",", "--json"}},
		{Args: []string{"move", "knowledge", "mx-abc123", "knowledge", "--json"}},
		{Args: []string{"move", "knowledge", "mx-abc123", "absent", "--json"}},
		{Args: []string{"restore", "mx-abc123", "--json"}},
	}
	parityExecute(t, steps, parityWritePrepare(t, nil, nil))
}
func TestParityWritePruneAndCompaction(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	rows := []Record{{"id": "mx-aaaaaa", "type": "decision", "title": "Old decision", "rationale": "Old reason", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}, {"id": "mx-bbbbbb", "type": "decision", "title": "New decision", "rationale": "New reason", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z", "supersedes": []string{"mx-aaaaaa"}}, {"id": "mx-cccccc", "type": "failure", "description": "Expired failure", "resolution": "An old resolution", "classification": "tactical", "recorded_at": "2025-01-01T00:00:00.000Z"}}
	steps := []parityStep{
		{Args: []string{"compact", "knowledge", "--analyze", "--json"}},
		{Args: []string{"prune", "--dry-run", "--explain", "--json"}},
		{Args: []string{"prune", "--explain", "--json"}},
		{Args: []string{"restore", "mx-cccccc", "--json"}},
		{Args: []string{"prune", "--aggressive", "--hard", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "failure", "--description", "First remaining failure", "--resolution", "First fix", "--classification", "foundational", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "failure", "--description", "Second remaining failure", "--resolution", "Second fix", "--classification", "foundational", "--json"}},
		{Args: []string{"compact", "knowledge", "--auto", "--min-group", "2", "--dry-run", "--json"}},
		{Args: []string{"compact", "knowledge", "--auto", "--min-group", "2", "--yes", "--json"}},
		{Args: []string{"compact", "knowledge", "--analyze", "--json"}},
		{Args: []string{"compact", "knowledge", "--auto", "--min-group", "3.7", "--json"}},
		{Args: []string{"compact", "knowledge", "--auto", "--max-records", "0", "--json"}},
		{Args: []string{"compact", "--apply", "--json"}},
		{Args: []string{"compact", "knowledge", "--json"}},
	}
	parityExecute(t, steps, parityWritePrepare(t, rows, nil))
}
func TestParityWriteDoctorFix(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	rows := []Record{{"id": "mx-aaaaaa", "type": "pattern", "name": "Broken anchors", "description": "Anchors point at absent files", "files": []string{"absent.ts"}, "dir_anchors": []string{"missing-dir"}, "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}, {"id": "mx-bbbbbb", "type": "convention", "content": "Avoid fixtures because races happen", "classification": "tactical", "recorded_at": "2025-01-01T00:00:00.000Z"}}
	steps := []parityStep{{Args: []string{"doctor", "--json"}}, {Args: []string{"doctor", "--fix", "--json"}}, {Args: []string{"doctor", "--json"}}, {Args: []string{"doctor"}}, {Args: []string{"doctor", "--quiet"}}}
	parityExecute(t, steps, parityWritePrepare(t, rows, func(dir string) {
		p := filepath.Join(dir, ".luuna", "atlas", "expertise", "orphan.jsonl")
		_ = os.WriteFile(p, []byte{}, 0666)
	}))
}
func TestParityWriteManualCompaction(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	rows := []Record{{"id": "mx-aaaaaa", "type": "convention", "content": "Prefer per-domain locks because writes overlap", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}, {"id": "mx-bbbbbb", "type": "convention", "content": "Avoid partial writes because readers need snapshots", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}}
	steps := []parityStep{
		{Args: []string{"compact", "knowledge", "--apply", "--records", "mx-aaaaaa", "--content", "Combined", "--json"}},
		{Args: []string{"compact", "knowledge", "--apply", "--records", "mx-aaaaaa,mx-bbbbbb", "--json"}},
		{Args: []string{"compact", "knowledge", "--apply", "--records", "mx-aaaaaa,mx-bbbbbb", "--content", "Prefer atomic locked writes because readers need snapshots", "--dry-run", "--json"}},
		{Args: []string{"restore", "mx-aaaaaa", "--json"}},
		{Args: []string{"restore", "mx-bbbbbb", "--json"}},
		{Args: []string{"delete", "knowledge", "--records", "mx-aaaaaa,mx-bbbbbb", "--dry-run", "--json"}},
		{Args: []string{"delete", "knowledge", "--records", "mx-aaaaaa,mx-bbbbbb", "--json"}},
	}
	parityExecute(t, steps, parityWritePrepare(t, rows, nil))
}

func writeParityShell(t *testing.T) {
	t.Helper()
	if _, e := exec.LookPath("sh"); e != nil {
		b, e := exec.Command("git", "--exec-path").Output()
		if e == nil {
			usr := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(strings.TrimSpace(string(b))))), "usr", "bin")
			if _, e = os.Stat(filepath.Join(usr, "sh.exe")); e == nil {
				t.Setenv("PATH", usr+string(os.PathListSeparator)+os.Getenv("PATH"))
			}
		}
	}
	if _, e := exec.LookPath("sh"); e != nil {
		t.Skip("POSIX sh required for real lifecycle hook parity")
	}
	t.Setenv("LUUNA_ATLAS_LOG_LEVEL", "silent")
	t.Setenv("FORCE_COLOR", "0")
}
func TestParityWriteLifecycleHooks(t *testing.T) {
	writeParityShell(t)
	row := Record{"id": "mx-aaaaaa", "type": "failure", "description": "Old failure", "resolution": "Old fix", "classification": "tactical", "recorded_at": "2025-01-01T00:00:00.000Z"}
	steps := []parityStep{
		{Args: []string{"record", "knowledge", "--type", "decision", "--title", "Hook record", "--rationale", "The hook receives the real candidate", "--json"}},
		{Args: []string{"prune", "--json"}},
		{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: `[{"type":"failure","description":"Hook batch","resolution":"Hook fix"}]`},
	}
	parityExecute(t, steps, parityWritePrepare(t, []Record{row}, func(dir string) {
		cfg, _ := ReadConfig(dir)
		cfg["hooks"] = map[string]any{"pre-record": []string{"cat"}, "post-record": []string{"cat > .luuna/atlas/post.json"}, "pre-prune": []string{"cat > .luuna/atlas/prune.json"}}
		if e := WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
	}))
}
func TestParityWriteBlockedHooks(t *testing.T) {
	writeParityShell(t)
	steps := []parityStep{{Args: []string{"record", "knowledge", "--type", "decision", "--title", "Blocked record", "--rationale", "No mutation is allowed", "--json"}}, {Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: `[{"type":"decision","title":"Blocked batch","rationale":"No mutation"}]`}, {Args: []string{"record", "knowledge", "--type", "decision", "--title", "Preview skips hooks", "--rationale", "No external side effects", "--dry-run", "--json"}}}
	parityExecute(t, steps, parityWritePrepare(t, nil, func(dir string) {
		cfg, _ := ReadConfig(dir)
		cfg["hooks"] = map[string]any{"pre-record": []string{"exit 7"}}
		if e := WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
	}))
}
func TestParityWritePostHookWarnings(t *testing.T) {
	writeParityShell(t)
	steps := []parityStep{{Args: []string{"record", "knowledge", "--type", "decision", "--title", "Post hook does not block", "--rationale", "Store writes already completed", "--json"}}}
	parityExecute(t, steps, parityWritePrepare(t, nil, func(dir string) {
		cfg, _ := ReadConfig(dir)
		cfg["hooks"] = map[string]any{"post-record": []string{"exit 4"}}
		if e := WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
	}))
}
func TestParityWriteSemanticCompactionHook(t *testing.T) {
	writeParityShell(t)
	rows := []Record{{"id": "mx-aaaaaa", "type": "failure", "description": "First", "resolution": "First fix", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}, {"id": "mx-bbbbbb", "type": "failure", "description": "Second", "resolution": "Second fix", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}}
	replacement := `{"replacement":{"type":"failure","description":"Summarized failure","resolution":"Summarized fix","classification":"foundational","recorded_at":"2025-01-01T00:00:00.000Z"}}`
	steps := []parityStep{{Args: []string{"compact", "knowledge", "--apply", "--records", "mx-aaaaaa,mx-bbbbbb", "--json"}}}
	parityExecute(t, steps, parityWritePrepare(t, rows, func(dir string) {
		cfg, _ := ReadConfig(dir)
		cfg["hooks"] = map[string]any{"pre-compact": []string{"printf '%s' '" + replacement + "'"}}
		if e := WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
	}))
}

func TestParityWriteValidation(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	steps := []parityStep{
		{Args: []string{"record", "knowledge", "--type", "decision", "--title", "Invalid link", "--rationale", "Malformed reference must fail", "--relates-to", "not-an-id", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "decision", "--title", "Invalid supersession", "--rationale", "Malformed supersession must fail", "--supersedes", "mx-zzzzzz", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "failure", "--description", "Bad duration", "--resolution", "Valid fix", "--outcome-status", "partial", "--outcome-duration", "2abc", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "convention", "--content", "Avoid traversal because anchors must stay relative", "--dir-anchor", "../outside", "--json"}},
		{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: `null`},
		{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: `[1,false,"text"]`},
		{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: `[{"type":"failure","description":"Wrong resolution type","resolution":4},{"type":"guide","name":"Incomplete"},{"type":"decision","title":"Extra field","rationale":"A reason","unexpected":"extra"}]`},
	}
	parityExecute(t, steps, parityWritePrepare(t, nil, nil))
}
func TestParityWriteCustomTypes(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	prepare := parityWritePrepare(t, nil, func(dir string) {
		cfg, _ := ReadConfig(dir)
		cfg["custom_types"] = map[string]any{"hypothesis": map[string]any{"required": []string{"statement", "prediction"}, "optional": []string{"sources"}, "dedup_key": "statement", "summary": "{statement}", "extracts_files": true, "files_field": "sources", "compact": "merge_outcomes"}}
		if e := WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
	})
	reg, _ := NewRegistry(Config{"custom_types": map[string]any{"hypothesis": map[string]any{"required": []string{"statement", "prediction"}, "optional": []string{"sources"}, "dedup_key": "statement", "summary": "{statement}", "extracts_files": true, "files_field": "sources", "compact": "merge_outcomes"}}})
	id := GenerateRecordID(Record{"type": "hypothesis", "statement": "Fixture isolation removes races"}, reg)
	steps := []parityStep{
		{Args: []string{"record", "knowledge", "--type", "hypothesis", "--statement", "Fixture isolation removes races", "--prediction", "Parallel runs stay deterministic", "--sources", "a.go,b.go", "--outcome-status", "success", "--json"}},
		{Args: []string{"edit", "knowledge", id, "--prediction", "Repeated parallel runs stay deterministic", "--sources", "c.go", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "hypothesis", "--statement", "Fixture isolation removes races", "--prediction", "New confirmation", "--outcome-status", "partial", "--json"}},
		{Args: []string{"outcome", "knowledge", id, "--status", "success", "--json"}},
		{Args: []string{"move", "knowledge", id, "target", "--json"}},
		{Args: []string{"archive", "target", id, "--reason", "Experiment completed", "--json"}},
		{Args: []string{"restore", id, "--json"}},
	}
	parityExecute(t, steps, prepare)
}
func TestParityWriteAnchorsAndHumanMaintenance(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	rows := []Record{{"id": "mx-aaaaaa", "type": "pattern", "name": "Old anchors", "description": "Missing code anchors", "files": []string{"missing.go"}, "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}, {"id": "mx-bbbbbb", "type": "pattern", "name": "Current anchors", "description": "Current replacement", "files": []string{"present.go"}, "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z", "supersedes": []string{"mx-aaaaaa"}}, {"id": "mx-cccccc", "type": "pattern", "name": "Third pattern", "description": "Also a pattern", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}}
	steps := []parityStep{
		{Args: []string{"compact", "knowledge", "--analyze"}},
		{Args: []string{"compact", "knowledge", "--analyze", "--quiet"}},
		{Args: []string{"compact", "knowledge", "--auto", "--min-group", "2", "--dry-run"}},
		{Args: []string{"prune", "--dry-run", "--check-anchors", "--explain", "--json"}},
		{Args: []string{"prune", "--dry-run", "--check-anchors", "--explain"}},
		{Args: []string{"prune", "--check-anchors", "--quiet"}},
		{Args: []string{"delete-domain", "target"}, Input: "no\n"},
		{Args: []string{"compact", "knowledge", "--auto", "--min-group", "2"}, Input: "no\n"},
	}
	parityExecute(t, steps, parityWritePrepare(t, rows, func(dir string) {
		_ = os.WriteFile(filepath.Join(dir, "present.go"), []byte("package present\n"), 0666)
	}))
}

func TestParityWriteColors(t *testing.T) {
	reference := parityReference(t)
	for _, level := range []string{"1", "2", "3"} {
		t.Run("level-"+level, func(t *testing.T) {
			t.Setenv("FORCE_COLOR", level)
			refDir, goDir := parityProjects(t)
			rows := []Record{{"id": "mx-aaaaaa", "type": "failure", "description": "First", "resolution": "First fix", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}, {"id": "mx-bbbbbb", "type": "failure", "description": "Second", "resolution": "Second fix", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}, {"id": "mx-cccccc", "type": "failure", "description": "Third", "resolution": "Third fix", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}}
			prepare := parityWritePrepare(t, rows, nil)
			prepare(refDir)
			prepare(goDir)
			started := time.Now()
			steps := []parityStep{
				{Args: []string{"record", "knowledge", "--type", "decision", "--title", "Colored decision", "--rationale", "Keep terminal interaction unchanged"}},
				{Args: []string{"edit", "knowledge", "mx-aaaaaa", "--resolution", "New fix"}},
				{Args: []string{"outcome", "knowledge", "mx-aaaaaa", "--status", "success", "--agent", "agent", "--duration", "5", "--notes", "Note"}},
				{Args: []string{"move", "knowledge", "mx-aaaaaa", "target", "--dry-run"}},
				{Args: []string{"move", "knowledge", "mx-aaaaaa", "target"}},
				{Args: []string{"archive", "target", "mx-aaaaaa", "--reason", "Moved guidance", "--dry-run"}},
				{Args: []string{"archive", "target", "mx-aaaaaa", "--reason", "Moved guidance"}},
				{Args: []string{"restore", "mx-aaaaaa"}},
				{Args: []string{"delete", "knowledge", "mx-bbbbbb", "--dry-run"}},
				{Args: []string{"compact", "knowledge", "--analyze"}},
			}
			for n, step := range steps {
				cmd := exec.Command("bun", append([]string{reference}, step.Args...)...)
				cmd.Dir = refDir
				cmd.Env = append(os.Environ(), "FORCE_COLOR="+level, "LUUNA_ATLAS_LOG_JSON=1")
				var out, stderr bytes.Buffer
				cmd.Stdout = &out
				cmd.Stderr = &stderr
				cmd.Stdin = strings.NewReader(step.Input)
				err := cmd.Run()
				code := 0
				if exit, ok := err.(*exec.ExitError); ok {
					code = exit.ExitCode()
				} else if err != nil {
					t.Fatal(err)
				}
				want := parityResult{Code: code, Out: out.String(), Err: stderr.String()}
				got := parityRun(t, reference, goDir, step, true)
				parityCompare(t, fmt.Sprintf("level %s step%d: %v", level, n, step.Args), want, got, refDir, goDir, started)
			}
		})
	}
}
