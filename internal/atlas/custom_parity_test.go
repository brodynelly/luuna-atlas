package atlas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The compatibility fixtures below are deliberately isolated project stores.
// They exercise persisted legacy/custom rows as well as the public command line.
func TestParityCustomDomainRulesAndAliases(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	prepare := parityWritePrepare(t, nil, func(dir string) {
		cfg, _ := ReadConfig(dir)
		cfg["custom_types"] = map[string]any{"release_decision": map[string]any{
			"extends": "decision", "required": []string{"release_tag"},
			"optional": []string{"rollback_plan"}, "aliases": map[string]any{"release_tag": []string{"old_release_name"}},
		}}
		cfg["disabled_types"] = []string{"reference"}
		Obj(cfg["domains"])["knowledge"] = map[string]any{"allowed_types": []string{"decision", "release_decision"}, "required_fields": []string{"owner"}}
		if e := WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
		path, _ := ExpertisePath(dir, "knowledge")
		legacy := `{"id":"mx-abcdef","type":"release_decision","title":"Legacy release","rationale":"Preserve existing records","old_release_name":"v0","owner":"maintainer","classification":"foundational","recorded_at":"2025-01-01T00:00:00.000Z"}` + "\n"
		if e := os.WriteFile(path, []byte(legacy), 0666); e != nil {
			t.Fatal(e)
		}
	})
	steps := []parityStep{
		{Args: []string{"query", "knowledge", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "pattern", "--name", "Forbidden pattern", "--description", "Reject domain mismatch", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "pattern", "--name", "Forbidden pattern", "--description", "Reject domain mismatch"}},
		{Args: []string{"record", "knowledge", "--type", "decision", "--title", "Missing owner", "--rationale", "Require domain ownership", "--json"}},
		{Args: []string{"record", "knowledge", "--type", "decision", "--title", "Missing owner", "--rationale", "Require domain ownership"}},
		{Args: []string{"record", "knowledge", "--type", "release_decision", "--title", "New release", "--rationale", "Custom fields remain supported", "--release-tag", "v1", "--owner", "maintainer", "--rollback-plan", "Restore prior release", "--json"}},
		{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: `[{"type":"decision","title":"Valid owner","rationale":"Domain requirement supplied","owner":"maintainer"},{"type":"decision","title":"Missing owner","rationale":"Must report missing owner"},{"type":"pattern","name":"Wrong type","description":"Must report forbidden type"}]`},
		{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: `[{"type":"release_decision","title":"Import legacy alias","rationale":"Aliases only normalize persisted rows","old_release_name":"v2","owner":"maintainer"}]`},
		{Args: []string{"edit", "knowledge", "mx-abcdef", "--release-tag", "v0.1", "--json"}},
		{Args: []string{"record", "target", "--type", "reference", "--name", "Deprecated reference", "--description", "Disabled types remain writable", "--json"}},
		{Args: []string{"record", "target", "--type", "reference", "--name", "Another deprecated reference", "--description", "Deprecation still warns"}},
		{Args: []string{"record", "target", "--stdin", "--json"}, Input: `[{"type":"reference","name":"Deprecated bulk reference","description":"Bulk emits deprecation metadata"}]`},
		{Args: []string{"record", "knowledge", "--type", "pattern", "--name", "Allowed escape hatch", "--description", "Permit delayed domain config", "--allow-domain-mismatch", "--json"}},
		{Args: []string{"doctor", "--json"}},
	}
	parityExecute(t, steps, prepare)
}

func TestParityCustomDoctorRepairs(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	prepare := parityWritePrepare(t, nil, func(dir string) {
		cfg, _ := ReadConfig(dir)
		cfg["custom_types"] = map[string]any{"incident": map[string]any{"extends": "failure", "required": []string{"severity"}, "aliases": map[string]any{"severity": []string{"old_severity"}}}}
		Obj(cfg["domains"])["target"] = map[string]any{"allowed_types": []string{"decision"}, "required_fields": []string{"owner", "release_tag"}}
		Obj(cfg["domains"])["missing-file"] = map[string]any{}
		cfg["governance"] = map[string]any{"max_entries": 2, "warn_entries": 3, "hard_limit": 6}
		cfg["decay"] = map[string]any{"anchor_validity": map[string]any{"threshold": 2, "grace_days": -1}}
		if e := WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
		rows := []string{
			`{"id":"mx-111111","type":"failure","description":"Missing required resolution","classification":"foundational","recorded_at":"2025-01-01T00:00:00.000Z"}`,
			`{"id":"mx-222222","type":"future_type","statement":"Keep unknown rows","classification":"foundational","recorded_at":"2025-01-01T00:00:00.000Z"}`,
			`{"id":"mx-333333","type":"convention","content":"Avoid shared state because jobs overlap","classification":"foundational","recorded_at":"2025-01-01T00:00:00.000Z","outcome":{"status":"success","duration":10,"notes":"Dropped legacy note"}}`,
			`{"id":"mx-444444","type":"convention","content":"Avoid shared state because jobs overlap","classification":"foundational","recorded_at":"2025-01-01T00:00:00.000Z"}`,
			`{"id":"mx-555555","type":"pattern","name":"Broken anchors","description":"Repair all path locations","files":["missing.go","present.go"],"dir_anchors":["missing-dir","existing-dir"],"evidence":{"file":"missing-evidence.go","commit":"abc123"},"classification":"foundational","recorded_at":"2025-01-01T00:00:00.000Z"}`,
			`{"id":"mx-666666","type":"decision","title":"Expired tactical","rationale":"Old evidence","classification":"tactical","recorded_at":"2025-01-01T00:00:00.000Z"}`,
			`{"id":"mx-777777","type":"incident","description":"Aliased severity","resolution":"Retain migration support","old_severity":"high","classification":"foundational","recorded_at":"2025-01-01T00:00:00.000Z"}`,
		}
		path, _ := ExpertisePath(dir, "knowledge")
		if e := os.WriteFile(path, []byte(strings.Join(rows, "\n")+"\n"), 0666); e != nil {
			t.Fatal(e)
		}
		for path, body := range map[string]string{
			filepath.Join(dir, "present.go"):                                   "package present\n",
			filepath.Join(dir, ".luuna", "atlas", "expertise", "orphan.jsonl"): "",
		} {
			if e := os.WriteFile(path, []byte(body), 0666); e != nil {
				t.Fatal(e)
			}
		}
		if e := os.MkdirAll(filepath.Join(dir, "existing-dir"), 0755); e != nil {
			t.Fatal(e)
		}
	})
	steps := []parityStep{
		{Args: []string{"doctor", "--json"}},
		{Args: []string{"doctor"}},
		{Args: []string{"doctor", "--fix", "--json"}},
		{Args: []string{"doctor", "--json"}},
		{Args: []string{"doctor", "--fix", "--quiet"}},
	}
	parityExecute(t, steps, prepare)
}

func TestParityCustomAuditThresholdsAndCitations(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	rows := []Record{}
	for n, text := range []string{"Avoid shared files because tests race", "The application has a service", "The module exports functions", "The project uses a config file", "Always lock before replacing data", "The README describes setup"} {
		r := Record{"id": fmt.Sprintf("mx-%06x", n+1), "type": "convention", "content": text, "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"}
		if n == 0 {
			r["evidence"] = map[string]any{"missions": "work-abcd", "gh": "#42"}
		}
		if n == 1 {
			r["evidence"] = map[string]any{"missions": "work-missing"}
		}
		if n == 2 {
			r["evidence"] = map[string]any{"commit": "0123456789"}
		}
		if n == 3 {
			r["relates_to"] = []string{"mx-000001"}
		}
		if n == 4 {
			r["classification"] = "tactical"
		}
		rows = append(rows, r)
	}
	prepare := parityWritePrepare(t, rows, func(dir string) {
		cfg, _ := ReadConfig(dir)
		cfg["audit"] = map[string]any{
			"thresholds":     map[string]any{"evidence_coverage": .75, "evidence_coverage_warn": .4, "floater_max": .1, "rule_density_min": .6, "rule_density_warn": .3, "max_records_per_domain": 3, "max_stale": 0},
			"per_domain":     map[string]any{"knowledge": map[string]any{"evidence_coverage": .2, "floater_max": .8, "max_records_per_domain": 5, "max_stale": 1}},
			"ignore_domains": []string{"links"},
		}
		if e := WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
		for domain, records := range map[string][]Record{
			"target": {{"id": "mx-aaaaaa", "type": "decision", "title": "A tracked decision", "rationale": "Use a concrete task", "evidence": map[string]any{"missions": "work-abcd", "linear": "TEAM-123"}, "classification": "foundational", "recorded_at": "2025-03-01T00:00:00.000Z"}, {"id": "mx-bbbbbb", "type": "failure", "description": "Expired failure", "resolution": "A known fix", "evidence": map[string]any{"bead": "bd-a1"}, "classification": "observational", "recorded_at": "2025-04-01T00:00:00.000Z"}},
			"links":  {{"id": "mx-cccccc", "type": "reference", "name": "Ignored reference", "description": "Scope can override configured exclusions", "classification": "foundational", "recorded_at": "2025-05-01T00:00:00.000Z"}},
		} {
			path, _ := ExpertisePath(dir, domain)
			if e := WriteRecords(path, records); e != nil {
				t.Fatal(e)
			}
		}
		missions := filepath.Join(dir, ".luuna", "missions")
		if e := os.MkdirAll(missions, 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(missions, "issues.jsonl"), []byte("# temporary fixture\nmalformed fixture line\n"+`{"id":"work-abcd","status":"closed","title":"A very long local fixture task title that is deliberately longer than fifty five characters"}`+"\n"), 0666); e != nil {
			t.Fatal(e)
		}
	})
	steps := []parityStep{
		{Args: []string{"audit", "--json"}},
		{Args: []string{"audit", "--json", "--suggest"}},
		{Args: []string{"audit", "--ci"}},
		{Args: []string{"audit"}},
		{Args: []string{"audit", "--suggest"}},
		{Args: []string{"audit", "--domain", "knowledge", "--json"}},
		{Args: []string{"audit", "--domain", "links", "--json", "--suggest"}},
		{Args: []string{"audit", "--ignore-domains", "knowledge,links", "--json"}},
		{Args: []string{"audit", "--domain", "missing", "--json"}},
	}
	parityExecute(t, steps, prepare)
}

func TestParityCustomSupersessionCycles(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	rows := []Record{
		{"id": "mx-aaaaaa", "type": "decision", "title": "Cycle one", "rationale": "Cycle members remain current", "supersedes": []string{"mx-bbbbbb"}, "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"id": "mx-bbbbbb", "type": "decision", "title": "Cycle two", "rationale": "Cycle members remain current", "supersedes": []string{"knowledge:mx-aaaaaa"}, "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"id": "mx-cccccc", "type": "decision", "title": "Outside cycle", "rationale": "Normal supersession proceeds", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"id": "mx-dddddd", "type": "decision", "title": "Supersede outside cycle", "rationale": "Normal supersession proceeds", "supersedes": []string{"mx-cccccc"}, "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
	}
	parityExecute(t, []parityStep{
		{Args: []string{"prune", "--dry-run", "--explain", "--json"}},
		{Args: []string{"prune", "--dry-run", "--explain"}},
		{Args: []string{"prune", "--json"}},
		{Args: []string{"prune", "--hard", "--aggressive", "--json"}},
		{Args: []string{"doctor", "--json"}},
	}, parityWritePrepare(t, rows, nil))
}

func TestParityCustomMalformedInput(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	inputs := []string{"", "{", "[", "[1", "[1,", `{"a"`, `{"a":`, `{"a":1`, `{"a":1,`, `"unterminated`, `{a:1}`, `{"a" 1}`, `{"a":1 "b":2}`, `[1 2]`, "tr", "falsex", "nul", "undefined", "NaN", "Infinity", "TRUE", "1x", "01", "-", "1.", "1e", `"a\x"`, `"a\u123x"`, "\"a\n\"", "[1,]", "{} x", "'single'", "}", "]", ":", "@", "[.1]", "[+1]", `{"a":1e}`, `{"a":01}`, `{"a":-x}`}
	steps := []parityStep{}
	for _, input := range inputs {
		steps = append(steps, parityStep{Args: []string{"record", "knowledge", "--stdin", "--json"}, Input: input})
	}
	parityExecute(t, steps, parityWritePrepare(t, nil, nil))
}

func TestParityCustomUnicodeTruncation(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	rows := []Record{
		{"id": "mx-aaaaaa", "type": "pattern", "name": strings.Repeat("a", 30), "description": "First pattern", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
		{"id": "mx-bbbbbb", "type": "pattern", "name": strings.Repeat("🌕", 20), "description": "Second pattern", "classification": "foundational", "recorded_at": "2025-01-01T00:00:00.000Z"},
	}
	parityExecute(t, []parityStep{
		{Args: []string{"record", "knowledge", "--type", "decision", "--description", "A" + strings.Repeat("🌕", 41), "--json"}},
		{Args: []string{"compact", "knowledge", "--auto", "--min-group", "2", "--yes", "--json"}},
	}, parityWritePrepare(t, rows, nil))
}

func TestParityCustomDoctorCorruptRows(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	for name, suffix := range map[string]string{"malformed": "not-json\n", "comment": "# stored comment\n"} {
		t.Run(name, func(t *testing.T) {
			prepare := parityWritePrepare(t, nil, func(dir string) {
				path, _ := ExpertisePath(dir, "knowledge")
				row := `{"id":"mx-abcdef","type":"decision","title":"Preserve data","rationale":"Repair only what the original repairs","classification":"foundational","recorded_at":"2025-01-01T00:00:00.000Z"}` + "\n"
				if e := os.WriteFile(path, []byte(row+suffix), 0666); e != nil {
					t.Fatal(e)
				}
			})
			parityExecute(t, []parityStep{{Args: []string{"doctor", "--json"}}, {Args: []string{"doctor", "--fix", "--json"}}, {Args: []string{"doctor", "--json"}}}, prepare)
		})
	}
}
