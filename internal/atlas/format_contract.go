package atlas

import (
	"fmt"
	"strings"
	"time"
)

var manifestQuickReference = []map[string]string{
	{"command": "luuna-atlas prime <domain>", "description": "load full records for one domain"},
	{"command": "luuna-atlas prime --files <path>", "description": "load records relevant to specific files"},
	{"command": "luuna-atlas search \"<query>\"", "description": "search records across domains"},
	{"command": "luuna-atlas record <domain> --type <type> --description \"...\"", "description": "store an insight"},
	{"command": "luuna-atlas learn", "description": "discover what to record from changed files"},
	{"command": "luuna-atlas sync", "description": "validate, stage, and commit .luuna/atlas/ changes"},
}
var formatTypeOrder = []string{"pattern", "convention", "failure", "decision", "reference", "guide"}

func pluralize(n int, s string) string {
	if n == 1 {
		return s
	}
	return s + "s"
}
func manifestStatusSuffix(n int, g map[string]any) string {
	if float64(n) >= Number(g["hard_limit"]) {
		return " ⚠ OVER HARD LIMIT — must decompose"
	}
	if float64(n) >= Number(g["warn_entries"]) {
		return " ⚠ consider splitting domain"
	}
	if float64(n) >= Number(g["max_entries"]) {
		return " — approaching limit"
	}
	return ""
}
func manifestHealth(n int, g map[string]any) string {
	if float64(n) >= Number(g["hard_limit"]) {
		return "over_hard_limit"
	}
	if float64(n) >= Number(g["warn_entries"]) {
		return "over_warn_threshold"
	}
	if float64(n) >= Number(g["max_entries"]) {
		return "approaching_limit"
	}
	return "ok"
}
func ComputeTypeCounts(records []Record) map[string]any {
	counts := map[string]any{}
	keys := []string{}
	for _, r := range records {
		typ := Text(r["type"])
		if _, exists := counts[typ]; !exists {
			keys = append(keys, typ)
		}
		counts[typ] = int(Number(counts[typ])) + 1
	}
	rememberOrder(counts, keys)
	return counts
}
func manifestTypeCounts(counts map[string]any) string {
	seen := map[string]bool{}
	parts := []string{}
	for _, typ := range formatTypeOrder {
		if n := int(Number(counts[typ])); n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, pluralize(n, typ)))
		}
		seen[typ] = true
	}
	for _, typ := range ObjectKeys(counts) {
		if seen[typ] {
			continue
		}
		if n := int(Number(counts[typ])); n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, pluralize(n, typ)))
		}
	}
	return strings.Join(parts, ", ")
}
func FormatManifest(domains []ManifestDomain, governance map[string]any, format string) string {
	lines := []string{}
	if format == "xml" {
		lines = append(lines, "<manifest>", "  <description>Manifest mode lists available domains. Load records on demand with `luuna-atlas prime &lt;domain&gt;` or `luuna-atlas prime --files &lt;path&gt;`.</description>", "  <quick_reference>")
		for _, q := range manifestQuickReference {
			lines = append(lines, "    <command name=\""+XMLAttrEscape(q["command"])+"\">"+XMLEscape(q["description"])+"</command>")
		}
		lines = append(lines, "  </quick_reference>", "  <domains>")
		for _, d := range domains {
			updated := ""
			if d.LastUpdated != nil {
				updated = " updated=\"" + FormatTimeAgo(*d.LastUpdated) + "\""
			}
			status := ""
			if s := strings.TrimSpace(manifestStatusSuffix(d.Count, governance)); s != "" {
				status = " status=\"" + XMLAttrEscape(s) + "\""
			}
			lines = append(lines, fmt.Sprintf("    <domain name=\"%s\" entries=\"%d\"%s%s>", XMLAttrEscape(d.Domain), d.Count, updated, status))
			for _, t := range formatTypeOrder {
				if n := int(Number(d.TypeCounts[t])); n > 0 {
					lines = append(lines, fmt.Sprintf("      <type_count type=\"%s\" count=\"%d\" />", t, n))
				}
			}
			lines = append(lines, "    </domain>")
		}
		return strings.Join(append(lines, "  </domains>", "</manifest>"), "\n")
	}
	plain := format == "plain"
	if plain {
		lines = append(lines, "Project Expertise Manifest (via Atlas)", "======================================", "", "Manifest mode lists available domains. Load records on demand with `luuna-atlas prime <domain>` or `luuna-atlas prime --files <path>`.", "", "Quick Reference:")
	} else {
		lines = append(lines, "# Project Expertise Manifest (via Atlas)", "", "> Manifest mode lists available domains. Load records on demand with `luuna-atlas prime <domain>` or `luuna-atlas prime --files <path>`.", "", "## Quick Reference", "")
	}
	for _, q := range manifestQuickReference {
		if plain {
			lines = append(lines, "  - "+q["command"]+" — "+q["description"])
		} else {
			lines = append(lines, "- `"+q["command"]+"` — "+q["description"])
		}
	}
	if plain {
		lines = append(lines, "", "Available Domains:")
	} else {
		lines = append(lines, "", "## Available Domains", "")
	}
	if len(domains) == 0 {
		if plain {
			lines = append(lines, "  No expertise recorded yet. Use `luuna-atlas add <domain>` and `luuna-atlas record` to get started.")
		} else {
			lines = append(lines, "No expertise recorded yet. Use `luuna-atlas add <domain>` to create a domain, then `luuna-atlas record` to add records.")
		}
	}
	for _, d := range domains {
		types := ""
		if s := manifestTypeCounts(d.TypeCounts); s != "" {
			types = " (" + s + ")"
		}
		updated := ""
		if d.LastUpdated != nil {
			updated = " — updated " + FormatTimeAgo(*d.LastUpdated)
		}
		prefix := "- **" + d.Domain + "**: "
		if plain {
			prefix = "  - " + d.Domain + ": "
		}
		lines = append(lines, fmt.Sprintf("%s%d %s%s%s%s", prefix, d.Count, pluralize(d.Count, "record"), types, updated, manifestStatusSuffix(d.Count, governance)))
	}
	return strings.Join(lines, "\n")
}
func BuildManifestPayload(domains []ManifestDomain, g map[string]any) map[string]any {
	entries := []any{}
	for _, d := range domains {
		var updated any
		if d.LastUpdated != nil {
			updated = d.LastUpdated.UTC().Format("2006-01-02T15:04:05.000Z")
		}
		health := map[string]any{"status": manifestHealth(d.Count, g), "max_entries": g["max_entries"], "warn_entries": g["warn_entries"], "hard_limit": g["hard_limit"]}
		rememberOrder(health, []string{"status", "max_entries", "warn_entries", "hard_limit"})
		entry := map[string]any{"domain": d.Domain, "count": d.Count, "lastUpdated": updated, "type_counts": d.TypeCounts, "health": health}
		rememberOrder(entry, []string{"domain", "count", "lastUpdated", "type_counts", "health"})
		entries = append(entries, entry)
	}
	result := map[string]any{"type": "manifest", "quick_reference": manifestQuickReference, "domains": entries}
	rememberOrder(result, []string{"type", "quick_reference", "domains"})
	return result
}

type contractCustom struct {
	Name, Parent       string
	Required, Optional []string
}
type contractDomain struct {
	Name              string
	Allowed, Required []string
}

func FormatProjectContract(cfg Config, registry *Registry, format string) string {
	custom := []contractCustom{}
	for _, name := range registry.Order {
		d := registry.Types[name]
		if !d.Builtin {
			custom = append(custom, contractCustom{name, Text(Obj(Obj(cfg["custom_types"])[name])["extends"]), d.Required, d.Optional})
		}
	}
	disabled := List(cfg["disabled_types"])
	domains := []contractDomain{}
	for _, name := range DomainNames(cfg) {
		dc := Obj(Obj(cfg["domains"])[name])
		allowed, required := List(dc["allowed_types"]), List(dc["required_fields"])
		if len(allowed) > 0 || len(required) > 0 {
			domains = append(domains, contractDomain{name, allowed, required})
		}
	}
	hooks := []string{}
	for _, event := range []string{"pre-record", "post-record", "pre-prime", "pre-prune", "pre-compact"} {
		if len(List(Obj(cfg["hooks"])[event])) > 0 {
			hooks = append(hooks, event)
		}
	}
	if len(custom)+len(disabled)+len(domains)+len(hooks) == 0 {
		return ""
	}
	suffix := func(t contractCustom) string {
		p := []string{}
		if len(t.Required) > 0 {
			p = append(p, "required: "+strings.Join(t.Required, ", "))
		}
		if len(t.Optional) > 0 {
			p = append(p, "optional: "+strings.Join(t.Optional, ", "))
		}
		if len(p) == 0 {
			return ""
		}
		return "; " + strings.Join(p, "; ")
	}
	lines := []string{}
	if format == "xml" {
		lines = append(lines, "<contract>")
		if len(custom) > 0 {
			lines = append(lines, "  <custom_types>")
			for _, t := range custom {
				ext := ""
				if t.Parent != "" {
					ext = " extends=\"" + XMLAttrEscape(t.Parent) + "\""
				}
				lines = append(lines, "    <type name=\""+XMLAttrEscape(t.Name)+"\""+ext+">")
				if len(t.Required) > 0 {
					lines = append(lines, "      <required>"+XMLEscape(strings.Join(t.Required, ", "))+"</required>")
				}
				if len(t.Optional) > 0 {
					lines = append(lines, "      <optional>"+XMLEscape(strings.Join(t.Optional, ", "))+"</optional>")
				}
				lines = append(lines, "    </type>")
			}
			lines = append(lines, "  </custom_types>")
		}
		if len(disabled) > 0 {
			lines = append(lines, "  <disabled_types>"+XMLEscape(strings.Join(disabled, ", "))+"</disabled_types>")
		}
		if len(domains) > 0 {
			lines = append(lines, "  <domains>")
			for _, d := range domains {
				attrs := ""
				if len(d.Allowed) > 0 {
					attrs += " allowed=\"" + XMLAttrEscape(strings.Join(d.Allowed, ", ")) + "\""
				}
				if len(d.Required) > 0 {
					attrs += " required=\"" + XMLAttrEscape(strings.Join(d.Required, ", ")) + "\""
				}
				lines = append(lines, "    <domain name=\""+XMLAttrEscape(d.Name)+"\""+attrs+" />")
			}
			lines = append(lines, "  </domains>")
		}
		if len(hooks) > 0 {
			lines = append(lines, "  <hooks>"+XMLEscape(strings.Join(hooks, ", "))+"</hooks>")
		}
		return strings.Join(append(lines, "</contract>"), "\n")
	}
	plain := format == "plain"
	if plain {
		lines = append(lines, "Project Contract (write-side gates)", "===================================")
	} else {
		lines = append(lines, "## Project Contract", "", "Write-side gates `luuna-atlas record` enforces in this project.", "")
	}
	if len(custom) > 0 {
		if plain {
			lines = append(lines, "", "Custom types:")
		} else {
			lines = append(lines, "**Custom types**:")
		}
		for _, t := range custom {
			ext := ""
			if t.Parent != "" {
				if plain {
					ext = " (extends " + t.Parent + ")"
				} else {
					ext = " (extends `" + t.Parent + "`)"
				}
			}
			if plain {
				lines = append(lines, "  - "+t.Name+ext+suffix(t))
			} else {
				lines = append(lines, "- `"+t.Name+"`"+ext+suffix(t))
			}
		}
		if !plain {
			lines = append(lines, "")
		}
	}
	if len(disabled) > 0 {
		if plain {
			lines = append(lines, "", "Disabled types: "+strings.Join(disabled, ", ")+" (writes emit a deprecation warning)")
		} else {
			names := []string{}
			for _, n := range disabled {
				names = append(names, "`"+n+"`")
			}
			lines = append(lines, "**Disabled types**: "+strings.Join(names, ", ")+" (writes emit a deprecation warning)", "")
		}
	}
	if len(domains) > 0 {
		if plain {
			lines = append(lines, "", "Per-domain rules:")
		} else {
			lines = append(lines, "**Per-domain rules**:")
		}
		for _, d := range domains {
			p := []string{}
			if len(d.Allowed) > 0 {
				label := "allowed types — "
				if plain {
					label = "allowed: "
				}
				p = append(p, label+strings.Join(d.Allowed, ", "))
			}
			if len(d.Required) > 0 {
				label := "required fields — "
				if plain {
					label = "required: "
				}
				p = append(p, label+strings.Join(d.Required, ", "))
			}
			if plain {
				lines = append(lines, "  - "+d.Name+": "+strings.Join(p, "; "))
			} else {
				lines = append(lines, "- `"+d.Name+"`: "+strings.Join(p, "; "))
			}
		}
		if !plain {
			lines = append(lines, "")
		}
	}
	if len(hooks) > 0 {
		if plain {
			lines = append(lines, "", "Active hooks: "+strings.Join(hooks, ", "))
		} else {
			lines = append(lines, "**Active hooks**: "+strings.Join(hooks, ", "))
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\r\n \t")
}

// Kept here for consumers constructing manifest timestamps without parsing text.
func FormatISODate(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }
