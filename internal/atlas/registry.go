package atlas

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var builtinNames = []string{"convention", "pattern", "failure", "decision", "reference", "guide"}
var customBaseFields = []string{"id", "type", "classification", "recorded_at", "evidence", "tags", "relates_to", "supersedes", "outcomes"}
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var templateTokenRE = regexp.MustCompile(`(?i)\{\{([a-z_][a-z0-9_]*)\}\}|\{([a-z_][a-z0-9_]*)\}`)
var registryState struct {
	sync.RWMutex
	registry *Registry
}

func BuiltinRegistry() *Registry { r, _ := NewRegistry(nil); return r }
func GetRegistry() *Registry {
	registryState.RLock()
	r := registryState.registry
	registryState.RUnlock()
	if r != nil {
		return r
	}
	return BuiltinRegistry()
}
func (r *Registry) Get(name string) *TypeDefinition {
	if r == nil {
		return nil
	}
	return r.Types[name]
}
func (r *Registry) Names() []string {
	if r == nil {
		return []string{}
	}
	return append([]string{}, r.Order...)
}
func NewRegistry(cfg Config) (*Registry, error) {
	r := &Registry{Types: map[string]*TypeDefinition{}, Order: append([]string{}, builtinNames...), Disabled: List(cfg["disabled_types"])}
	r.Types["convention"] = &TypeDefinition{Name: "convention", Required: []string{"content"}, Optional: []string{}, DedupKey: "content", IDKey: "content", Summary: "{content}", SectionTitle: "Conventions", Compact: "concat", FilesField: "files", Builtin: true}
	r.Types["pattern"] = &TypeDefinition{Name: "pattern", Required: []string{"name", "description"}, Optional: []string{"files"}, DedupKey: "name", IDKey: "name", Summary: "{name}", SectionTitle: "Patterns", Compact: "concat", FilesField: "files", ExtractsFiles: true, Builtin: true}
	r.Types["failure"] = &TypeDefinition{Name: "failure", Required: []string{"description", "resolution"}, Optional: []string{}, DedupKey: "description", IDKey: "description", Summary: "{description}", SectionTitle: "Known Failures", Compact: "concat", FilesField: "files", Builtin: true}
	r.Types["decision"] = &TypeDefinition{Name: "decision", Required: []string{"title", "rationale"}, Optional: []string{"date"}, DedupKey: "title", IDKey: "title", Summary: "{title}", SectionTitle: "Decisions", Compact: "concat", FilesField: "files", Builtin: true}
	r.Types["reference"] = &TypeDefinition{Name: "reference", Required: []string{"name", "description"}, Optional: []string{"files"}, DedupKey: "name", IDKey: "name", Summary: "{name}", SectionTitle: "References", Compact: "concat", FilesField: "files", ExtractsFiles: true, Builtin: true}
	r.Types["guide"] = &TypeDefinition{Name: "guide", Required: []string{"name", "description"}, Optional: []string{}, DedupKey: "name", IDKey: "name", Summary: "{name}", SectionTitle: "Guides", Compact: "concat", FilesField: "files", Builtin: true}
	custom := Obj(cfg["custom_types"])
	for _, name := range ObjectKeys(custom) {
		c := Obj(custom[name])
		parentName := Text(c["extends"])
		if parentName != "" && Contains(r.Disabled, parentName) {
			return nil, fmt.Errorf("Custom type \"%s\" extends \"%s\", which is in disabled_types. Re-enable the parent or remove the extends clause.", name, parentName)
		}
		def, e := buildCustomType(name, c, r)
		if e != nil {
			return nil, e
		}
		r.Types[name] = def
		r.Order = append(r.Order, name)
	}
	for _, name := range r.Disabled {
		def := r.Types[name]
		if def == nil {
			return nil, fmt.Errorf("disabled_types references unregistered type \"%s\". Declare it under custom_types or remove it from disabled_types.", name)
		}
		def.Disabled = true
	}
	registryState.Lock()
	registryState.registry = r
	registryState.Unlock()
	return r, nil
}
func nameViolation(s string) string {
	switch {
	case s == "":
		return "must not be empty"
	case strings.Contains(s, "-"):
		return "contains a hyphen"
	case s[0] >= '0' && s[0] <= '9':
		return "starts with a digit"
	case s[0] < 'a' || s[0] > 'z':
		return "must start with a lowercase letter"
	}
	if regexp.MustCompile(`[A-Z]`).MatchString(s) {
		return "contains uppercase letters"
	}
	return "contains a character outside [a-z0-9_]"
}

const namingRule = "lowercase letters, digits, and underscores; must start with a letter; no hyphens"

func buildCustomType(name string, c map[string]any, r *Registry) (*TypeDefinition, error) {
	if !nameRE.MatchString(name) {
		return nil, fmt.Errorf("Invalid custom_types name \"%s\" (%s). Allowed: %s.", name, nameViolation(name), namingRule)
	}
	if Contains(builtinNames, name) {
		return nil, fmt.Errorf("Custom type \"%s\" shadows a built-in record type. Reserved names: %s.", name, strings.Join(builtinNames, ", "))
	}
	var parent *TypeDefinition
	parentName := Text(c["extends"])
	if value, exists := c["extends"]; exists {
		if _, ok := value.(string); !ok || parentName == "" {
			return nil, fmt.Errorf("Custom type \"%s\" extends must be a non-empty string.", name)
		}
		if !Contains(builtinNames, parentName) {
			return nil, fmt.Errorf("Custom type \"%s\" extends \"%s\" must be a built-in type. Built-ins: %s. Custom-from-custom inheritance is not supported in v1.", name, parentName, strings.Join(builtinNames, ", "))
		}
		parent = r.Types[parentName]
	}
	req, opt := List(c["required"]), List(c["optional"])
	if parent == nil && len(AnyList(c["required"])) == 0 {
		return nil, fmt.Errorf("Custom type \"%s\" must declare a non-empty \"required\" array (or set \"extends\" to inherit from a built-in).", name)
	}
	merged := []string{}
	if parent != nil {
		merged = append(append(merged, parent.Required...), parent.Optional...)
	}
	seen := map[string]bool{}
	for _, kind := range []string{"required", "optional"} {
		for _, value := range AnyList(c[kind]) {
			field, ok := value.(string)
			if !ok || field == "" {
				encoded, _ := MarshalJSON(value)
				return nil, fmt.Errorf("Custom type \"%s\" has an invalid %s field: %s.", name, kind, encoded)
			}
			if !nameRE.MatchString(field) {
				return nil, fmt.Errorf("Custom type \"%s\" %s field \"%s\" (%s). Allowed: %s.", name, kind, field, nameViolation(field), namingRule)
			}
			if Contains(customBaseFields, field) {
				if kind == "required" {
					return nil, fmt.Errorf("Custom type \"%s\" cannot declare base field \"%s\" as required (base fields: %s).", name, field, strings.Join(customBaseFields, ", "))
				}
				return nil, fmt.Errorf("Custom type \"%s\" cannot declare base field \"%s\" as optional.", name, field)
			}
			if seen[field] {
				if kind == "required" {
					return nil, fmt.Errorf("Custom type \"%s\" has duplicate required field \"%s\".", name, field)
				}
				return nil, fmt.Errorf("Custom type \"%s\" has field \"%s\" in both required and optional.", name, field)
			}
			seen[field] = true
			merged = Unique(append(merged, field))
		}
	}
	d := &TypeDefinition{Name: name, Required: []string{}, Optional: []string{}, FilesField: "files", Compact: "manual", SectionTitle: strings.ToUpper(name[:1]) + name[1:] + "s", Aliases: map[string][]string{}, Parent: parentName}
	if parent != nil {
		d.Required = append(d.Required, parent.Required...)
		d.Optional = append(d.Optional, parent.Optional...)
		d.DedupKey = parent.DedupKey
		d.IDKey = parent.IDKey
		d.Summary = parent.Summary
		d.ExtractsFiles = parent.ExtractsFiles
		d.FilesField = parent.FilesField
		d.Compact = parent.Compact
		d.SectionTitle = parent.SectionTitle
	}
	d.Required = Unique(append(d.Required, req...))
	optional := Unique(append(d.Optional, opt...))
	d.Optional = []string{}
	for _, f := range optional {
		if !Contains(d.Required, f) {
			d.Optional = append(d.Optional, f)
		}
	}
	if v, ok := c["dedup_key"]; ok {
		d.DedupKey = Text(v)
	}
	if d.DedupKey == "" {
		return nil, fmt.Errorf("Custom type \"%s\" must set \"dedup_key\" (field name or \"content_hash\"), or extend a built-in that defines one.", name)
	}
	if d.DedupKey != "content_hash" && !Contains(merged, d.DedupKey) {
		return nil, fmt.Errorf("Custom type \"%s\" dedup_key \"%s\" must be declared in required/optional or be \"content_hash\".", name, d.DedupKey)
	}
	if v, ok := c["id_key"]; ok {
		d.IDKey = Text(v)
	} else if v, ok := c["dedup_key"]; ok {
		d.IDKey = Text(v)
	} else if d.IDKey == "" {
		d.IDKey = d.DedupKey
	}
	if d.IDKey != "content_hash" && !Contains(merged, d.IDKey) {
		return nil, fmt.Errorf("Custom type \"%s\" id_key \"%s\" must be declared in required/optional or be \"content_hash\".", name, d.IDKey)
	}
	if v, ok := c["summary"]; ok {
		s, isString := v.(string)
		if !isString || s == "" {
			return nil, fmt.Errorf("Custom type \"%s\" \"summary\" must be a non-empty string.", name)
		}
		d.Summary = s
	} else if parent == nil {
		return nil, fmt.Errorf("Custom type \"%s\" must set \"summary\" template string.", name)
	} else {
		d.SummaryInherited = true
	}
	known := Unique(append(append([]string{}, customBaseFields...), merged...))
	for _, match := range templateTokenRE.FindAllStringSubmatch(d.Summary, -1) {
		token := match[1]
		if token == "" {
			token = match[2]
		}
		if !Contains(known, token) {
			sort.Strings(known)
			return nil, fmt.Errorf("Custom type \"%s\" summary references unknown field \"{%s}\". Declared fields: %s.", name, token, strings.Join(known, ", "))
		}
	}
	if v, ok := c["compact"]; ok {
		d.Compact = Text(v)
		if !Contains([]string{"concat", "merge_outcomes", "keep_latest", "manual"}, d.Compact) {
			return nil, fmt.Errorf("Custom type \"%s\" compact strategy \"%s\" is invalid. Use one of: concat, merge_outcomes, keep_latest, manual.", name, d.Compact)
		}
	}
	if v, ok := c["section_title"]; ok {
		d.SectionTitle = Text(v)
	}
	if v, ok := c["extracts_files"]; ok {
		d.ExtractsFiles = Bool(v)
	}
	if v, ok := c["files_field"]; ok {
		d.FilesField = Text(v)
		if d.ExtractsFiles && !Contains(merged, d.FilesField) {
			return nil, fmt.Errorf("Custom type \"%s\" files_field \"%s\" must be declared in required/optional when extracts_files: true.", name, d.FilesField)
		}
	}
	if av, exists := c["aliases"]; exists {
		aliases, ok := av.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Custom type \"%s\" aliases must be a map of canonical field name → legacy alias names.", name)
		}
		allBuiltins := []string{}
		for _, bn := range builtinNames {
			allBuiltins = Unique(append(append(allBuiltins, r.Types[bn].Required...), r.Types[bn].Optional...))
		}
		seenAliases := map[string]bool{}
		for _, canonical := range ObjectKeys(aliases) {
			if !Contains(merged, canonical) {
				return nil, fmt.Errorf("Custom type \"%s\" aliases key \"%s\" must be declared in required/optional.", name, canonical)
			}
			rawNames := AnyList(aliases[canonical])
			names := List(aliases[canonical])
			if len(rawNames) == 0 {
				return nil, fmt.Errorf("Custom type \"%s\" aliases entry \"%s\" must be a non-empty array of legacy field names.", name, canonical)
			}
			for _, rawLegacy := range rawNames {
				legacy, isString := rawLegacy.(string)
				if !isString || legacy == "" {
					encoded, _ := MarshalJSON(rawLegacy)
					return nil, fmt.Errorf("Custom type \"%s\" aliases entry \"%s\" has an invalid legacy name: %s.", name, canonical, encoded)
				}
				if !nameRE.MatchString(legacy) {
					return nil, fmt.Errorf("Custom type \"%s\" aliases legacy name \"%s\" (%s). Allowed: %s.", name, legacy, nameViolation(legacy), namingRule)
				}
				if Contains(customBaseFields, legacy) {
					return nil, fmt.Errorf("Custom type \"%s\" aliases legacy name \"%s\" collides with a base field.", name, legacy)
				}
				if Contains(allBuiltins, legacy) && !Contains(merged, legacy) {
					return nil, fmt.Errorf("Custom type \"%s\" aliases legacy name \"%s\" collides with a built-in type's field. Use a name that isn't already owned by another type, or set it as required/optional on this type if the field really belongs here.", name, legacy)
				}
				if Contains(merged, legacy) {
					return nil, fmt.Errorf("Custom type \"%s\" aliases legacy name \"%s\" is already declared as a current field; aliases are for retired names only.", name, legacy)
				}
				if seenAliases[legacy] {
					return nil, fmt.Errorf("Custom type \"%s\" aliases legacy name \"%s\" appears under multiple canonical fields; each alias maps to one canonical name.", name, legacy)
				}
				seenAliases[legacy] = true
			}
			d.Aliases[canonical] = names
		}
	}
	return d, nil
}
func ApplyAliases(raw Record, aliases map[string][]string) Record {
	for canonical, names := range aliases {
		for _, legacy := range names {
			if value, exists := raw[legacy]; exists {
				if raw[canonical] == nil || raw[canonical] == "" {
					raw[canonical] = value
				}
				delete(raw, legacy)
			}
		}
	}
	return raw
}
