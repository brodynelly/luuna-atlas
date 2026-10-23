package atlas

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFormatReferenceFixture(t *testing.T) {
	raw, e := ParseJSON([]byte(formatFixtureJSON))
	if e != nil {
		t.Fatal(e)
	}
	records := RecordList(raw)
	var expected map[string]any
	if e = json.Unmarshal([]byte(formatExpectedJSON), &expected); e != nil {
		t.Fatal(e)
	}
	oldNow := formatNow
	formatNow = func() time.Time { return time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { formatNow = oldNow })
	reg := BuiltinRegistry()
	updated := time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		key, format string
		full        bool
	}{{"markdown_compact", "compact", false}, {"markdown_summary", "markdown", false}, {"markdown_full", "markdown", true}, {"xml", "xml", false}, {"plain", "plain", false}} {
		t.Run(tc.key, func(t *testing.T) {
			got := FormatDomain("test", records, &updated, reg, FormatOptions{Format: tc.format, Full: tc.full})
			want := Text(expected[tc.key])
			if got != want {
				t.Fatalf("format differs\nGOT:\n%s\nWANT:\n%s", got, want)
			}
		})
	}
	for index, r := range records {
		if got, want := RecordSummary(r, reg), List(expected["summaries"])[index]; got != want {
			t.Fatalf("summary %d got %q want %q", index, got, want)
		}
	}
}
func TestFormatCustomTypesAndAnnotations(t *testing.T) {
	cfg := Config{"domains": map[string]any{}, "custom_types": map[string]any{"incident": map[string]any{"extends": "failure", "required": []string{"severity"}, "summary": "[{severity}] {description}"}}}
	reg, e := NewRegistry(cfg)
	if e != nil {
		t.Fatal(e)
	}
	record := Record{"id": "mx-abc123", "type": "incident", "classification": "tactical", "recorded_at": "2026-01-01", "description": "worker failed <now>", "resolution": "restart & retry", "severity": "high", "dir_anchors": []string{"src/worker"}}
	got := FormatDomain("ops", []Record{record}, nil, reg, FormatOptions{Format: "markdown", Full: true, Annotations: map[string]string{"mx-abc123": "why: file match (src/worker)"}})
	for _, part := range []string{"### Known Failures", "**[high] worker failed <now>**", "severity: high", "why: file match"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %q in %s", part, got)
		}
	}
	xml := FormatDomain("ops", []Record{record}, nil, reg, FormatOptions{Format: "xml"})
	if !strings.Contains(xml, "worker failed &lt;now&gt;") || !strings.Contains(xml, "restart &amp; retry") {
		t.Fatal(xml)
	}
	plain := FormatDomain("ops", []Record{record}, nil, reg, FormatOptions{Format: "plain"})
	if plain != "[ops] 1 records" {
		t.Fatalf("plain custom omission changed: %q", plain)
	}
}
func TestSessionCloseAndWrapperContracts(t *testing.T) {
	cfg := Config{"prime": map[string]any{"session_close": map[string]any{"style": "none"}}}
	if SessionEndReminder("markdown", cfg) != "" {
		t.Fatal("none style")
	}
	cfg = Config{"prime": map[string]any{"session_close": map[string]any{"style": "none", "custom": "verbatim <footer>"}}}
	if SessionEndReminder("xml", cfg) != "verbatim <footer>" {
		t.Fatal("custom must override")
	}
	if FormatPrime([]string{"alpha", "beta"}, "plain") != "alpha\n\nbeta" {
		t.Fatal("plain wrapper")
	}
	if FormatPrime([]string{"alpha", "beta"}, "xml") != "<expertise>\nalpha\nbeta\n</expertise>" {
		t.Fatal("xml wrapper")
	}
	if !strings.Contains(FormatPrime(nil, "compact"), "No expertise recorded yet.") {
		t.Fatal("compact empty")
	}
}
func TestColorCompatibility(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	if Style("green", "ok") != "ok" {
		t.Fatal("force off")
	}
	t.Setenv("FORCE_COLOR", "1")
	if Style("green", "ok") != "\x1b[32mok\x1b[39m" {
		t.Fatal("basic ANSI")
	}
	t.Setenv("FORCE_COLOR", "3")
	if Style("brand", "soil") != "\x1b[38;2;139;90;43msoil\x1b[39m" {
		t.Fatal("truecolor")
	}
}

func TestXMLFormattingPreservesCallerArrays(t *testing.T) {
	reg, _ := NewRegistry(nil)
	files, tags := []string{"src/a&b.ts"}, []string{"<tag>"}
	r := Record{"id": "mx-abc123", "type": "pattern", "name": "Example", "description": "Body", "classification": "foundational", "files": files, "tags": tags}
	first := FormatDomain("code", []Record{r}, nil, reg, FormatOptions{Format: "xml"})
	second := FormatDomain("code", []Record{r}, nil, reg, FormatOptions{Format: "xml"})
	if first != second || files[0] != "src/a&b.ts" || tags[0] != "<tag>" {
		t.Fatalf("formatting mutated arrays: files=%v tags=%v\nfirst=%s\nsecond=%s", files, tags, first, second)
	}
}
