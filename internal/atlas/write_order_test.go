package atlas

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// JSON property order is observable in terminal snapshots and JSONL files.
// These assertions compare literal strings, normalizing only project paths and
// generated timestamps; they do not parse or reserialize the compared JSON.
func parityWriteStrict(t *testing.T, steps []parityStep, prepare func(string)) {
	parityWriteStrictFiles(t, steps, prepare, true)
}
func parityWriteStrictFiles(t *testing.T, steps []parityStep, prepare func(string), strictOutput bool) {
	t.Helper()
	reference := parityReference(t)
	refDir, goDir := parityProjects(t)
	prepare(refDir)
	prepare(goDir)
	started := time.Now()
	for n, step := range steps {
		label := fmt.Sprintf("step %d %v", n, step.Args)
		ref := parityRun(t, reference, refDir, step, false)
		got := parityRun(t, reference, goDir, step, true)
		parityCompare(t, label, ref, got, refDir, goDir, started)
		for _, stream := range []struct{ name, ref, got string }{{"stdout", ref.Out, got.Out}, {"stderr", ref.Err, got.Err}} {
			if strictOutput && parityNormalize(stream.ref, refDir, started) != parityNormalize(stream.got, goDir, started) {
				t.Errorf("%s literal %s mismatch\nREFERENCE:\n%s\nGO:\n%s", label, stream.name, stream.ref, stream.got)
			}
		}
		for _, domain := range []string{"knowledge", "target", "links"} {
			for _, folder := range []string{"expertise", "archive"} {
				refData, e := os.ReadFile(filepath.Join(refDir, ".luuna", "atlas", folder, domain+".jsonl"))
				gotData, gotError := os.ReadFile(filepath.Join(goDir, ".luuna", "atlas", folder, domain+".jsonl"))
				if os.IsNotExist(e) && os.IsNotExist(gotError) {
					continue
				}
				if e != nil {
					t.Fatal(e)
				}
				if gotError != nil {
					t.Fatal(gotError)
				}
				if parityNormalize(string(refData), refDir, started) != parityNormalize(string(gotData), goDir, started) {
					t.Errorf("%s literal %s/%s.jsonl mismatch\nREFERENCE:\n%s\nGO:\n%s", label, folder, domain, refData, gotData)
				}
			}
		}
	}
}

func TestParityWriteStrictMaintenanceJSONL(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	rows := []Record{
		{"id": "mx-aaaaaa", "type": "pattern", "name": "Old anchored pattern", "description": "Both anchor and supersession decay apply", "files": []string{"missing.go"}, "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"id": "mx-bbbbbb", "type": "pattern", "name": "Replacement pattern", "description": "Supersede the old pattern", "supersedes": []string{"mx-aaaaaa"}, "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"id": "mx-cccccc", "type": "failure", "description": "First failure", "resolution": "First fix", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"id": "mx-dddddd", "type": "failure", "description": "Second failure", "resolution": "Second fix", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"id": "mx-eeeeee", "type": "decision", "title": "Expired decision", "rationale": "Exercise stale archival", "classification": "tactical", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"id": "mx-111111", "type": "finding", "question": "Does order survive merge?", "answer": "Old answer", "sources": []string{"old.go"}, "tags": []string{"ordered"}, "outcomes": []Record{{"status": "success", "agent": "first"}}, "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"id": "mx-222222", "type": "finding", "question": "Does order survive merge?", "answer": "Latest answer", "sources": []string{"latest.go"}, "outcomes": []Record{{"status": "partial", "agent": "second"}}, "classification": "foundational", "recorded_at": "2026-01-01T00:00:00.000Z"},
	}
	prepare := parityWritePrepare(t, rows, func(dir string) {
		cfg, _ := ReadConfig(dir)
		cfg["custom_types"] = map[string]any{"finding": map[string]any{"required": []string{"question", "answer"}, "optional": []string{"sources"}, "extracts_files": true, "files_field": "sources", "dedup_key": "question", "id_key": "question", "summary": "{question}", "compact": "merge_outcomes"}}
		if e := WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
	})
	manualID := GenerateRecordID(Record{"type": "failure", "description": "Combined failure"}, BuiltinRegistry())
	parityWriteStrictFiles(t, []parityStep{
		{Args: []string{"prune", "--check-anchors", "--explain", "--json"}},
		{Args: []string{"compact", "knowledge", "--apply", "--records", "mx-cccccc,mx-dddddd", "--description", "Combined failure", "--resolution", "Atomic writes", "--json"}},
		{Args: []string{"compact", "knowledge", "--auto", "--min-group", "2", "--yes", "--json"}},
		{Args: []string{"archive", "knowledge", manualID, "--reason", "Complete", "--json"}},
		{Args: []string{"restore", "mx-cccccc", "--json"}},
	}, prepare, false)
}

func TestParityWriteStrictPropertyOrder(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	id := GenerateRecordID(Record{"type": "pattern", "name": "Ordered pattern"}, BuiltinRegistry())
	steps := []parityStep{
		{Args: []string{"record", "knowledge", "--type", "pattern", "--name", "Ordered pattern", "--description", "Keep property order stable", "--files", "a.go,b.go", "--tags", "first,second", "--relates-to", "mx-aaaaaa", "--supersedes", "links:mx-bbbbbb", "--dir-anchor", "src", "--evidence-commit", "abc123", "--evidence-issue", "issue-1", "--evidence-file", "a.go", "--evidence-bead", "bd-1", "--evidence-missions", "work-1", "--evidence-gh", "#1", "--evidence-linear", "TEAM-1", "--outcome-status", "success", "--outcome-duration", "4.5", "--outcome-test-results", "Passed", "--outcome-agent", "ordered-agent", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "pattern", "--name", "Ordered pattern", "--description", "Upserts preserve nested ordering", "--files", "c.go", "--evidence-missions", "work-2", "--evidence-gh", "#2", "--outcome-status", "partial", "--outcome-agent", "new-agent", "--json"}},
		{Args: []string{"edit", "knowledge", id, "--classification", "observational", "--relates-to", "mx-cccccc", "--supersedes", "mx-dddddd", "--outcome-status", "failure", "--outcome-duration", "5", "--outcome-test-results", "Failed", "--outcome-agent", "edit-agent", "--description", "Updates keep existing property positions", "--json"}},
		{Args: []string{"outcome", "knowledge", id, "--status", "success", "--duration", "6", "--agent", "outcome-agent", "--notes", "Ordered note", "--test-results", "Passed", "--json"}},
		{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: `[{"title":"Bulk title","type":"decision","rationale":"Keep input order","evidence":{"linear":"TEAM-3","missions":"work-3","commit":"abc"},"outcomes":[{"agent":"existing","status":"success","duration":7}]}]`},
		{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: `[{"type":"decision","title":"Bulk title","rationale":"Upsert reordered input","outcomes":[{"status":"partial","agent":"new"}]}]`},
		{Args: []string{"record", "knowledge", "--type", "decision", "--title", "Preview property order", "--rationale", "Dry runs keep the same construction order", "--evidence-gh", "#4", "--outcome-status", "success", "--dry-run", "--json"}},
	}
	parityWriteStrict(t, steps, parityWritePrepare(t, nil, nil))
}

func TestParityWriteStrictCustomPropertyOrder(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	prepare := parityWritePrepare(t, nil, func(dir string) {
		cfg, _ := ReadConfig(dir)
		cfg["custom_types"] = map[string]any{"finding": map[string]any{"required": []string{"question", "answer"}, "optional": []string{"sources", "owner"}, "extracts_files": true, "files_field": "sources", "dedup_key": "question", "summary": "{question}"}}
		if e := WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
	})
	parityWriteStrict(t, []parityStep{
		{Args: []string{"record", "knowledge", "--type", "finding", "--question", "Does native order survive?", "--answer", "Keep declared required/optional order", "--sources", "a.go,b.go", "--owner", "maintainer", "--tags", "ordered", "--evidence-missions", "work-4", "--evidence-linear", "TEAM-4", "--outcome-status", "success", "--outcome-test-results", "Passed", "--outcome-agent", "agent", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "finding", "--question", "Does native order survive?", "--answer", "Nested evidence/outcomes survive upsert", "--sources", "c.go", "--owner", "maintainer", "--evidence-linear", "TEAM-5", "--outcome-status", "partial", "--outcome-agent", "agent-2", "--json"}},
	}, prepare)
}

func TestParityWriteStrictAutoCommitPropertyOrder(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	if _, e := exec.LookPath("git"); e != nil {
		t.Skip("Git context requires Git")
	}
	prepare := parityWritePrepare(t, nil, func(dir string) {
		for _, args := range [][]string{{"init", "-q", "--initial-branch=main"}, {"-c", "commit.gpgsign=false", "-c", "user.name=AtlasFixture", "-c", "user.email=fixture@example.invalid", "commit", "-q", "--allow-empty", "-m", "Isolated fixture"}} {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2025-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2025-01-01T00:00:00Z")
			if output, e := cmd.CombinedOutput(); e != nil {
				t.Fatalf("fixture git %v: %v %s", args, e, output)
			}
		}
	})
	parityWriteStrict(t, []parityStep{
		{Args: []string{"record", "knowledge", "--type", "failure", "--description", "Implicit commits follow explicit evidence", "--resolution", "Append the auto commit after existing fields", "--evidence-file", "a.go", "--evidence-missions", "work-6", "--json"}},
	}, prepare)
}
