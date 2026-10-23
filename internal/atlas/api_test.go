package atlas

import "testing"

func TestNativeLibraryBoundaryKeepsItsDistinctUpsertAndMutation(t *testing.T) {
	cwd, _ := fixtureStore(t)
	r := testRecord("API record")
	result, e := RecordExpertise("testing", r, RecordOptions{Cwd: cwd})
	if e != nil || result.Action != "created" || Text(r["id"]) == "" {
		t.Fatalf("API create %v %v", result, e)
	}
	skip, e := RecordExpertise("testing", testRecord("API record"), RecordOptions{Cwd: cwd})
	if e != nil || skip.Action != "skipped" {
		t.Fatalf("API duplicate %v %v", skip, e)
	}
	forced, e := RecordExpertise("testing", testRecord("API record"), RecordOptions{Cwd: cwd, Force: true})
	if e != nil || forced.Action != "created" {
		t.Fatalf("API force %v %v", forced, e)
	}
	rows, e := QueryDomain("testing", QueryOptions{Cwd: cwd})
	if e != nil || len(rows) != 2 {
		t.Fatal("API force should append duplicate")
	}
}
func TestNativeLibraryNamedUpsertReplacesOutcomes(t *testing.T) {
	cwd, _ := fixtureStore(t)
	a := Record{"type": "pattern", "name": "stable name", "description": "first", "classification": "tactical", "recorded_at": "now", "outcomes": []any{map[string]any{"status": "success"}}}
	if _, e := RecordExpertise("testing", a, RecordOptions{Cwd: cwd}); e != nil {
		t.Fatal(e)
	}
	b := Record{"type": "pattern", "name": "stable name", "description": "replacement", "classification": "tactical", "recorded_at": "now"}
	upsert, e := RecordExpertise("testing", b, RecordOptions{Cwd: cwd})
	if e != nil || upsert.Action != "updated" {
		t.Fatal(e)
	}
	if _, exists := upsert.Record["outcomes"]; exists {
		t.Fatal("API upsert unexpectedly merged CLI outcome behavior")
	}
	edited, e := EditRecord("testing", Text(b["id"]), Record{"description": "new", "content": "ignored"}, EditOptions{Cwd: cwd})
	if e != nil || Text(edited["description"]) != "new" {
		t.Fatal(e)
	}
	if _, exists := edited["content"]; exists {
		t.Fatal("API edited foreign type field")
	}
	outcome, e := AppendOutcome("testing", Text(b["id"]), Record{"status": "partial"}, OutcomeOptions{Cwd: cwd})
	if e != nil || outcome.TotalOutcomes != 1 || Text(outcome.Outcome["recorded_at"]) == "" {
		t.Fatalf("outcome %v %v", outcome, e)
	}
}
func TestNativeLibraryPrivateBridge(t *testing.T) {
	cwd, _ := fixtureStore(t)
	r := testRecord("private native bridge")
	result, e := RunAPIBridge(map[string]any{"operation": "recordExpertise", "args": []any{"testing", r, map[string]any{"cwd": cwd}}})
	if e != nil {
		t.Fatal(e)
	}
	if result.(RecordResult).Action != "created" {
		t.Fatalf("bridge %v", result)
	}
	matches, e := RunAPIBridge(map[string]any{"operation": "searchExpertise", "args": []any{"bridge", map[string]any{"cwd": cwd}}})
	if e != nil || len(matches.([]SearchResult)) != 1 {
		t.Fatalf("bridge search %v %v", matches, e)
	}
}
