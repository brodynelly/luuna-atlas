package atlas

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestStore(t *testing.T) string {
	t.Helper()
	t.Setenv("FORCE_COLOR", "0")
	dir := t.TempDir()
	if _, _, e := InitStore(dir); e != nil {
		t.Fatal(e)
	}
	if e := AddDomain(dir, "knowledge"); e != nil {
		t.Fatal(e)
	}
	return dir
}
func writeTestInvoke(t *testing.T, dir, input string, args ...string) (int, map[string]any, string) {
	t.Helper()
	out, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	app := NewApp()
	app.Cwd = dir
	app.Out = out
	app.Err = stderr
	app.In = strings.NewReader(input)
	args = append([]string{"--json", "--quiet"}, args...)
	code := app.Run(args)
	result := map[string]any{}
	if out.Len() > 0 {
		if e := json.Unmarshal(out.Bytes(), &result); e != nil {
			t.Fatalf("invalid output JSON for %v: %s\nstderr:%s", args, out.String(), stderr.String())
		}
	}
	return code, result, stderr.String()
}
func writeTestRows(t *testing.T, dir, domain string) []Record {
	t.Helper()
	p, e := ExpertisePath(dir, domain)
	if e != nil {
		t.Fatal(e)
	}
	r, e := ReadRecords(p, GetRegistry(), true)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func writeTestSeed(t *testing.T, dir, domain string, rows []Record) {
	t.Helper()
	p, e := ExpertisePath(dir, domain)
	if e != nil {
		t.Fatal(e)
	}
	if e = WriteRecords(p, rows); e != nil {
		t.Fatal(e)
	}
}
func writeTestFailure(description string) Record {
	return Record{"type": "failure", "description": description, "resolution": "Use separate unique fixtures", "classification": "foundational", "recorded_at": NowISO()}
}

func TestWriteRecordCreatesAndExactForceStillRejects(t *testing.T) {
	dir := writeTestStore(t)
	code, result, stderr := writeTestInvoke(t, dir, "", "record", "knowledge", "--type", "failure", "--description", "Parallel jobs shared a fixture", "--resolution", "Use separate unique fixtures")
	if code != 0 {
		t.Fatalf("create: %s", stderr)
	}
	record := Obj(result["record"])
	if Text(result["action"]) != "created" || Text(record["id"]) == "" {
		t.Fatalf("unexpected create result %#v", result)
	}
	code, _, stderr = writeTestInvoke(t, dir, "", "record", "knowledge", "--type", "failure", "--description", "Parallel jobs shared a fixture", "--resolution", "Use separate unique fixtures", "--force")
	if code == 0 || !strings.Contains(stderr, "duplicate failure already exists") {
		t.Fatalf("force duplicated anonymous record: %d %s", code, stderr)
	}
	if len(writeTestRows(t, dir, "knowledge")) != 1 {
		t.Fatal("duplicate modified corpus")
	}
}
func TestWriteRecordDryRunStillCreatesDomain(t *testing.T) {
	dir := writeTestStore(t)
	code, result, stderr := writeTestInvoke(t, dir, "", "record", "new-domain", "--type", "decision", "--title", "Choose atomic replacement", "--rationale", "Readers need a complete file", "--dry-run")
	if code != 0 || Text(result["wouldDo"]) != "created" {
		t.Fatalf("preview: %d %#v %s", code, result, stderr)
	}
	cfg, e := ReadConfig(dir)
	if e != nil {
		t.Fatal(e)
	}
	if !writeHasDomain(cfg, "new-domain") {
		t.Fatal("compatibility domain-creation side effect was lost")
	}
	if len(writeTestRows(t, dir, "new-domain")) != 0 {
		t.Fatal("preview wrote record")
	}
}
func TestWriteNamedUpsertMergesOutcomeHistory(t *testing.T) {
	dir := writeTestStore(t)
	code, result, stderr := writeTestInvoke(t, dir, "", "record", "knowledge", "--type", "decision", "--title", "Use atomic writes", "--rationale", "Complete snapshots", "--outcome-status", "success")
	if code != 0 {
		t.Fatal(stderr)
	}
	id := Text(Obj(result["record"])["id"])
	code, result, stderr = writeTestInvoke(t, dir, "", "record", "knowledge", "--type", "decision", "--title", "Use atomic writes", "--rationale", "Readers and concurrent workers need complete snapshots", "--outcome-status", "partial")
	if code != 0 || Text(result["action"]) != "updated" {
		t.Fatalf("upsert: %#v %s", result, stderr)
	}
	rows := writeTestRows(t, dir, "knowledge")
	if len(rows) != 1 || Text(rows[0]["id"]) != id || len(RecordList(rows[0]["outcomes"])) != 2 {
		t.Fatalf("upsert lost ID/history: %#v", rows)
	}
}
func TestWriteBulkPartialSuccessAndAnonymousDedup(t *testing.T) {
	dir := writeTestStore(t)
	input := `[{"type":"failure","description":"A real failure","resolution":"A real fix"},{"type":"failure","description":"Incomplete"},{"type":"failure","description":"A real failure","resolution":"A real fix"}]`
	code, result, stderr := writeTestInvoke(t, dir, input, "record", "knowledge", "--stdin")
	if code != 0 || Number(result["created"]) != 1 || Number(result["skipped"]) != 1 || len(AnyList(result["errors"])) != 1 || !strings.Contains(stderr, "Validation errors:") {
		t.Fatalf("partial bulk: %d %#v %s", code, result, stderr)
	}
	if len(writeTestRows(t, dir, "knowledge")) != 1 {
		t.Fatal("bulk failed dedup")
	}
}
func TestWriteEditAndOutcomePreserveRecordIdentity(t *testing.T) {
	dir := writeTestStore(t)
	row := writeTestFailure("A fixture failure")
	writeTestSeed(t, dir, "knowledge", []Record{row})
	id := Text(row["id"])
	code, result, stderr := writeTestInvoke(t, dir, "", "edit", "knowledge", id, "--resolution", "Create a new temporary directory", "--outcome-status", "partial")
	if code != 0 || Text(Obj(result["record"])["id"]) != id {
		t.Fatalf("edit: %#v %s", result, stderr)
	}
	code, result, stderr = writeTestInvoke(t, dir, "", "outcome", "knowledge", id, "--status", "success", "--duration", "12.5", "--notes", "Verified")
	if code != 0 || Number(result["total_outcomes"]) != 2 {
		t.Fatalf("append: %#v %s", result, stderr)
	}
	code, result, stderr = writeTestInvoke(t, dir, "", "outcome", "knowledge", id)
	if code != 0 || len(AnyList(result["outcomes"])) != 2 {
		t.Fatalf("inspect: %#v %s", result, stderr)
	}
}
func TestWriteOutcomeRejectsTrailingNumberGarbage(t *testing.T) {
	dir := writeTestStore(t)
	row := writeTestFailure("Numeric duration")
	writeTestSeed(t, dir, "knowledge", []Record{row})
	code, _, stderr := writeTestInvoke(t, dir, "", "outcome", "knowledge", Text(row["id"]), "--status", "success", "--duration", "12abc")
	if code == 0 || !strings.Contains(stderr, "--duration must be a non-negative number") {
		t.Fatalf("bad duration accepted: %d %s", code, stderr)
	}
	if len(RecordList(writeTestRows(t, dir, "knowledge")[0]["outcomes"])) != 0 {
		t.Fatal("invalid outcome persisted")
	}
}
func TestWriteArchiveRestoreAndCollisionRefusal(t *testing.T) {
	dir := writeTestStore(t)
	row := writeTestFailure("An obsolete fixture")
	writeTestSeed(t, dir, "knowledge", []Record{row})
	id := Text(row["id"])
	code, _, stderr := writeTestInvoke(t, dir, "", "archive", "knowledge", id, "--reason", "Replaced fixture")
	if code != 0 {
		t.Fatal(stderr)
	}
	if len(writeTestRows(t, dir, "knowledge")) != 0 {
		t.Fatal("archive kept live source")
	}
	path, e := ArchivePath(dir, "knowledge")
	if e != nil {
		t.Fatal(e)
	}
	archived, e := ReadArchive(path, GetRegistry())
	if e != nil || len(archived) != 1 || Text(archived[0]["archive_reason"]) != "manual: Replaced fixture" {
		t.Fatalf("archive metadata: %#v %v", archived, e)
	}
	code, result, stderr := writeTestInvoke(t, dir, "", "restore", id)
	if code != 0 || Text(result["archive_reason"]) != "manual: Replaced fixture" {
		t.Fatalf("restore: %#v %s", result, stderr)
	}
	live := writeTestRows(t, dir, "knowledge")
	if len(live) != 1 || live[0]["status"] != nil || live[0]["archived_at"] != nil {
		t.Fatalf("restore retained archive fields: %#v", live)
	}
	if e = ArchiveRecords(dir, "knowledge", []Record{row}, time.Now(), "manual", GetRegistry()); e != nil {
		t.Fatal(e)
	}
	code, _, stderr = writeTestInvoke(t, dir, "", "restore", id)
	if code == 0 || !strings.Contains(stderr, "already exists") {
		t.Fatal("restore overwrote live collision")
	}
}
func TestWriteMoveRetainsMetadataAndReportsInboundLinks(t *testing.T) {
	dir := writeTestStore(t)
	if e := AddDomain(dir, "target"); e != nil {
		t.Fatal(e)
	}
	if e := AddDomain(dir, "links"); e != nil {
		t.Fatal(e)
	}
	row := writeTestFailure("Move fixture")
	row["tags"] = []string{"verified"}
	writeTestSeed(t, dir, "knowledge", []Record{row})
	id := Text(row["id"])
	link := writeTestFailure("A related failure")
	link["relates_to"] = []string{id}
	writeTestSeed(t, dir, "links", []Record{link})
	code, result, stderr := writeTestInvoke(t, dir, "", "move", "knowledge", id, "target")
	if code != 0 || len(AnyList(result["incomingReferences"])) != 1 {
		t.Fatalf("move: %#v %s", result, stderr)
	}
	if len(writeTestRows(t, dir, "knowledge")) != 0 {
		t.Fatal("source not removed")
	}
	target := writeTestRows(t, dir, "target")
	if len(target) != 1 || Text(target[0]["id"]) != id || !Contains(List(target[0]["tags"]), "verified") {
		t.Fatal("move lost metadata")
	}
}
func TestWriteDeleteBulkResolutionIsAtomic(t *testing.T) {
	dir := writeTestStore(t)
	rows := []Record{writeTestFailure("First"), writeTestFailure("Second"), writeTestFailure("Third")}
	writeTestSeed(t, dir, "knowledge", rows)
	code, _, _ := writeTestInvoke(t, dir, "", "delete", "knowledge", "--records", Text(rows[0]["id"])+",mx-doesnotexist")
	if code == 0 || len(writeTestRows(t, dir, "knowledge")) != 3 {
		t.Fatal("invalid bulk ID partially deleted")
	}
	code, result, stderr := writeTestInvoke(t, dir, "", "delete", "knowledge", "--all-except", Text(rows[1]["id"]))
	if code != 0 || len(AnyList(result["deleted"])) != 2 || Number(result["kept"]) != 1 {
		t.Fatalf("bulk keep: %#v %s", result, stderr)
	}
}
func TestWritePrunePreviewAndSupersessionLadder(t *testing.T) {
	dir := writeTestStore(t)
	old := Record{"id": "mx-aaaaaa", "type": "decision", "title": "Old decision", "rationale": "Prior rationale", "classification": "foundational", "recorded_at": NowISO()}
	newer := Record{"id": "mx-bbbbbb", "type": "decision", "title": "New decision", "rationale": "Current rationale", "classification": "foundational", "recorded_at": NowISO(), "supersedes": []string{"mx-aaaaaa"}}
	writeTestSeed(t, dir, "knowledge", []Record{old, newer})
	code, result, stderr := writeTestInvoke(t, dir, "", "prune", "--dry-run", "--explain")
	if code != 0 || Number(result["totalDemoted"]) != 1 || len(AnyList(result["explanations"])) != 1 {
		t.Fatalf("prune preview: %#v %s", result, stderr)
	}
	if Text(writeTestRows(t, dir, "knowledge")[0]["classification"]) != "foundational" {
		t.Fatal("preview demoted source")
	}
	code, result, stderr = writeTestInvoke(t, dir, "", "prune")
	if code != 0 || Number(result["totalSupersessionDemoted"]) != 1 {
		t.Fatalf("prune: %#v %s", result, stderr)
	}
	if Text(writeTestRows(t, dir, "knowledge")[0]["classification"]) != "tactical" {
		t.Fatal("ladder did not demote one tier")
	}
}
func TestWritePrunePreservesSupersessionCycles(t *testing.T) {
	dir := writeTestStore(t)
	rows := []Record{{"id": "mx-aaaaaa", "type": "convention", "content": "Avoid A because B", "classification": "foundational", "recorded_at": NowISO(), "supersedes": []string{"mx-bbbbbb"}}, {"id": "mx-bbbbbb", "type": "convention", "content": "Avoid B because A", "classification": "foundational", "recorded_at": NowISO(), "supersedes": []string{"mx-aaaaaa"}}}
	writeTestSeed(t, dir, "knowledge", rows)
	code, result, stderr := writeTestInvoke(t, dir, "", "prune", "--aggressive")
	if code != 0 || Number(result["totalPruned"]) != 0 || len(writeTestRows(t, dir, "knowledge")) != 2 {
		t.Fatalf("cycle pruned: %#v %s", result, stderr)
	}
}
func TestWriteCompactManualDryRunFlagDoesNotGuardApply(t *testing.T) {
	dir := writeTestStore(t)
	rows := []Record{writeTestFailure("First compact source"), writeTestFailure("Second compact source")}
	writeTestSeed(t, dir, "knowledge", rows)
	code, result, stderr := writeTestInvoke(t, dir, "", "compact", "knowledge", "--apply", "--records", Text(rows[0]["id"])+","+Text(rows[1]["id"]), "--type", "failure", "--description", "Combined failure", "--resolution", "Combined resolution", "--dry-run")
	if code != 0 || Text(result["action"]) != "applied" {
		t.Fatalf("apply: %#v %s", result, stderr)
	}
	live := writeTestRows(t, dir, "knowledge")
	if len(live) != 1 || Text(live[0]["classification"]) != "foundational" || len(List(live[0]["supersedes"])) != 2 {
		t.Fatalf("replacement: %#v", live)
	}
	path, _ := ArchivePath(dir, "knowledge")
	archived, e := ReadArchive(path, GetRegistry())
	if e != nil || len(archived) != 2 {
		t.Fatalf("compaction archive: %#v %v", archived, e)
	}
}
func TestWriteCompactAutoPreviewLeavesSources(t *testing.T) {
	dir := writeTestStore(t)
	rows := []Record{}
	for _, s := range []string{"One", "Two", "Three", "Four", "Five"} {
		rows = append(rows, writeTestFailure(s))
	}
	writeTestSeed(t, dir, "knowledge", rows)
	code, result, stderr := writeTestInvoke(t, dir, "", "compact", "knowledge", "--auto", "--dry-run")
	if code != 0 || Number(result["wouldCompact"]) != 5 {
		t.Fatalf("auto preview: %#v %s", result, stderr)
	}
	if len(writeTestRows(t, dir, "knowledge")) != 5 {
		t.Fatal("preview modified sources")
	}
	code, result, stderr = writeTestInvoke(t, dir, "", "compact", "knowledge", "--auto", "--yes")
	if code != 0 || Number(result["compacted"]) != 5 || len(writeTestRows(t, dir, "knowledge")) != 1 {
		t.Fatalf("auto apply: %#v %s", result, stderr)
	}
}
func TestWriteDeleteDomainJSONSkipsConfirmation(t *testing.T) {
	dir := writeTestStore(t)
	code, result, stderr := writeTestInvoke(t, dir, "", "delete-domain", "knowledge")
	if code != 0 || !Bool(result["deletedFile"]) {
		t.Fatalf("delete domain: %#v %s", result, stderr)
	}
	cfg, e := ReadConfig(dir)
	if e != nil || writeHasDomain(cfg, "knowledge") {
		t.Fatal("domain remains configured")
	}
	path := filepath.Join(dir, ".luuna", "atlas", "expertise", "knowledge.jsonl")
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("domain file remains")
	}
}
