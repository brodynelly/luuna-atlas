package atlas

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

type FormatOptions struct {
	Format      string
	Full        bool
	Annotations map[string]string
	StaleIDs    map[string]bool
}
type DomainStatus struct {
	Domain                                      string
	Count                                       int
	LastUpdated, OldestRecorded, NewestRecorded *time.Time
	Rotting                                     bool
	RottingDays                                 *int
}
type ManifestDomain struct {
	Domain      string
	Count       int
	LastUpdated *time.Time
	TypeCounts  map[string]any
}

var formatNow = time.Now
var formatWhitespace = regexp.MustCompile(`\s+`)
var summaryToken = regexp.MustCompile(`(?i)\{\{([a-z_][a-z0-9_]*)\}\}|\{([a-z_][a-z0-9_]*)\}`)
var sentenceEnd = regexp.MustCompile(`[.!?]\s`)

// JS uses UTF-16 string positions, which matters for truncating non-ASCII records.
func formatSlice(s string, n int) string {
	u := utf16.Encode([]rune(s))
	if len(u) <= n {
		return s
	}
	return string(utf16.Decode(u[:n]))
}
func formatLen(s string) int { return len(utf16.Encode([]rune(s))) }
func Truncate(text string, max int) string {
	if formatLen(text) <= max {
		return text
	}
	if loc := sentenceEnd.FindStringIndex(text); loc != nil && loc[0] > 0 && formatLen(text[:loc[0]]) < max {
		return text[:loc[0]+1]
	}
	return formatSlice(text, max) + "..."
}
func FormatTimeAgo(t time.Time) string {
	d := formatNow().Sub(t)
	mins := int(math.Floor(d.Minutes()))
	if mins < 1 {
		return "just now"
	}
	if mins < 60 {
		return fmt.Sprintf("%dm ago", mins)
	}
	hours := int(math.Floor(d.Hours()))
	if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	}
	return fmt.Sprintf("%dd ago", int(math.Floor(d.Hours()/24)))
}
func XMLEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
func XMLAttrEscape(s string) string { return strings.ReplaceAll(XMLEscape(s), "\"", "&quot;") }
func formatJS(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []string:
		return strings.Join(x, ",")
	case []any:
		p := make([]string, len(x))
		for i, v := range x {
			p[i] = formatJS(v)
		}
		return strings.Join(p, ",")
	case map[string]any, Record:
		return "[object Object]"
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return fmt.Sprint(v)
}
func formatScore(r Record) float64 {
	return ConfirmationScore(r)
}
func RecordSummary(r Record, registry *Registry) string {
	switch Text(r["type"]) {
	case "convention":
		return Truncate(Text(r["content"]), 60)
	case "failure":
		return Truncate(Text(r["description"]), 60)
	case "decision":
		return Text(r["title"])
	case "pattern", "reference", "guide":
		return Text(r["name"])
	}
	if registry == nil {
		return ""
	}
	def := registry.Types[Text(r["type"])]
	if def == nil {
		return ""
	}
	if def.SummaryInherited {
		switch def.Parent {
		case "convention":
			return Truncate(Text(r["content"]), 60)
		case "failure":
			return Truncate(Text(r["description"]), 60)
		}
	}
	return summaryToken.ReplaceAllStringFunc(def.Summary, func(tok string) string {
		m := summaryToken.FindStringSubmatch(tok)
		key := m[1]
		if key == "" {
			key = m[2]
		}
		return formatJS(r[key])
	})
}
func FormatLinks(r Record) string {
	parts := []string{}
	if a := List(r["relates_to"]); len(a) > 0 {
		parts = append(parts, "relates to: "+strings.Join(a, ", "))
	}
	if a := List(r["supersedes"]); len(a) > 0 {
		parts = append(parts, "supersedes: "+strings.Join(a, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, "; ") + "]"
}
func FormatEvidence(v any) string {
	e := Obj(v)
	parts := []string{}
	for _, key := range []string{"commit", "date", "issue", "file"} {
		if s := Text(e[key]); s != "" {
			parts = append(parts, key+": "+s)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, ", ") + "]"
}
func FormatRecordMeta(r Record, full bool) string {
	if !full {
		return FormatLinks(r)
	}
	parts := []string{"(" + Text(r["classification"]) + ")" + FormatEvidence(r["evidence"])}
	if tags := append([]string{}, List(r["tags"])...); len(tags) > 0 {
		parts = append(parts, "[tags: "+strings.Join(tags, ", ")+"]")
	}
	return " " + strings.Join(parts, " ") + FormatLinks(r)
}
func formatIDTag(r Record) string {
	if id := Text(r["id"]); id != "" {
		return "[" + id + "] "
	}
	return ""
}
func FormatRecordIndex(r Record, registry *Registry, stale bool) string {
	head := ""
	body := ""
	for _, k := range []string{"content", "name", "title"} {
		if s := Text(r[k]); s != "" {
			head = s
			break
		}
	}
	for _, k := range []string{"description", "rationale"} {
		if s := Text(r[k]); s != "" {
			body = s
			break
		}
	}
	s := head
	if head != "" && body != "" {
		s = head + ": " + body
	} else if s == "" {
		s = body
	}
	if s == "" {
		s = RecordSummary(r, registry)
	}
	s = strings.TrimSpace(formatWhitespace.ReplaceAllString(s, " "))
	if formatLen(s) > 100 {
		s = formatSlice(s, 100) + "..."
	}
	id := ""
	if Text(r["id"]) != "" {
		id = Text(r["id"]) + " "
	}
	field := "files"
	if registry != nil && registry.Types[Text(r["type"])] != nil && registry.Types[Text(r["type"])].FilesField != "" {
		field = registry.Types[Text(r["type"])].FilesField
	}
	anchors := append(append([]string{}, List(r[field])...), List(r["dir_anchors"])...)
	suffix := ""
	if len(anchors) > 0 {
		shown := anchors
		if len(shown) > 3 {
			shown = shown[:3]
		}
		suffix = " (" + strings.Join(shown, ", ")
		if len(anchors) > 3 {
			suffix += fmt.Sprintf(", +%d", len(anchors)-3)
		}
		suffix += ")"
	}
	if score := formatScore(r); score > 0 {
		suffix += " ★" + strconv.FormatFloat(score, 'f', -1, 64)
	}
	if stale {
		suffix += " (stale?)"
	}
	return "- " + id + "[" + Text(r["type"]) + "] " + s + suffix
}

func formatMarkdownRecords(def *TypeDefinition, records []Record, full bool, registry *Registry, annotations map[string]string) string {
	if len(records) == 0 {
		return ""
	}
	lines := []string{"### " + def.SectionTitle}
	for _, r := range records {
		line := "- " + formatIDTag(r)
		switch def.Name {
		case "convention":
			line += Text(r["content"]) + FormatRecordMeta(r, full)
		case "pattern", "reference", "guide":
			line += "**" + Text(r["name"]) + "**: " + Text(r["description"])
			if files := List(r["files"]); len(files) > 0 {
				line += " (" + strings.Join(files, ", ") + ")"
			}
			line += FormatRecordMeta(r, full)
		case "failure":
			line += Text(r["description"]) + FormatRecordMeta(r, full)
		case "decision":
			line += "**" + Text(r["title"]) + "**: " + Text(r["rationale"]) + FormatRecordMeta(r, full)
		default:
			line += "**" + RecordSummary(r, registry) + "**"
			details := []string{}
			for _, f := range append(append([]string{}, def.Required...), def.Optional...) {
				if def.ExtractsFiles && f == def.FilesField {
					continue
				}
				if v, ok := r[f]; ok && v != nil && formatJS(v) != "" {
					details = append(details, f+": "+formatJS(v))
				}
			}
			if len(details) > 0 {
				line += " — " + strings.Join(details, "; ")
			}
			if def.ExtractsFiles {
				if files := List(r[def.FilesField]); len(files) > 0 {
					line += " (" + strings.Join(files, ", ") + ")"
				}
			}
			line += FormatRecordMeta(r, full)
		}
		if id := Text(r["id"]); id != "" && annotations[id] != "" {
			line += " — " + annotations[id]
		}
		lines = append(lines, line)
		if def.Name == "failure" {
			lines = append(lines, "  → "+Text(r["resolution"]))
		}
	}
	return strings.Join(lines, "\n")
}
func formatPlainRecords(def *TypeDefinition, records []Record, annotations map[string]string) []string {
	if !def.Builtin {
		return nil
	}
	lines := []string{def.SectionTitle + ":"}
	for _, r := range records {
		line := "  - " + formatIDTag(r)
		switch def.Name {
		case "convention":
			line += Text(r["content"])
		case "pattern", "reference", "guide":
			line += Text(r["name"]) + ": " + Text(r["description"])
			if files := List(r["files"]); len(files) > 0 {
				line += " (" + strings.Join(files, ", ") + ")"
			}
		case "failure":
			line += Text(r["description"])
		case "decision":
			line += Text(r["title"]) + ": " + Text(r["rationale"])
		}
		line += FormatLinks(r)
		if why := annotations[Text(r["id"])]; why != "" {
			line += "  (" + why + ")"
		}
		lines = append(lines, line)
		if def.Name == "failure" {
			lines = append(lines, "    Fix: "+Text(r["resolution"]))
		}
	}
	return append(lines, "")
}
func formatXMLRecord(r Record, def *TypeDefinition, annotations map[string]string) []string {
	typ := Text(r["type"])
	id := ""
	if Text(r["id"]) != "" {
		id = " id=\"" + XMLEscape(Text(r["id"])) + "\""
	}
	why := ""
	if annotations[Text(r["id"])] != "" {
		why = " why=\"" + XMLAttrEscape(annotations[Text(r["id"])]) + "\""
	}
	lines := []string{"  <" + typ + id + " classification=\"" + Text(r["classification"]) + "\"" + why + ">"}
	fields := []string{}
	switch typ {
	case "convention":
		lines = append(lines, "    "+XMLEscape(Text(r["content"])))
	case "pattern", "reference", "guide":
		fields = []string{"name", "description"}
		if len(List(r["files"])) > 0 {
			fields = append(fields, "files")
		}
	case "failure":
		fields = []string{"description", "resolution"}
	case "decision":
		fields = []string{"title", "rationale"}
	default:
		fields = append(append([]string{}, def.Required...), def.Optional...)
	}
	for _, f := range fields {
		v, ok := r[f]
		if !ok || v == nil {
			continue
		}
		value := ""
		switch v.(type) {
		case []string, []any:
			parts := append([]string{}, List(v)...)
			for j := range parts {
				parts[j] = XMLEscape(parts[j])
			}
			value = strings.Join(parts, ", ")
		default:
			value = XMLEscape(formatJS(v))
		}
		lines = append(lines, "    <"+f+">"+value+"</"+f+">")
	}
	if tags := append([]string{}, List(r["tags"])...); len(tags) > 0 {
		for j := range tags {
			tags[j] = XMLEscape(tags[j])
		}
		lines = append(lines, "    <tags>"+strings.Join(tags, ", ")+"</tags>")
	}
	for _, f := range []string{"relates_to", "supersedes"} {
		if v := List(r[f]); len(v) > 0 {
			lines = append(lines, "    <"+f+">"+strings.Join(v, ", ")+"</"+f+">")
		}
	}
	if outcomes, ok := r["outcomes"].([]any); ok {
		for _, v := range outcomes {
			o := Obj(v)
			duration := ""
			if d, ok := o["duration"]; ok {
				duration = " duration=\"" + formatJS(d) + "\""
			}
			agent := ""
			if Text(o["agent"]) != "" {
				agent = " agent=\"" + XMLEscape(Text(o["agent"])) + "\""
			}
			lines = append(lines, "    <outcome status=\""+Text(o["status"])+"\""+duration+agent+">"+XMLEscape(Text(o["test_results"]))+"</outcome>")
		}
	}
	return append(lines, "  </"+typ+">")
}
func FormatDomain(domain string, records []Record, lastUpdated *time.Time, registry *Registry, opts FormatOptions) string {
	format := opts.Format
	if format == "" {
		format = "markdown"
	}
	updated := ""
	if lastUpdated != nil {
		updated = FormatTimeAgo(*lastUpdated)
	}
	if format == "compact" {
		suffix := ""
		if updated != "" {
			suffix = ", updated " + updated
		}
		lines := []string{fmt.Sprintf("## %s (%d records%s)", domain, len(records), suffix)}
		for _, r := range records {
			lines = append(lines, FormatRecordIndex(r, registry, opts.StaleIDs[Text(r["id"])]))
		}
		return strings.Join(lines, "\n")
	}
	if format == "xml" {
		suffix := ""
		if updated != "" {
			suffix = " updated=\"" + updated + "\""
		}
		lines := []string{fmt.Sprintf("<domain name=\"%s\" entries=\"%d\"%s>", XMLEscape(domain), len(records), suffix)}
		for _, r := range records {
			def := registry.Types[Text(r["type"])]
			if def != nil {
				lines = append(lines, formatXMLRecord(r, def, opts.Annotations)...)
			}
		}
		return strings.Join(append(lines, "</domain>"), "\n")
	}
	suffix := ""
	if updated != "" {
		if format == "plain" {
			suffix = " (updated " + updated + ")"
		} else {
			suffix = ", updated " + updated
		}
	}
	lines := []string{}
	if format == "plain" {
		lines = append(lines, fmt.Sprintf("[%s] %d records%s", domain, len(records), suffix), "")
	} else {
		lines = append(lines, fmt.Sprintf("## %s (%d records%s)", domain, len(records), suffix), "")
	}
	sections := []string{}
	for _, name := range registry.Order {
		def := registry.Types[name]
		subset := []Record{}
		for _, r := range records {
			if Text(r["type"]) == name {
				subset = append(subset, r)
			}
		}
		if len(subset) == 0 {
			continue
		}
		if format == "plain" {
			lines = append(lines, formatPlainRecords(def, subset, opts.Annotations)...)
		} else {
			sections = append(sections, formatMarkdownRecords(def, subset, opts.Full, registry, opts.Annotations))
		}
	}
	if format == "plain" {
		return strings.TrimRight(strings.Join(lines, "\n"), " \t\r\n")
	}
	return strings.Join(append(lines, strings.Join(sections, "\n\n")), "\n")
}
func FormatPrime(sections []string, format string) string {
	if format == "" {
		format = "markdown"
	}
	key := "prime_" + format
	if len(sections) == 0 {
		return formatAssets["prime_empty_"+format]
	}
	sep := "\n\n"
	if format == "xml" {
		sep = "\n"
	}
	return strings.ReplaceAll(formatAssets[key], "__ATLAS_SECTIONS__", strings.Join(sections, sep))
}
func SessionEndReminder(format string, cfg Config) string {
	close := Obj(Obj(cfg["prime"])["session_close"])
	if custom := Text(close["custom"]); custom != "" {
		return custom
	}
	style := Text(close["style"])
	if style == "" {
		style = "conditional"
	}
	key := "close_" + style + "_" + format
	if value, ok := formatAssets[key]; ok {
		return value
	}
	return formatAssets["close_conditional_"+format]
}
func FormatStatus(stats []DomainStatus, governance map[string]any) string {
	lines := []string{"Atlas Status", "============", ""}
	if len(stats) == 0 {
		return strings.Join(append(lines, "No domains configured. Run `luuna-atlas add <domain>` to get started."), "\n")
	}
	for _, s := range stats {
		updated := "never"
		if s.LastUpdated != nil {
			updated = FormatTimeAgo(*s.LastUpdated)
		}
		rangeText := ""
		if s.OldestRecorded != nil && s.NewestRecorded != nil {
			old, new := FormatTimeAgo(*s.OldestRecorded), FormatTimeAgo(*s.NewestRecorded)
			rangeText = " — recorded " + old
			if old != new {
				rangeText += " → " + new
			}
		}
		rot := ""
		if s.Rotting {
			rot = " ⚠ ROTTING"
			if s.RottingDays != nil {
				rot += fmt.Sprintf(" (no writes in %dd)", *s.RottingDays)
			}
		}
		lines = append(lines, fmt.Sprintf("  %s: %d records (updated %s)%s%s%s", s.Domain, s.Count, updated, rangeText, manifestStatusSuffix(s.Count, governance), rot))
	}
	return strings.Join(lines, "\n")
}
func formatKeys(m map[string]any) []string { return ObjectKeys(m) }
func FormatJSON(value any) string          { return JSONPretty(value) }
func FormatTypeRecords(records []Record, registry *Registry, full bool) string {
	if len(records) == 0 {
		return ""
	}
	def := registry.Types[Text(records[0]["type"])]
	if def == nil {
		return ""
	}
	return formatMarkdownRecords(def, records, full, registry, nil)
}
