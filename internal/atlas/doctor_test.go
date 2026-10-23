package atlas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func doctorTestFind(result map[string]any, name string) map[string]any {
	for _, v := range AnyList(result["checks"]) {
		m := Obj(v)
		if Text(m["name"]) == name {
			return m
		}
	}
	return nil
}
func TestDoctorHealthyStoreReportsSeventeenChecks(t *testing.T) {
	dir := writeTestStore(t)
	code, result, stderr := writeTestInvoke(t, dir, "", "doctor")
	if code != 0 || len(AnyList(result["checks"])) != 17 || Number(Obj(result["summary"])["pass"]) != 17 {
		t.Fatalf("healthy doctor: %d %#v %s", code, result, stderr)
	}
	checks := AnyList(result["checks"])
	names := []string{"config", "jsonl-integrity", "legacy-outcome", "schema-validation", "unknown-types", "type-registry", "domain-rules-compatibility", "domain-conformance", "domain-violations", "stale-records", "orphaned-domains", "duplicates", "file-anchors", "governance", "decay-config", "compact-summarizer", "upgrade"}
	for n, name := range names {
		if Text(Obj(checks[n])["name"]) != name {
			t.Fatalf("check order changed at %d: %#v", n, checks[n])
		}
	}
}
func TestDoctorFixUsesPreFixFailureSummaryAndMigratesLegacyOutcome(t *testing.T) {
	dir := writeTestStore(t)
	path, _ := ExpertisePath(dir, "knowledge")
	row := `{"id":"mx-abcdef","type":"convention","content":"Avoid sharing test files because races corrupt them","classification":"foundational","recorded_at":"2026-01-01T00:00:00.000Z","outcome":{"status":"success","notes":"legacy notes are dropped","duration":12}}` + "\n"
	if e := os.WriteFile(path, []byte(row), 0666); e != nil {
		t.Fatal(e)
	}
	code, result, stderr := writeTestInvoke(t, dir, "", "doctor", "--fix")
	if code != 1 || Text(doctorTestFind(result, "legacy-outcome")["status"]) != "warn" || len(List(result["fixed"])) != 1 {
		t.Fatalf("pre-fix result: %d %#v %s", code, result, stderr)
	}
	rows := writeTestRows(t, dir, "knowledge")
	if len(rows) != 1 || rows[0]["outcome"] != nil || len(RecordList(rows[0]["outcomes"])) != 1 {
		t.Fatalf("legacy not migrated: %#v", rows)
	}
	o := RecordList(rows[0]["outcomes"])[0]
	if o["notes"] != nil {
		t.Fatalf("legacy normalization gained fields: %#v", o)
	}
	code, result, stderr = writeTestInvoke(t, dir, "", "doctor")
	if code != 0 || Number(Obj(result["summary"])["fail"]) != 0 {
		t.Fatalf("post-fix result: %#v %s", result, stderr)
	}
}
func TestDoctorFixHardRemovesStaleAndInvalidSchemaRecords(t *testing.T) {
	dir := writeTestStore(t)
	old := writeTestFailure("Old observation")
	old["classification"] = "tactical"
	old["recorded_at"] = ISO(time.Now().Add(-40 * 24 * time.Hour))
	good := writeTestFailure("Keep active failure")
	invalid := Record{"id": "mx-aaaaaa", "type": "failure", "classification": "foundational", "recorded_at": NowISO(), "description": "Missing resolution"}
	writeTestSeed(t, dir, "knowledge", []Record{old, good, invalid})
	code, result, stderr := writeTestInvoke(t, dir, "", "doctor", "--fix")
	if code != 1 || len(List(result["fixed"])) != 2 {
		t.Fatalf("fix removals: %d %#v %s", code, result, stderr)
	}
	rows := writeTestRows(t, dir, "knowledge")
	if len(rows) != 1 || Text(rows[0]["id"]) != Text(good["id"]) {
		t.Fatalf("wrong fixed corpus: %#v", rows)
	}
	archive, _ := ArchivePath(dir, "knowledge")
	if _, e := os.Stat(archive); !os.IsNotExist(e) {
		t.Fatal("doctor fix unexpectedly soft-archived removed records")
	}
}
func TestDoctorUnknownTypesFailButSurviveFix(t *testing.T) {
	dir := writeTestStore(t)
	path, _ := ExpertisePath(dir, "knowledge")
	row := `{"id":"mx-abcdef","type":"future_type","classification":"foundational","recorded_at":"2026-01-01T00:00:00.000Z","statement":"Leave unknown records intact"}` + "\n"
	if e := os.WriteFile(path, []byte(row), 0666); e != nil {
		t.Fatal(e)
	}
	code, result, stderr := writeTestInvoke(t, dir, "", "doctor", "--fix")
	if code != 1 || Text(doctorTestFind(result, "unknown-types")["status"]) != "fail" || Text(doctorTestFind(result, "schema-validation")["status"]) != "pass" {
		t.Fatalf("unknown handling: %d %#v %s", code, result, stderr)
	}
	b, e := os.ReadFile(path)
	if e != nil || string(b) != row {
		t.Fatalf("unknown row was rewritten/deleted: %s %v", b, e)
	}
}
func TestDoctorFixStripsBrokenAnchorsAndRepairsOrphans(t *testing.T) {
	dir := writeTestStore(t)
	row := Record{"type": "pattern", "name": "Fixture", "description": "Keep path anchors valid", "classification": "foundational", "recorded_at": NowISO(), "files": []string{"missing.go"}, "dir_anchors": []string{"missing-dir"}, "evidence": map[string]any{"file": "missing-evidence.go", "commit": "abc"}}
	writeTestSeed(t, dir, "knowledge", []Record{row})
	orphan := filepath.Join(dir, ".luuna", "atlas", "expertise", "orphan.jsonl")
	if e := os.WriteFile(orphan, []byte{}, 0666); e != nil {
		t.Fatal(e)
	}
	code, result, stderr := writeTestInvoke(t, dir, "", "doctor", "--fix")
	if code != 0 || len(List(result["fixed"])) != 2 {
		t.Fatalf("anchor/orphan fix: %d %#v %s", code, result, stderr)
	}
	rows := writeTestRows(t, dir, "knowledge")
	if rows[0]["files"] != nil || rows[0]["dir_anchors"] != nil || Obj(rows[0]["evidence"])["file"] != nil || Text(Obj(rows[0]["evidence"])["commit"]) != "abc" {
		t.Fatalf("anchor fields not stripped correctly: %#v", rows)
	}
	cfg, e := ReadConfig(dir)
	if e != nil || !writeHasDomain(cfg, "orphan") {
		t.Fatal("orphan not registered")
	}
}
func TestDoctorReportsDomainRulesIndependentlyOfSchema(t *testing.T) {
	dir := writeTestStore(t)
	cfg, _ := ReadConfig(dir)
	Obj(cfg["domains"])["knowledge"] = map[string]any{"allowed_types": []string{"decision"}, "required_fields": []string{"owner"}}
	if e := WriteConfig(dir, cfg); e != nil {
		t.Fatal(e)
	}
	row := writeTestFailure("Type not allowed here")
	writeTestSeed(t, dir, "knowledge", []Record{row})
	code, result, stderr := writeTestInvoke(t, dir, "", "doctor")
	if code != 1 || Text(doctorTestFind(result, "schema-validation")["status"]) != "pass" || Text(doctorTestFind(result, "domain-violations")["status"]) != "fail" {
		t.Fatalf("domain conformance: %d %#v %s", code, result, stderr)
	}
	details := List(doctorTestFind(result, "domain-violations")["details"])
	if len(details) != 2 || !strings.Contains(details[0], "not in allowed_types") || !strings.Contains(details[1], "missing required field(s): owner") {
		t.Fatalf("domain details: %#v", details)
	}
}
