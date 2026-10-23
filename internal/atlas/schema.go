package atlas

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
)

var idRE = regexp.MustCompile(`^mx-[0-9a-f]{4,8}$`)
var linkRE = regexp.MustCompile(`^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$`)
var baseRecordFields = []string{"id", "type", "classification", "recorded_at", "evidence", "tags", "relates_to", "supersedes", "outcomes", "dir_anchors", "supersession_demoted_at", "anchor_decay_demoted_at", "owner", "status"}

func ValidateRecord(record Record, registry *Registry) []ValidationIssue {
	return ValidateRecordValue(RecordValue(record), registry)
}

func ValidateRecordValue(record any, registry *Registry) []ValidationIssue {
	if issues := validateSchemaAt(record, map[string]any{"type": "object"}, nil, ""); len(issues) > 0 {
		return issues
	}
	if registry == nil {
		registry = GetRegistry()
	}
	issues := []ValidationIssue{}
	valid := 0
	for _, name := range registry.Order {
		schema := recordVariantSchema(registry.Types[name])
		errors := validateSchemaAt(record, schema, schema, "")
		if len(errors) == 0 {
			valid++
		} else {
			issues = append(issues, errors[0])
		}
	}
	if valid == 1 {
		return []ValidationIssue{}
	}
	return append(issues, ValidationIssue{Path: "", Keyword: "oneOf", Message: "must match exactly one schema in oneOf", Params: map[string]any{"passingSchemas": nil}})
}
func recordVariantSchema(def *TypeDefinition) map[string]any {
	str := func() map[string]any { return map[string]any{"type": "string"} }
	enum := func(values ...string) map[string]any { return map[string]any{"type": "string", "enum": values} }
	arr := func(item map[string]any) map[string]any { return map[string]any{"type": "array", "items": item} }
	evProps := map[string]any{}
	evKeys := []string{"commit", "date", "issue", "file", "bead", "missions", LegacyEvidenceKey, "gh", "linear"}
	for _, key := range evKeys {
		evProps[key] = str()
	}
	rememberOrder(evProps, evKeys)
	outcomeProps := map[string]any{"status": enum("success", "failure", "partial"), "duration": map[string]any{"type": "number"}, "test_results": str(), "agent": str(), "notes": str(), "recorded_at": str()}
	rememberOrder(outcomeProps, []string{"status", "duration", "test_results", "agent", "notes", "recorded_at"})
	link := map[string]any{"type": "string", "pattern": `^([a-z0-9-]+:)?mx-[0-9a-f]{4,8}$`}
	props := map[string]any{"id": map[string]any{"type": "string", "pattern": `^mx-[0-9a-f]{4,8}$`}, "classification": enum("foundational", "tactical", "observational"), "recorded_at": str(), "evidence": map[string]any{"type": "object", "properties": evProps, "additionalProperties": false}, "tags": arr(str()), "relates_to": arr(link), "supersedes": arr(link), "outcomes": arr(map[string]any{"type": "object", "properties": outcomeProps, "required": []string{"status"}, "additionalProperties": false}), "dir_anchors": arr(str()), "supersession_demoted_at": str(), "anchor_decay_demoted_at": str(), "owner": str(), "status": enum("draft", "active", "deprecated"), "type": map[string]any{"type": "string", "const": def.Name}}
	order := []string{"id", "classification", "recorded_at", "evidence", "tags", "relates_to", "supersedes", "outcomes", "dir_anchors", "supersession_demoted_at", "anchor_decay_demoted_at", "owner", "status", "type"}
	if !def.Builtin {
		order = []string{"id", "type", "classification", "recorded_at", "evidence", "tags", "relates_to", "supersedes", "outcomes", "dir_anchors", "supersession_demoted_at", "anchor_decay_demoted_at", "owner", "status"}
	}
	for _, field := range append(append([]string{}, def.Required...), def.Optional...) {
		if def.ExtractsFiles && field == def.FilesField {
			props[field] = arr(str())
		} else {
			props[field] = str()
		}
		if !Contains(order, field) {
			order = append(order, field)
		}
	}
	rememberOrder(props, order)
	return map[string]any{"type": "object", "properties": props, "required": append(append([]string{"type"}, def.Required...), "classification", "recorded_at"), "additionalProperties": false}
}

func isNumeric(v any) bool {
	switch number := v.(type) {
	case float64:
		return !math.IsNaN(number) && !math.IsInf(number, 0)
	case float32:
		return !math.IsNaN(float64(number)) && !math.IsInf(float64(number), 0)
	case int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
		return true
	}
	return false
}
func GetAllowedTypes(cfg Config, domain string) []string {
	return List(Obj(Obj(cfg["domains"])[domain])["allowed_types"])
}
func GetRequiredFields(cfg Config, domain string) []string {
	return List(Obj(Obj(cfg["domains"])[domain])["required_fields"])
}
func FindMissingDomainFields(record Record, fields []string) []string {
	out := []string{}
	for _, f := range fields {
		v, ok := record[f]
		if !ok || v == nil || v == "" {
			out = append(out, f)
			continue
		}
		kind := reflect.TypeOf(v).Kind()
		if kind == reflect.Slice && reflect.ValueOf(v).Len() == 0 {
			out = append(out, f)
		}
	}
	return out
}
func ValidateDomainRecord(record Record, domain string, cfg Config) []DomainViolation {
	out := []DomainViolation{}
	allowed := GetAllowedTypes(cfg, domain)
	if len(allowed) > 0 && !Contains(allowed, Text(record["type"])) {
		out = append(out, DomainViolation{Kind: "allowed_types", Field: "type", Message: fmt.Sprintf("type %q is not allowed in domain %q (allowed: %s)", Text(record["type"]), domain, strings.Join(allowed, ", "))})
	}
	for _, f := range FindMissingDomainFields(record, GetRequiredFields(cfg, domain)) {
		out = append(out, DomainViolation{Kind: "required_fields", Field: f, Message: fmt.Sprintf("missing required field %q in domain %q", f, domain)})
	}
	return out
}

type IncompatibleRequiredField struct {
	Field        string
	AllowedTypes []string
}

func FindIncompatibleRequiredFields(cfg Config, domain string, registry *Registry) []IncompatibleRequiredField {
	out := []IncompatibleRequiredField{}
	allowed := GetAllowedTypes(cfg, domain)
	if len(allowed) == 0 {
		allowed = registry.Order
	}
	for _, f := range GetRequiredFields(cfg, domain) {
		if Contains(baseRecordFields, f) && f != "anchor_decay_demoted_at" {
			continue
		}
		accepted := false
		for _, typeName := range allowed {
			d := registry.Get(typeName)
			if d != nil && (Contains(d.Required, f) || Contains(d.Optional, f)) {
				accepted = true
				break
			}
		}
		if !accepted {
			out = append(out, IncompatibleRequiredField{Field: f, AllowedTypes: append([]string{}, allowed...)})
		}
	}
	return out
}

func ValidateJSONSchema(value any, schema map[string]any) []ValidationIssue {
	return validateSchemaAt(value, schema, schema, "")
}
func validateSchemaAt(value any, schema, root map[string]any, path string) []ValidationIssue {
	issues := []ValidationIssue{}
	add := func(keyword, message string, params map[string]any) {
		issues = append(issues, ValidationIssue{Path: path, Message: message, Keyword: keyword, Params: params})
	}
	if ref := Text(schema["$ref"]); strings.HasPrefix(ref, "#/") {
		var v any = root
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			v = Obj(v)[strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")]
		}
		return validateSchemaAt(value, Obj(v), root, path)
	}
	if options := AnyList(schema["oneOf"]); len(options) > 0 {
		all := []ValidationIssue{}
		valid := 0
		for _, option := range options {
			errs := validateSchemaAt(value, Obj(option), root, path)
			if len(errs) == 0 {
				valid++
			} else {
				all = append(all, errs...)
			}
		}
		if valid != 1 {
			issues = append(issues, all...)
			add("oneOf", "must match exactly one schema in oneOf", nil)
		}
		return issues
	}
	if options := AnyList(schema["anyOf"]); len(options) > 0 {
		all := []ValidationIssue{}
		valid := false
		for _, option := range options {
			errs := validateSchemaAt(value, Obj(option), root, path)
			if len(errs) == 0 {
				valid = true
				break
			}
			all = append(all, errs...)
		}
		if !valid {
			issues = append(issues, all...)
			add("anyOf", "must match a schema in anyOf", nil)
		}
		return issues
	}
	kind := Text(schema["type"])
	validType := true
	switch kind {
	case "object":
		switch value.(type) {
		case map[string]any, Record, Config:
		default:
			validType = false
		}
	case "array":
		validType = value != nil && reflect.TypeOf(value).Kind() == reflect.Slice
	case "string":
		_, validType = value.(string)
	case "number":
		validType = isNumeric(value)
	case "integer":
		validType = isNumeric(value) && Number(value) == math.Trunc(Number(value))
	case "boolean":
		_, validType = value.(bool)
	case "null":
		validType = value == nil
	}
	if !validType {
		add("type", "must be "+kind, map[string]any{"type": kind})
		return issues
	}
	if enum := AnyList(schema["enum"]); len(enum) > 0 {
		valid := false
		for _, choice := range enum {
			if reflect.DeepEqual(choice, value) {
				valid = true
				break
			}
		}
		if !valid {
			add("enum", "must be equal to one of the allowed values", map[string]any{"allowedValues": enum})
		}
	}
	if constant, ok := schema["const"]; ok && !reflect.DeepEqual(constant, value) {
		add("const", "must be equal to constant", map[string]any{"allowedValue": constant})
	}
	if kind == "object" {
		object := Obj(value)
		for _, required := range List(schema["required"]) {
			if _, ok := object[required]; !ok {
				add("required", fmt.Sprintf("must have required property '%s'", required), map[string]any{"missingProperty": required})
			}
		}
		properties := Obj(schema["properties"])
		for _, name := range ObjectKeys(object) {
			if _, known := properties[name]; !known {
				switch additional := schema["additionalProperties"].(type) {
				case bool:
					if !additional {
						add("additionalProperties", "must NOT have additional properties", map[string]any{"additionalProperty": name})
					}
				case map[string]any:
					escaped := strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
					issues = append(issues, validateSchemaAt(object[name], additional, root, path+"/"+escaped)...)
				}
			}
		}
		for _, name := range ObjectKeys(properties) {
			if value, exists := object[name]; exists {
				escaped := strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
				issues = append(issues, validateSchemaAt(value, Obj(properties[name]), root, path+"/"+escaped)...)
			}
		}
	}
	if kind == "array" {
		items := AnyList(value)
		if min, ok := schema["minItems"]; ok && len(items) < int(Number(min)) {
			add("minItems", fmt.Sprintf("must NOT have fewer than %s items", Text(min)), map[string]any{"limit": min})
		}
		if Bool(schema["uniqueItems"]) {
			for i := 0; i < len(items); i++ {
				for j := i + 1; j < len(items); j++ {
					if reflect.DeepEqual(items[i], items[j]) {
						add("uniqueItems", fmt.Sprintf("must NOT have duplicate items (items ## %d and %d are identical)", i, j), nil)
					}
				}
			}
		}
		if itemSchema, ok := schema["items"].(map[string]any); ok {
			for i, item := range items {
				issues = append(issues, validateSchemaAt(item, itemSchema, root, fmt.Sprintf("%s/%d", path, i))...)
			}
		}
	}
	if kind == "string" {
		s := Text(value)
		if pattern := Text(schema["pattern"]); pattern != "" {
			if re, e := regexp.Compile(pattern); e == nil && !re.MatchString(s) {
				add("pattern", fmt.Sprintf("must match pattern %q", pattern), map[string]any{"pattern": pattern})
			}
		}
		if min, ok := schema["minLength"]; ok && len([]rune(s)) < int(Number(min)) {
			add("minLength", fmt.Sprintf("must NOT have fewer than %s characters", Text(min)), map[string]any{"limit": min})
		}
	}
	if kind == "number" || kind == "integer" {
		v := Number(value)
		if min, ok := schema["minimum"]; ok && v < Number(min) {
			add("minimum", "must be >= "+Text(min), map[string]any{"comparison": ">=", "limit": min})
		}
		if max, ok := schema["maximum"]; ok && v > Number(max) {
			add("maximum", "must be <= "+Text(max), map[string]any{"comparison": "<=", "limit": max})
		}
	}
	return issues
}
