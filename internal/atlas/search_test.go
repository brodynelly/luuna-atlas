package atlas

import (
	"testing"
	"time"
)

func TestNativeBM25AndOutcomeRanking(t *testing.T) {
	a := Record{"type": "pattern", "name": "Retries", "description": "retry timeout backoff", "classification": "tactical", "recorded_at": "now"}
	b := Record{"type": "pattern", "name": "Timeout", "description": "retry timeout backoff", "classification": "tactical", "recorded_at": "now", "outcomes": []any{map[string]any{"status": "success"}, map[string]any{"status": "partial"}, map[string]any{"status": "failure"}}}
	if ConfirmationScore(b) != 1.5 || SuccessRate(b) != .5 {
		t.Fatal("outcome scoring changed")
	}
	hits := SearchRecords([]Record{a, b}, "retry", .1)
	if len(hits) != 2 || Text(hits[0]["name"]) != "Timeout" {
		t.Fatalf("boost ranking wrong: %v", hits)
	}
	if len(SearchRecords([]Record{a, b}, "retr", 0)) != 0 {
		t.Fatal("search became substring matching")
	}
}
func TestNativeRetentionAnchorsAndCycles(t *testing.T) {
	now := ParseDate("2026-10-22T12:00:00Z")
	r := testRecord("x")
	r["recorded_at"] = "2026-10-08T12:00:00Z"
	shelf := map[string]any{"tactical": 14, "observational": 30}
	if IsRecordStale(r, now, shelf) {
		t.Fatal("14 day boundary changed")
	}
	if !IsRecordStale(r, now.Add(24*time.Hour), shelf) {
		t.Fatal("15 day stale missing")
	}
	r["id"] = "mx-aaaaaa"
	s := CloneRecord(r)
	s["id"] = "mx-bbbbbb"
	r["supersedes"] = []string{"mx-bbbbbb"}
	s["supersedes"] = []string{"mx-aaaaaa"}
	sup, cycles := CollectSupersededIDs([]DomainRecords{{Domain: "x", Records: []Record{r, s}}})
	if len(cycles) != 2 || len(sup) != 0 {
		t.Fatalf("cycle handling wrong: %v %v", sup, cycles)
	}
	v := AnchorValidity(r, t.TempDir())
	if v.ValidFraction != nil {
		t.Fatal("unanchored ratio should be null")
	}
}
func TestNativeArchiveRoundTrip(t *testing.T) {
	cwd, reg := fixtureStore(t)
	r := testRecord("Archive this")
	r["id"] = GenerateRecordID(r, reg)
	if e := ArchiveRecords(cwd, "testing", []Record{r}, time.Now(), "manual: test", reg); e != nil {
		t.Fatal(e)
	}
	path, _ := ArchivePath(cwd, "testing")
	rows, e := ReadArchive(path, reg)
	if e != nil || len(rows) != 1 || Text(rows[0]["status"]) != "archived" {
		t.Fatalf("archive %v %v", rows, e)
	}
	removed, e := RemoveFromArchive(cwd, "testing", Text(r["id"]), reg)
	if e != nil || removed == nil {
		t.Fatal(e)
	}
	live, _ := ExpertisePath(cwd, "testing")
	conflict, e := RestoreToExpertise(live, StripArchiveFields(removed), reg)
	if e != nil || conflict != nil {
		t.Fatal("restore failed")
	}
	conflict, e = RestoreToExpertise(live, StripArchiveFields(removed), reg)
	if e != nil || conflict == nil {
		t.Fatal("restore conflict not enforced")
	}
}
