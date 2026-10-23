package atlas

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixtureStore(t *testing.T) (string, *Registry) {
	t.Helper()
	cwd := t.TempDir()
	if _, _, e := InitStore(cwd); e != nil {
		t.Fatal(e)
	}
	if e := AddDomain(cwd, "testing"); e != nil {
		t.Fatal(e)
	}
	reg, e := NewRegistry(nil)
	if e != nil {
		t.Fatal(e)
	}
	return cwd, reg
}
func testRecord(content string) Record {
	return Record{"type": "convention", "content": content, "classification": "tactical", "recorded_at": "2026-10-01T12:00:00.000Z"}
}
func TestNativeRecordStorageAndDedup(t *testing.T) {
	cwd, reg := fixtureStore(t)
	path, _ := ExpertisePath(cwd, "testing")
	r := testRecord("Avoid shared files because writers race")
	if e := WithFileLock(path, func() error { return AppendRecord(path, r, reg) }); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(Text(r["id"]), "mx-") || len(Text(r["id"])) != 9 {
		t.Fatalf("invalid ID: %v", r)
	}
	rows, e := ReadRecords(path, reg, false)
	if e != nil || len(rows) != 1 {
		t.Fatalf("read %v %v", rows, e)
	}
	candidate := testRecord("avoid SHARED files, because writers race.")
	if FindDuplicate(rows, candidate, reg) != 0 {
		t.Fatal("normalized duplicate missed")
	}
	if idx, e := ResolveRecordID(rows, Text(r["id"])[3:6]); e != nil || idx != 0 {
		t.Fatalf("prefix resolution %d %v", idx, e)
	}
	if e = WriteRecords(path, []Record{}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	if len(b) != 0 {
		t.Fatal("empty rewrite must have no newline")
	}
}
func TestNativeLocksProtectConcurrentReadModifyWrite(t *testing.T) {
	cwd, reg := fixtureStore(t)
	path, _ := ExpertisePath(cwd, "testing")
	var wg sync.WaitGroup
	errors := make(chan error, 16)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errors <- WithFileLock(path, func() error {
				rows, e := ReadRecords(path, reg, false)
				if e != nil {
					return e
				}
				rows = append(rows, testRecord(strings.Repeat("x", i+1)))
				return WriteRecords(path, rows)
			})
		}(i)
	}
	wg.Wait()
	close(errors)
	for e := range errors {
		if e != nil {
			t.Fatal(e)
		}
	}
	rows, e := ReadRecords(path, reg, false)
	if e != nil || len(rows) != 12 {
		t.Fatalf("concurrency lost writes: %d %v", len(rows), e)
	}
	if exists(path + ".lock") {
		t.Fatal("lock leaked")
	}
	temps, _ := filepath.Glob(path + ".tmp.*")
	if len(temps) != 0 {
		t.Fatal("temporary files leaked")
	}
}
func TestNativeStaleLockAndCleanup(t *testing.T) {
	cwd, _ := fixtureStore(t)
	path, _ := ExpertisePath(cwd, "testing")
	lock := path + ".lock"
	if e := os.WriteFile(lock, nil, 0666); e != nil {
		t.Fatal(e)
	}
	at := time.Now().Add(-time.Minute)
	_ = os.Chtimes(lock, at, at)
	if e := WithFileLock(path, func() error { return nil }); e != nil {
		t.Fatal(e)
	}
	if exists(lock) {
		t.Fatal("stale lock leaked")
	}
}
func TestNativeLegacyOutcomeAndUnknownTypeReading(t *testing.T) {
	cwd, reg := fixtureStore(t)
	path, _ := ExpertisePath(cwd, "testing")
	raw := `{"type":"convention","content":"x","classification":"tactical","recorded_at":"now","outcome":{"status":"success","notes":"discarded"}}` + "\n"
	_ = os.WriteFile(path, []byte(raw), 0666)
	rows, e := ReadRecords(path, reg, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, exists := rows[0]["outcome"]; exists {
		t.Fatal("legacy key retained")
	}
	if _, exists := Obj(AnyList(rows[0]["outcomes"])[0])["notes"]; exists {
		t.Fatal("legacy normalization changed")
	}
	_ = os.WriteFile(path, []byte(`{"type":"unregistered","id":"mx-abcdef"}`), 0666)
	if _, e = ReadRecords(path, reg, false); e == nil {
		t.Fatal("unknown type accepted")
	}
	if rows, e = ReadRecords(path, reg, true); e != nil || len(rows) != 1 {
		t.Fatal("unknown escape hatch failed")
	}
}
func TestNativeConfigDomainOrderAndDefaults(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, ".luuna", "atlas")
	_ = os.MkdirAll(dir, 0777)
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("domains:\n  zeta: {}\n  alpha: {}\ngovernance:\n  max_entries: 7\n"), 0666)
	cfg, e := ReadConfig(cwd)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Join(DomainNames(cfg), ",") != "zeta,alpha" {
		t.Fatalf("domain order changed: %v", DomainNames(cfg))
	}
	if Number(ConfigValue(cfg, "governance", "hard_limit")) != 200 {
		t.Fatal("partial defaults lost")
	}
	if e = WriteConfig(cwd, cfg); e != nil {
		t.Fatal(e)
	}
	cfg, e = ReadConfig(cwd)
	if e != nil || strings.Join(DomainNames(cfg), ",") != "zeta,alpha" {
		t.Fatal("write reordered domains")
	}
}
func TestNativeLegacyLayoutMigration(t *testing.T) {
	cwd := t.TempDir()
	old := filepath.Join(cwd, ".mulch")
	_ = os.MkdirAll(filepath.Join(old, "expertise"), 0777)
	_ = os.WriteFile(filepath.Join(old, "mulch.config.yaml"), []byte("version: '1'\ndomains: {}\n"), 0666)
	_ = os.WriteFile(filepath.Join(cwd, ".gitattributes"), []byte(" /.mulch/expertise/*.jsonl merge=union\n"), 0666)
	cfg, e := ReadConfig(cwd)
	if e != nil || Text(cfg["version"]) != "1" {
		t.Fatalf("migration failed %v", e)
	}
	if exists(old) {
		t.Fatal("legacy store retained")
	}
	attrs, _ := os.ReadFile(filepath.Join(cwd, ".gitattributes"))
	if string(attrs) != " /.luuna/atlas/expertise/*.jsonl merge=union\n" {
		t.Fatalf("migration rewrite: %s", attrs)
	}
}

func TestNativeYAMLScalarFormatting(t *testing.T) {
	value := map[string]any{"version": "1", "boolean": "true", "empty": "", "yes": "yes", "multi": "one\ntwo"}
	rememberOrder(value, []string{"version", "boolean", "empty", "yes", "multi"})
	got, e := MarshalYAML(value)
	if e != nil {
		t.Fatal(e)
	}
	want := "version: '1'\nboolean: 'true'\nempty: ''\n'yes': 'yes'\nmulti: |-\n  one\n  two\n"
	if string(got) != want {
		t.Fatalf("YAML reference formatting mismatch\n%s", got)
	}
}

func TestNativeObjectKeysFollowJavaScriptIndexOrder(t *testing.T) {
	value, err := ParseJSON([]byte(`{"10":{},"2":{},"zebra":{},"0":{},"01":{},"4294967295":{},"4294967294":{},"10":{"updated":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"0", "2", "10", "4294967294", "zebra", "01", "4294967295"}
	if got := ObjectKeys(value); !reflect.DeepEqual(got, want) {
		t.Fatalf("Object.keys order: %v, want %v", got, want)
	}
	encoded, err := MarshalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(encoded), `"10":`) != 1 {
		t.Fatalf("duplicate input properties were emitted more than once: %s", encoded)
	}
}

func TestNativeCloneRetainsNestedJSONOrder(t *testing.T) {
	const body = `{"type":"convention","evidence":{"missions":"atlas-abcd","file":"src/a.go","commit":"abc"},"outcomes":[{"notes":"verified","status":"success"}],"content":"Preserve object ordering"}`
	value, err := ParseJSON([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	cloned := CloneRecord(Record(Obj(value)))
	encoded, err := MarshalJSON(cloned)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != body {
		t.Fatalf("nested order changed: %s", encoded)
	}
}

func TestNativeFullUnicodeLowercase(t *testing.T) {
	for input, want := range map[string]string{"ΟΣ": "ος", "ΟΣΑ": "οσα", "İstanbul": "i̇stanbul", "AİB": "ai̇b"} {
		if got := JSLower(input); got != want {
			t.Fatalf("lower(%q)=%q, want %q", input, got, want)
		}
	}
	if NormalizeText("ΟΣ") != NormalizeText("ος") {
		t.Fatal("contextual final sigma changed duplicate equivalence")
	}
	if NormalizeText("İstanbul") != "i stanbul" {
		t.Fatalf("combining-dot token boundary changed: %q", NormalizeText("İstanbul"))
	}
	if got := Tokenize("İSTANBUL"); !reflect.DeepEqual(got, []string{"i", "stanbul"}) {
		t.Fatalf("ASCII tokenizer must receive full lowercase expansion: %v", got)
	}
}
