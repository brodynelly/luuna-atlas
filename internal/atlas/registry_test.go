package atlas

import "testing"

func TestNativeCustomRegistryInheritanceAliasesAndCompatibility(t *testing.T) {
	cfg := Config{"custom_types": map[string]any{"incident": map[string]any{"extends": "failure", "required": []any{"severity"}, "summary": "[{severity}] {description}", "aliases": map[string]any{"severity": []any{"old_severity"}}}}}
	reg, e := NewRegistry(cfg)
	if e != nil {
		t.Fatal(e)
	}
	d := reg.Get("incident")
	if !Contains(d.Required, "description") || !Contains(d.Required, "resolution") || !Contains(d.Required, "severity") {
		t.Fatalf("inheritance failed: %v", d.Required)
	}
	r := Record{"type": "incident", "classification": "tactical", "recorded_at": "now", "description": "x", "resolution": "y", "old_severity": "high"}
	ApplyAliases(r, d.Aliases)
	if r["severity"] != "high" {
		t.Fatal("alias not applied")
	}
	if issues := ValidateRecord(r, reg); len(issues) != 0 {
		t.Fatalf("custom record invalid: %v", issues)
	}
	if len(SearchRecords([]Record{r}, "high", 0)) != 0 {
		t.Fatal("custom prose search compatibility changed")
	}
}
func TestNativeContentHashIDCompatibility(t *testing.T) {
	reg, e := NewRegistry(Config{"custom_types": map[string]any{"hypothesis": map[string]any{"required": []any{"statement"}, "dedup_key": "content_hash", "summary": "{statement}"}}})
	if e != nil {
		t.Fatal(e)
	}
	a := Record{"type": "hypothesis", "statement": "one"}
	b := Record{"type": "hypothesis", "statement": "two"}
	if GenerateRecordID(a, reg) != GenerateRecordID(b, reg) {
		t.Fatal("existing content_hash ID behavior changed")
	}
	if FindDuplicate([]Record{a}, b, reg) >= 0 {
		t.Fatal("body dedup conflated distinct statements")
	}
}
func TestNativeRecordSchemaAndDomainRules(t *testing.T) {
	reg := BuiltinRegistry()
	r := testRecord("Avoid collisions because concurrency")
	if issues := ValidateRecord(r, reg); len(issues) != 0 {
		t.Fatal(issues)
	}
	r["unexpected"] = true
	if len(ValidateRecord(r, reg)) == 0 {
		t.Fatal("additional field accepted")
	}
	cfg := Config{"domains": map[string]any{"testing": map[string]any{"allowed_types": []any{"failure"}, "required_fields": []any{"owner"}}}}
	violations := ValidateDomainRecord(r, "testing", cfg)
	if len(violations) != 2 {
		t.Fatalf("expected domain violations: %v", violations)
	}
}
func TestNativeJSONSchemaConfigPrimitives(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []any{"count"}, "properties": map[string]any{"count": map[string]any{"type": "integer", "minimum": 1}}, "additionalProperties": false}
	if issues := ValidateJSONSchema(map[string]any{"count": 2}, schema); len(issues) != 0 {
		t.Fatal(issues)
	}
	issues := ValidateJSONSchema(map[string]any{"count": 0, "other": true}, schema)
	if len(issues) != 2 {
		t.Fatalf("schema issues %v", issues)
	}
}

func TestNativeAJVOneOfFailureContract(t *testing.T) {
	reg := BuiltinRegistry()
	r := Record{"type": "pattern", "name": "p", "description": 17, "classification": "tactical", "recorded_at": "now"}
	errors := ValidateRecord(r, reg)
	want := []string{" must have required property 'content'", "/description must be string", " must have required property 'resolution'", " must have required property 'title'", "/type must be equal to constant", "/type must be equal to constant", " must match exactly one schema in oneOf"}
	if len(errors) != len(want) {
		t.Fatalf("errors %v", errors)
	}
	for i, e := range errors {
		if e.Path+" "+e.Message != want[i] {
			t.Fatalf("error %d: %s %s", i, e.Path, e.Message)
		}
	}
}
