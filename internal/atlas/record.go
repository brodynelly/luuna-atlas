package atlas

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var writeNonNegativeNumber = regexp.MustCompile(`^\d+(\.\d+)?$`)
var writeNameWords = regexp.MustCompile(`[^a-z0-9]+`)
var writeWhiteSpace = regexp.MustCompile(`\s+`)

func (a *App) writeSuccess(message string) {
	a.Human(Style("brand", "✓") + " " + Style("brand", message))
}
func writeStatusColor(status string) string {
	if status == "success" {
		return Style("green", status)
	}
	if status == "failure" {
		return Style("red", status)
	}
	return Style("yellow", status)
}

func writeObject(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case Record:
		return map[string]any(m)
	case Config:
		return map[string]any(m)
	}
	return nil
}

func writeRecordsValue(v any) []Record {
	out := []Record{}
	switch rows := v.(type) {
	case []Record:
		return rows
	case []any:
		for _, r := range rows {
			if m := writeObject(r); m != nil {
				out = append(out, Record(m))
			}
		}
	}
	return out
}
func writeID(r Record) any {
	if id, ok := r["id"]; ok {
		return id
	}
	return nil
}
func writeIDText(r Record, fallback string) string {
	if s := Text(r["id"]); s != "" {
		return s
	}
	return fallback
}
func writeValidation(issues []ValidationIssue) string {
	parts := []string{}
	for _, e := range issues {
		parts = append(parts, e.Path+" "+e.Message)
	}
	return strings.Join(parts, "; ")
}
func writeHasDomain(cfg Config, domain string) bool { _, ok := Obj(cfg["domains"])[domain]; return ok }
func (a *App) writeDomainCheck(cfg Config, domain, command string) bool {
	if writeHasDomain(cfg, domain) {
		return true
	}
	available := strings.Join(DomainNames(cfg), ", ")
	if available == "" {
		available = "(none)"
	}
	if a.JSONMode {
		a.Error(command, fmt.Sprintf("Domain %q not found in config. Available domains: %s", domain, available))
	} else {
		a.ExitCode = 1
		fmt.Fprintln(a.Err, Style("red", fmt.Sprintf("Error: domain %q not found in config.", domain)))
		fmt.Fprintln(a.Err, Style("red", "Available domains: "+available))
	}
	return false
}
func writeUnique(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range values {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
func writeFlagsCopy(i Invocation) map[string]string {
	m := map[string]string{}
	for k := range i.Flags {
		m[k] = i.String(k)
	}
	return m
}
func writeIsNamed(def *TypeDefinition) bool {
	if def == nil {
		return false
	}
	if def.Builtin {
		return def.Name != "convention" && def.Name != "failure"
	}
	return def.DedupKey != "content_hash"
}
func writeParseNumber(raw, flag string) (float64, error) {
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || !writeNonNegativeNumber.MatchString(raw) || math.IsInf(n, 0) || math.IsNaN(n) || n < 0 {
		return 0, fmt.Errorf("--%s must be a non-negative number (got %q).", flag, raw)
	}
	return n, nil
}
func writeOutcomeFlags(i Invocation) (Record, error) {
	if i.String("outcome-status") == "" {
		return nil, nil
	}
	r := Record{"status": i.String("outcome-status")}
	if i.Has("outcome-duration") {
		n, e := writeParseNumber(i.String("outcome-duration"), "outcome-duration")
		if e != nil {
			return nil, e
		}
		r["duration"] = n
	}
	for _, f := range []string{"test-results", "agent"} {
		if v := i.String("outcome-" + f); v != "" {
			r[strings.ReplaceAll(f, "-", "_")] = v
		}
	}
	rememberOrder(map[string]any(r), []string{"status", "duration", "test_results", "agent"})
	return r, nil
}
func writeDerivedName(s string) string {
	words := strings.Fields(writeNameWords.ReplaceAllString(strings.ToLower(s), " "))
	if len(words) > 6 {
		words = words[:6]
	}
	name := strings.Join(words, "-")
	if len(name) > 60 {
		name = name[:60]
	}
	name = strings.TrimRight(name, "-")
	if name == "" {
		name = "record"
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(s)))
	if len(name) > 53 {
		name = name[:53]
	}
	return strings.TrimRight(name, "-") + "-" + hex.EncodeToString(sum[:])[:6]
}
func writeDerivedTitle(s string) string {
	s = strings.TrimSpace(s)
	for n, r := range s {
		if n > 0 && (r == ' ' || r == '\n' || r == '\t') && (s[n-1] == '.' || s[n-1] == '!' || s[n-1] == '?') {
			s = strings.TrimSpace(s[:n])
			break
		}
	}
	if formatLen(s) > 80 {
		return strings.TrimRight(formatSlice(s, 77), " \t\r\n") + "..."
	}
	return s
}
func writeJSONQuote(s string) string { b, _ := MarshalJSON(s); return string(b) }
func writeRetry(domain, content string, i Invocation, missing []string) string {
	parts := []string{"luuna-atlas record", domain}
	if content != "" {
		parts = append(parts, writeJSONQuote(content))
	}
	if v := i.String("type"); v != "" {
		parts = append(parts, "--type "+v)
	}
	if v := i.String("classification"); v != "" && v != "tactical" {
		parts = append(parts, "--classification "+v)
	}
	for _, f := range []string{"name", "content", "description", "resolution", "title", "rationale", "files"} {
		if v := i.String(f); v != "" {
			parts = append(parts, "--"+f+" "+writeJSONQuote(v))
		}
	}
	for _, v := range i.Strings("dir-anchor") {
		parts = append(parts, "--dir-anchor "+writeJSONQuote(v))
	}
	if v := i.String("tags"); v != "" {
		parts = append(parts, "--tags "+writeJSONQuote(v))
	}
	for _, f := range []string{"commit", "issue", "file", "bead", "missions", "gh", "linear"} {
		if v := i.String("evidence-" + f); v != "" {
			if f != "commit" {
				v = writeJSONQuote(v)
			}
			parts = append(parts, "--evidence-"+f+" "+v)
		}
	}
	for _, f := range missing {
		parts = append(parts, "--"+strings.ReplaceAll(f, "_", "-")+" "+writeJSONQuote("<"+f+">"))
	}
	return strings.Join(parts, " ")
}
func (a *App) writeRecordSchemaError(r Record, issues []ValidationIssue, prefix string) {
	if a.JSONMode {
		a.Error("record", prefix+writeValidation(issues))
		return
	}
	a.ExitCode = 1
	fmt.Fprintln(a.Err, Style("red", "Error: record failed schema validation:"))
	for _, e := range issues {
		fmt.Fprintln(a.Err, Style("red", fmt.Sprintf("  %s %s", e.Path, e.Message)))
	}
	if d := a.Registry.Types[Text(r["type"])]; d != nil {
		fmt.Fprintln(a.Err, Style("yellow", fmt.Sprintf("Hint: %s records require: %s", d.Name, strings.Join(d.Required, ", "))))
	}
}

func (a *App) runRecord(i Invocation) error {
	domain := i.Positionals[0]
	if i.Has("batch") || i.Bool("stdin") {
		action := "stdin"
		var b []byte
		var err error
		if i.Has("batch") {
			action = "batch"
			b, err = os.ReadFile(i.String("batch"))
			if os.IsNotExist(err) {
				return fmt.Errorf("Batch file not found: %s", i.String("batch"))
			}
		} else {
			b, err = io.ReadAll(a.In)
		}
		if err != nil {
			return err
		}
		return a.writeBulkRecord(domain, string(b), action, i)
	}
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	if !writeHasDomain(cfg, domain) {
		if err = AddDomain(a.Cwd, domain); err != nil {
			return err
		}
		a.writeSuccess(fmt.Sprintf("Auto-created domain %q", domain))
	}
	typ := i.String("type")
	if typ == "" {
		return fmt.Errorf("--type is required (%s)", strings.Join(a.Registry.Order, ", "))
	}
	def := a.Registry.Types[typ]
	if def == nil {
		return fmt.Errorf("Unknown record type %q. Available: %s.", typ, strings.Join(a.Registry.Order, ", "))
	}
	content := ""
	hasContent := len(i.Positionals) > 1
	if hasContent {
		content = i.Positionals[1]
	}
	flags := writeFlagsCopy(i)
	if def.Builtin {
		text := flags["description"]
		if strings.TrimSpace(text) == "" {
			text = content
		}
		if strings.TrimSpace(text) != "" {
			fill := func(k, v string) {
				if flags[k] == "" {
					flags[k] = v
				}
			}
			switch typ {
			case "convention":
				if !hasContent {
					fill("content", text)
				}
			case "pattern", "reference", "guide":
				fill("name", writeDerivedName(text))
			case "decision":
				fill("title", writeDerivedTitle(text))
				fill("rationale", text)
			}
		}
	}
	classification := i.String("classification")
	if classification == "" {
		classification = "tactical"
	}
	r := Record{"type": typ, "classification": classification, "recorded_at": NowISO()}
	evidence := map[string]any{}
	evidenceOrder := []string{}
	for _, f := range []string{"commit", "issue", "file", "bead", "missions", "gh", "linear"} {
		if v := i.String("evidence-" + f); v != "" {
			evidence[f] = v
			evidenceOrder = append(evidenceOrder, f)
		}
	}
	autoCommit := false
	if i.String("evidence-commit") == "" {
		if commit := GetCurrentCommit(a.Cwd); commit != "" {
			evidence["commit"] = commit
			evidenceOrder = append(evidenceOrder, "commit")
			autoCommit = true
		}
	}
	if len(evidence) > 0 {
		rememberOrder(evidence, evidenceOrder)
		r["evidence"] = evidence
	}
	for _, f := range []string{"tags", "relates-to", "supersedes"} {
		if v := SplitCSV(i.String(f)); len(v) > 0 {
			r[strings.ReplaceAll(f, "-", "_")] = v
		}
	}
	outcome, err := writeOutcomeFlags(i)
	if err != nil {
		return err
	}
	if outcome != nil {
		r["outcomes"] = []Record{outcome}
	}
	dirs, err := NormalizeDirAnchors(i.Strings("dir-anchor"))
	if err != nil {
		return err
	}
	if len(dirs) > 0 {
		dirs = writeUnique(dirs)
		sort.Strings(dirs)
	} else if !i.Has("files") {
		dirs = InferDirAnchors(GetContextFiles(a.Cwd))
	}
	if len(dirs) > 0 {
		r["dir_anchors"] = dirs
	}
	domainCfg := Obj(Obj(cfg["domains"])[domain])
	allowed := List(domainCfg["allowed_types"])
	if !a.AllowDomainMismatch && len(allowed) > 0 && !writeContains(allowed, typ) {
		msg := fmt.Sprintf("type %q is not allowed in domain %q. Allowed types: %s.", typ, domain, strings.Join(allowed, ", "))
		a.Error("record", msg)
		if !a.JSONMode {
			retry := i
			retry.Flags = map[string][]string{}
			for k, v := range i.Flags {
				retry.Flags[k] = v
			}
			retry.Flags["type"] = []string{allowed[0]}
			fmt.Fprintln(a.Err, Style("dim", "  Retry: "+writeRetry(domain, content, retry, nil)))
		}
		return nil
	}
	disabled := ""
	if def.Disabled {
		disabled = fmt.Sprintf("type %q is disabled (declared in disabled_types). Records will still be written; consider migrating off this type.", typ)
		if !a.Quiet && !a.JSONMode {
			a.Warning(disabled)
		}
	}
	missing := []string{}
	collect := func(field string, required bool) {
		value := flags[strings.ReplaceAll(field, "_", "-")]
		fallback := ""
		if def.Builtin {
			if typ == "convention" {
				fallback = "content"
			} else if typ == "pattern" || typ == "reference" || typ == "guide" {
				fallback = "description"
			}
		}
		if value == "" && fallback == field && hasContent && !i.Has(strings.ReplaceAll(field, "_", "-")) {
			value = content
		}
		if value == "" {
			if required {
				missing = append(missing, field)
			}
			return
		}
		if def.ExtractsFiles && field == def.FilesField {
			r[field] = SplitCSV(value)
		} else {
			r[field] = value
		}
	}
	for _, f := range def.Required {
		collect(f, true)
	}
	for _, f := range def.Optional {
		collect(f, false)
	}
	if def.ExtractsFiles {
		if _, ok := r[def.FilesField]; !ok {
			if files := GetContextFiles(a.Cwd); len(files) > 0 {
				r[def.FilesField] = files
			}
		}
	}
	fieldOrder := []string{"type", "classification", "recorded_at", "evidence", "tags", "relates_to", "supersedes", "outcomes", "dir_anchors"}
	fieldOrder = append(fieldOrder, def.Required...)
	fieldOrder = append(fieldOrder, def.Optional...)
	if def.ExtractsFiles {
		fieldOrder = append(fieldOrder, def.FilesField)
	}
	rememberOrder(map[string]any(r), Unique(fieldOrder))
	if len(missing) > 0 {
		mflags := []string{}
		for _, f := range missing {
			mflags = append(mflags, "--"+strings.ReplaceAll(f, "_", "-"))
		}
		base := fmt.Sprintf("%s records are missing required flag(s): %s.", typ, strings.Join(mflags, ", "))
		retry := writeRetry(domain, content, i, missing)
		if a.JSONMode {
			a.Error("record", base+" Example: "+retry)
		} else {
			a.Error("record", base)
			fmt.Fprintln(a.Err, Style("dim", "  Retry: "+retry))
		}
		return nil
	}
	if issues := ValidateRecord(r, a.Registry); len(issues) > 0 {
		if a.JSONMode {
			a.Error("record", "Schema validation failed: "+writeValidation(issues)+fmt.Sprintf(". Hint: %s records require: %s", typ, strings.Join(def.Required, ", ")))
		} else {
			a.writeRecordSchemaError(r, issues, "Schema validation failed: ")
		}
		return nil
	}
	if !a.AllowDomainMismatch {
		missingFields := writeMissingFields(r, List(domainCfg["required_fields"]))
		if len(missingFields) > 0 {
			msg := fmt.Sprintf("domain %q requires field(s) %s.", domain, writeQuoted(missingFields))
			a.Error("record", msg)
			if !a.JSONMode {
				fmt.Fprintln(a.Err, Style("dim", "  Retry: "+writeRetry(domain, content, i, missingFields)))
			}
			return nil
		}
	}
	path, err := ExpertisePath(a.Cwd, domain)
	if err != nil {
		return err
	}
	existing, err := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
	if err != nil {
		return err
	}
	dup := FindDuplicate(existing, r, a.Registry)
	force := i.Bool("force")
	if dup >= 0 && !writeIsNamed(def) {
		id := writeIDText(existing[dup], fmt.Sprintf("#%d", dup+1))
		return fmt.Errorf("duplicate %s already exists in %s (%s); not recorded. If it helped, confirm it instead: luuna-atlas outcome %s %s --status success", typ, domain, id, domain, id)
	}
	warnings := []string{}
	qualityWarnings := []string{}
	quality := Text(Obj(cfg["quality"])["level"])
	if quality == "" {
		quality = "warn"
	}
	if dup < 0 && quality != "off" {
		near := FindNearDuplicates(existing, r, a.Registry)
		if len(near) > 0 && !force {
			return fmt.Errorf("%s", writeNearDuplicates(domain, near))
		}
		issues := CheckRecordQuality(r, autoCommit)
		if len(issues) > 0 && quality == "error" && !force {
			return fmt.Errorf("record failed quality checks (quality.level: error): %s Pass --force to record anyway.", strings.Join(issues, " "))
		}
		for _, v := range issues {
			qualityWarnings = append(qualityWarnings, "quality: "+v)
		}
	}
	if disabled != "" {
		warnings = append(warnings, disabled)
	}
	warnings = append(warnings, qualityWarnings...)
	if i.Bool("dry-run") {
		would := "created"
		if dup >= 0 && !force {
			would = "updated"
		}
		if a.JSONMode {
			data := map[string]any{"action": "dry-run", "wouldDo": would, "domain": domain, "type": typ, "record": r}
			if len(warnings) > 0 {
				data["warnings"] = warnings
			}
			rememberOrder(data, []string{"action", "wouldDo", "domain", "type", "record", "warnings"})
			a.OK("record", data)
		} else {
			verb := fmt.Sprintf("Dry-run: Would create %s in %s", typ, domain)
			if would == "updated" {
				verb = fmt.Sprintf("Dry-run: Would update existing %s in %s", typ, domain)
			}
			a.writeSuccess(verb)
			a.Human(Style("dim", "  Run without --dry-run to apply changes."))
			for _, w := range qualityWarnings {
				a.Warning(w)
			}
		}
		return nil
	}
	pre, err := RunHooks(a.Cwd, cfg, "pre-record", map[string]any{"domain": domain, "record": r}, a.Err)
	if err != nil {
		return err
	}
	if pre.Blocked {
		return fmt.Errorf("%s", writeFallback(pre.BlockReason, "pre-record hook blocked the write"))
	}
	warnings = append(warnings, pre.Warnings...)
	if pre.RanAny {
		if mr := writeObject(Obj(pre.Payload)["record"]); mr != nil {
			if issues := ValidateRecord(Record(mr), a.Registry); len(issues) > 0 {
				return fmt.Errorf("pre-record hook produced an invalid record: %s", writeValidation(issues))
			}
			r = Record(mr)
		}
	}
	action := "created"
	index := -1
	err = WithFileLock(path, func() error {
		current, e := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
		if e != nil {
			return e
		}
		d := FindDuplicate(current, r, a.Registry)
		if d >= 0 && !(force && writeIsNamed(def)) {
			index = d
			if writeIsNamed(def) {
				r = writeMergeOutcomes(current[d], r)
				current[d] = r
				action = "updated"
				return WriteRecords(path, current)
			}
			action = "skipped"
			return nil
		}
		return AppendRecord(path, r, a.Registry)
	})
	if err != nil {
		return err
	}
	if action != "skipped" {
		post, e := RunHooks(a.Cwd, cfg, "post-record", writeRecordPayload(domain, r, action), a.Err)
		if e != nil {
			return e
		}
		warnings = append(warnings, post.Warnings...)
	}
	if a.JSONMode {
		data := map[string]any{"action": action, "domain": domain, "type": typ}
		if action != "skipped" {
			data["record"] = r
		}
		if index >= 0 {
			data["index"] = index + 1
		}
		if len(warnings) > 0 {
			data["warnings"] = warnings
		}
		rememberOrder(data, []string{"action", "domain", "type", "index", "record", "warnings"})
		a.OK("record", data)
	} else {
		switch action {
		case "created":
			a.writeSuccess(fmt.Sprintf("Recorded %s in %s", typ, domain))
		case "updated":
			a.writeSuccess(fmt.Sprintf("Updated existing %s in %s (record #%d)", typ, domain, index+1))
		case "skipped":
			a.Human(Style("yellow", fmt.Sprintf("Duplicate %s already exists in %s (record #%d). Use --force to add anyway.", typ, domain, index+1)))
		}
		for _, w := range warnings {
			if w != disabled {
				a.Warning(w)
			}
		}
	}
	return nil
}
func writeFallback(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
func writeContains(values []string, v string) bool {
	for _, s := range values {
		if s == v {
			return true
		}
	}
	return false
}
func writeQuoted(values []string) string {
	p := []string{}
	for _, s := range values {
		p = append(p, writeJSONQuote(s))
	}
	return strings.Join(p, ", ")
}
func writeMissingFields(r Record, fields []string) []string {
	return FindMissingDomainFields(r, fields)
}
func writeRecordPayload(domain string, r Record, action string) map[string]any {
	payload := map[string]any{"domain": domain, "record": r, "action": action}
	rememberOrder(payload, []string{"domain", "record", "action"})
	return payload
}
func writeMergeOutcomes(old, new Record) Record {
	out := CloneRecord(new)
	merged := append(append([]Record{}, writeRecordsValue(old["outcomes"])...), writeRecordsValue(new["outcomes"])...)
	if len(merged) > 0 {
		out["outcomes"] = merged
	}
	return out
}
func writeNearDuplicates(domain string, near []NearDuplicate) string {
	lines := []string{"similar record(s) already exist in " + domain + ":"}
	for _, n := range near {
		summary := ""
		for _, key := range []string{"name", "title", "content", "description"} {
			if v := Text(n.Record[key]); v != "" {
				summary = writeWhiteSpace.ReplaceAllString(v, " ")
				break
			}
		}
		if formatLen(summary) > 100 {
			summary = formatSlice(summary, 97) + "..."
		}
		shared := ""
		if len(n.SharedAnchors) > 0 {
			shared = ", shares " + strings.Join(n.SharedAnchors, ", ")
		}
		lines = append(lines, fmt.Sprintf("  %s [%s] %s (%d%% similar%s)", writeIDText(n.Record, "?"), Text(n.Record["type"]), summary, int(math.Floor(n.Similarity*100+0.5)), shared))
	}
	id := writeIDText(near[0].Record, "<id>")
	lines = append(lines, fmt.Sprintf("Replace it: add --supersedes %s. Update it: luuna-atlas edit %s %s. Confirm it helped: luuna-atlas outcome %s %s --status success. Record anyway: add --force.", id, domain, id, domain, id))
	return strings.Join(lines, "\n")
}

func (a *App) writeBulkRecord(domain, input, action string, i Invocation) error {
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	if !writeHasDomain(cfg, domain) {
		if err = AddDomain(a.Cwd, domain); err != nil {
			return err
		}
	}
	parsed, err := ParseJSON([]byte(input))
	if err != nil {
		return fmt.Errorf("Failed to parse JSON from stdin: %s", writeJSONParseError(input))
	}
	items, ok := parsed.([]any)
	if !ok {
		items = []any{parsed}
	}
	valid := []Record{}
	errors := []string{}
	warnings := []string{}
	seenDisabled := map[string]bool{}
	domainCfg := Obj(Obj(cfg["domains"])[domain])
	for n, item := range items {
		obj := writeObject(item)
		if obj == nil {
			errors = append(errors, fmt.Sprintf("Record %d:  must be object", n))
			continue
		}
		r := Record(obj)
		fieldOrder := ObjectKeys(obj)
		if _, ok := r["recorded_at"]; !ok {
			r["recorded_at"] = NowISO()
			fieldOrder = append(fieldOrder, "recorded_at")
		}
		if _, ok := r["classification"]; !ok {
			r["classification"] = "tactical"
			fieldOrder = append(fieldOrder, "classification")
		}
		rememberOrder(obj, fieldOrder)
		if issues := ValidateRecord(r, a.Registry); len(issues) > 0 {
			hint := ""
			if d := a.Registry.Types[Text(r["type"])]; d != nil {
				hint = fmt.Sprintf(". Hint: %s records require: %s", d.Name, strings.Join(d.Required, ", "))
			}
			errors = append(errors, fmt.Sprintf("Record %d: %s%s", n, writeValidation(issues), hint))
			continue
		}
		if !a.AllowDomainMismatch {
			allowed := List(domainCfg["allowed_types"])
			if len(allowed) > 0 && !writeContains(allowed, Text(r["type"])) {
				errors = append(errors, fmt.Sprintf("Record %d: type %q is not allowed in domain %q. Allowed types: %s.", n, Text(r["type"]), domain, strings.Join(allowed, ", ")))
				continue
			}
			missing := writeMissingFields(r, List(domainCfg["required_fields"]))
			if len(missing) > 0 {
				errors = append(errors, fmt.Sprintf("Record %d: domain %q requires field(s) %s.", n, domain, writeQuoted(missing)))
				continue
			}
		}
		valid = append(valid, r)
	}
	for _, r := range valid {
		typ := Text(r["type"])
		if def := a.Registry.Types[typ]; def != nil && def.Disabled && !seenDisabled[typ] {
			seenDisabled[typ] = true
			w := fmt.Sprintf("type %q is disabled (declared in disabled_types). Records will still be written; consider migrating off this type.", typ)
			warnings = append(warnings, w)
			if !a.Quiet {
				a.Warning(w)
			}
		}
	}
	toWrite := []Record{}
	for n, r := range valid {
		if !i.Bool("dry-run") {
			pre, e := RunHooks(a.Cwd, cfg, "pre-record", map[string]any{"domain": domain, "record": r}, a.Err)
			if e != nil {
				return e
			}
			warnings = append(warnings, pre.Warnings...)
			if pre.Blocked {
				errors = append(errors, fmt.Sprintf("Record %d: %s", n, writeFallback(pre.BlockReason, "pre-record hook blocked the write")))
				continue
			}
			if pre.RanAny {
				if mr := writeObject(Obj(pre.Payload)["record"]); mr != nil {
					if issues := ValidateRecord(Record(mr), a.Registry); len(issues) > 0 {
						errors = append(errors, fmt.Sprintf("Record %d: pre-record hook produced an invalid record: %s", n, writeValidation(issues)))
						continue
					}
					r = Record(mr)
				}
			}
		}
		toWrite = append(toWrite, r)
	}
	created, updated, skipped := 0, 0, 0
	postPayloads := []map[string]any{}
	path, err := ExpertisePath(a.Cwd, domain)
	if err != nil {
		return err
	}
	process := func() error {
		current, e := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
		if e != nil {
			return e
		}
		for _, r := range toWrite {
			d := FindDuplicate(current, r, a.Registry)
			named := writeIsNamed(a.Registry.Types[Text(r["type"])])
			if d >= 0 && !(i.Bool("force") && named) {
				if named {
					updated++
					if !i.Bool("dry-run") {
						merged := writeMergeOutcomes(current[d], r)
						current[d] = merged
						postPayloads = append(postPayloads, writeRecordPayload(domain, merged, "updated"))
					}
				} else {
					skipped++
				}
			} else {
				created++
				if !i.Bool("dry-run") {
					if Text(r["id"]) == "" {
						r["id"] = GenerateRecordID(r, a.Registry)
					}
					current = append(current, r)
					postPayloads = append(postPayloads, writeRecordPayload(domain, r, "created"))
				}
			}
		}
		if !i.Bool("dry-run") && (created > 0 || updated > 0) {
			return WriteRecords(path, current)
		}
		return nil
	}
	if len(toWrite) > 0 {
		if i.Bool("dry-run") {
			err = process()
		} else {
			err = WithFileLock(path, process)
		}
		if err != nil {
			return err
		}
	}
	for _, payload := range postPayloads {
		post, e := RunHooks(a.Cwd, cfg, "post-record", payload, a.Err)
		if e != nil {
			return e
		}
		warnings = append(warnings, post.Warnings...)
	}
	if len(errors) > 0 {
		if a.JSONMode {
			old := a.ExitCode
			a.Error("record", "Validation errors: "+strings.Join(errors, "; "))
			a.ExitCode = old
		} else {
			fmt.Fprintln(a.Err, Style("red", "Validation errors:"))
			for _, e := range errors {
				fmt.Fprintln(a.Err, Style("red", "  "+e))
			}
		}
	}
	if a.JSONMode {
		if i.Bool("dry-run") {
			action = "dry-run"
		}
		result := map[string]any{"success": len(errors) == 0 || created+updated > 0, "command": "record", "action": action, "domain": domain, "created": created, "updated": updated, "skipped": skipped, "errors": errors, "warnings": warnings}
		rememberOrder(result, []string{"success", "command", "action", "domain", "created", "updated", "skipped", "errors", "warnings"})
		a.Emit(result)
	} else if i.Bool("dry-run") {
		total := created + updated
		if total > 0 || skipped > 0 {
			a.writeSuccess(fmt.Sprintf("Dry-run complete. Would process %d record(s) in %s:", total, domain))
			if created > 0 {
				a.Human(Style("dim", fmt.Sprintf("  Create: %d", created)))
			}
			if updated > 0 {
				a.Human(Style("dim", fmt.Sprintf("  Update: %d", updated)))
			}
			if skipped > 0 {
				a.Human(Style("dim", fmt.Sprintf("  Skip: %d", skipped)))
			}
			a.Human(Style("dim", "  Run without --dry-run to apply changes."))
		} else {
			a.Human(Style("yellow", "No records would be processed."))
		}
	} else {
		if created > 0 {
			a.writeSuccess(fmt.Sprintf("Created %d record(s) in %s", created, domain))
		}
		if updated > 0 {
			a.writeSuccess(fmt.Sprintf("Updated %d record(s) in %s", updated, domain))
		}
		if skipped > 0 {
			a.Human(Style("yellow", fmt.Sprintf("Skipped %d duplicate(s) in %s", skipped, domain)))
		}
	}
	if len(errors) > 0 && created+updated == 0 {
		a.ExitCode = 1
	}
	return nil
}
