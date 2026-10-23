package atlas

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type compactCandidateRecord struct {
	ID         *string `json:"id,omitempty"`
	Summary    string  `json:"summary"`
	RecordedAt string  `json:"recorded_at"`
}
type compactCandidate struct {
	Domain  string                   `json:"domain"`
	Type    string                   `json:"type"`
	Records []compactCandidateRecord `json:"records"`
}

func (a *App) compactCandidates(domain string, records []Record, now time.Time, shelf map[string]any, min int) []compactCandidate {
	groups := map[string][]Record{}
	keys := []string{}
	for _, r := range records {
		key := Text(r["type"])
		if def := a.Registry.Types[key]; def != nil && def.Compact == "merge_outcomes" {
			key += "::" + Text(r[def.IDKey])
		}
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], r)
	}
	out := []compactCandidate{}
	for _, key := range keys {
		group := groups[key]
		if len(group) < 2 {
			continue
		}
		stale := false
		for _, r := range group {
			if IsRecordStale(r, now, shelf) {
				stale = true
				break
			}
		}
		if !stale && len(group) < min {
			continue
		}
		candidate := compactCandidate{Domain: domain, Type: Text(group[0]["type"]), Records: []compactCandidateRecord{}}
		for _, r := range group {
			var id *string
			if v, ok := r["id"].(string); ok {
				id = &v
			}
			candidate.Records = append(candidate.Records, compactCandidateRecord{ID: id, Summary: RecordSummary(r, a.Registry), RecordedAt: Text(r["recorded_at"])})
		}
		out = append(out, candidate)
	}
	return out
}
func (a *App) runCompact(i Invocation) error {
	domain := ""
	if len(i.Positionals) > 0 {
		domain = i.Positionals[0]
	}
	if i.Bool("analyze") {
		return a.compactAnalyze(domain)
	}
	if i.Bool("auto") {
		return a.compactAuto(domain, i)
	}
	if i.Bool("apply") {
		if domain == "" {
			return fmt.Errorf("Domain is required for --apply.")
		}
		return a.compactApply(domain, i)
	}
	return fmt.Errorf("Specify --analyze, --auto, or --apply.")
}
func (a *App) compactDomains(domain string, cfg Config) ([]string, error) {
	if domain != "" {
		if !writeHasDomain(cfg, domain) {
			return nil, fmt.Errorf("Domain %q not found in config.", domain)
		}
		return []string{domain}, nil
	}
	return DomainNames(cfg), nil
}
func (a *App) compactCollect(domain string, cfg Config, min int) ([]compactCandidate, error) {
	domains, err := a.compactDomains(domain, cfg)
	if err != nil {
		return nil, err
	}
	out := []compactCandidate{}
	shelf := Obj(ConfigValue(cfg, "classification_defaults", "shelf_life"))
	now := time.Now()
	for _, d := range domains {
		path, e := ExpertisePath(a.Cwd, d)
		if e != nil {
			return nil, e
		}
		records, e := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
		if e != nil {
			return nil, e
		}
		if len(records) >= 2 {
			out = append(out, a.compactCandidates(d, records, now, shelf, min)...)
		}
	}
	return out, nil
}
func (a *App) compactAnalyze(domain string) error {
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	candidates, err := a.compactCollect(domain, cfg, 3)
	if err != nil {
		return err
	}
	if a.JSONMode {
		a.OK("compact", map[string]any{"action": "analyze", "candidates": candidates})
		return nil
	}
	if len(candidates) == 0 {
		a.Human(Style("brand", "No compaction candidates found."))
		return nil
	}
	total := 0
	for _, c := range candidates {
		total += len(c.Records)
	}
	fmt.Fprintln(a.Out, Style("bold", "\nCompaction candidates:\n"))
	fmt.Fprintln(a.Out, Style("dim", fmt.Sprintf("Found %d groups (%d records that could be compacted)\n", len(candidates), total)))
	lastDomain := ""
	for _, c := range candidates {
		if c.Domain != lastDomain {
			if lastDomain != "" {
				fmt.Fprintln(a.Out)
			}
			fmt.Fprintln(a.Out, Style("bold", c.Domain+":"))
			lastDomain = c.Domain
		}
		fmt.Fprintf(a.Out, "  %s (%d records)\n", Style("cyan", c.Type), len(c.Records))
		for n, r := range c.Records {
			if n >= 3 {
				break
			}
			id := Style("dim", "(no id)")
			if r.ID != nil {
				id = Style("accent", *r.ID)
			}
			fmt.Fprintf(a.Out, "    %s: %s\n", id, r.Summary)
		}
		if len(c.Records) > 3 {
			fmt.Fprintln(a.Out, Style("dim", fmt.Sprintf("    ... and %d more", len(c.Records)-3)))
		}
	}
	fmt.Fprintln(a.Out)
	fmt.Fprintln(a.Out, Style("dim", "To compact manually:"))
	fmt.Fprintln(a.Out, Style("dim", "  luuna-atlas compact <domain> --apply --records <ids> --type <type> [fields...]"))
	fmt.Fprintln(a.Out, Style("dim", "\nTo compact automatically:"))
	fmt.Fprintln(a.Out, Style("dim", "  luuna-atlas compact --auto [--dry-run]"))
	return nil
}
func (a *App) compactPrintSummary(heading, verb string, process, all []compactCandidate) {
	total := 0
	for _, c := range process {
		total += len(c.Records)
	}
	fmt.Fprintln(a.Out, Style("bold", "\n"+heading+":\n"))
	fmt.Fprintf(a.Out, "  %d groups %s compacted\n  %d records → %d records\n\n", len(process), verb, total, len(process))
	for _, c := range process {
		fmt.Fprintf(a.Out, "%s (%d records)\n", Style("cyan", c.Domain+"/"+c.Type), len(c.Records))
		for n, r := range c.Records {
			if n >= 3 {
				break
			}
			id := Style("dim", "(no id)")
			if r.ID != nil {
				id = Style("accent", *r.ID)
			}
			fmt.Fprintf(a.Out, "  %s: %s\n", id, r.Summary)
		}
		if len(c.Records) > 3 {
			fmt.Fprintln(a.Out, Style("dim", fmt.Sprintf("  ... and %d more", len(c.Records)-3)))
		}
		fmt.Fprintln(a.Out)
	}
	if len(all) > len(process) {
		fmt.Fprintln(a.Out, Style("yellow", fmt.Sprintf("Note: %d additional groups skipped due to --max-records limit\n", len(all)-len(process))))
	}
}
func compactPositive(raw string, def int, flag string) (int, error) {
	if raw == "" {
		return def, nil
	}
	n, e := strconv.ParseUint(raw, 10, 53)
	if e != nil || n == 0 || fmt.Sprint(n) != raw && strings.TrimLeft(raw, "0") != fmt.Sprint(n) {
		return 0, fmt.Errorf("--%s must be a positive integer (got %q).", flag, raw)
	}
	return int(n), nil
}
func (a *App) compactAuto(domain string, i Invocation) error {
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	min, err := compactPositive(i.String("min-group"), 5, "min-group")
	if err != nil {
		return err
	}
	max, err := compactPositive(i.String("max-records"), 50, "max-records")
	if err != nil {
		return err
	}
	all, err := a.compactCollect(domain, cfg, min)
	if err != nil {
		return err
	}
	action := "auto"
	if i.Bool("dry-run") {
		action = "dry-run"
	}
	if len(all) == 0 {
		if a.JSONMode {
			a.OK("compact", map[string]any{"action": action, "compacted": 0, "results": []any{}})
		} else {
			a.Human(Style("brand", "No compaction candidates found."))
		}
		return nil
	}
	process := []compactCandidate{}
	total := 0
	for _, c := range all {
		if total+len(c.Records) > max {
			break
		}
		process = append(process, c)
		total += len(c.Records)
	}
	if !a.JSONMode && !i.Bool("dry-run") {
		a.compactPrintSummary("Compaction summary", "will be", process, all)
	}
	if i.Bool("dry-run") {
		if a.JSONMode {
			groups := []map[string]any{}
			for _, c := range process {
				group := map[string]any{"domain": c.Domain, "type": c.Type, "count": len(c.Records), "records": c.Records}
				rememberOrder(group, []string{"domain", "type", "count", "records"})
				groups = append(groups, group)
			}
			a.OK("compact", map[string]any{"action": "dry-run", "wouldCompact": total, "groups": groups})
		} else {
			a.compactPrintSummary("Dry-run preview", "would be", process, all)
			a.writeSuccess(fmt.Sprintf("Dry-run complete. Would compact %d records across %d groups.", total, len(process)))
			a.Human(Style("dim", "  Run without --dry-run to apply changes."))
		}
		return nil
	}
	if !a.JSONMode && !i.Bool("yes") {
		if !a.writeConfirm("Proceed with compaction?") {
			fmt.Fprintln(a.Out, Style("yellow", "Compaction cancelled."))
			return nil
		}
	}
	byDomain := map[string][]compactCandidate{}
	domains := []string{}
	for _, c := range process {
		if _, ok := byDomain[c.Domain]; !ok {
			domains = append(domains, c.Domain)
		}
		byDomain[c.Domain] = append(byDomain[c.Domain], c)
	}
	totalCompacted := 0
	results := []map[string]any{}
	warnings := []string{}
	for _, d := range domains {
		path, e := ExpertisePath(a.Cwd, d)
		if e != nil {
			return e
		}
		archive := []Record{}
		e = WithFileLock(path, func() error {
			records, e := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
			if e != nil {
				return e
			}
			current := append([]Record{}, records...)
			for _, candidate := range byDomain[d] {
				sources := []Record{}
				for _, r := range current {
					if Text(r["type"]) != candidate.Type {
						continue
					}
					for _, cr := range candidate.Records {
						same := cr.ID == nil && r["id"] == nil || cr.ID != nil && *cr.ID == Text(r["id"])
						if same {
							sources = append(sources, r)
							break
						}
					}
				}
				if len(sources) < 2 {
					continue
				}
				replacement, e := a.compactResolveReplacement(cfg, d, candidate.Type, sources, nil)
				if e != nil {
					warnings = append(warnings, d+"/"+candidate.Type+": "+e.Error())
					continue
				}
				remove := map[string]bool{}
				for _, r := range sources {
					remove[Text(r["id"])] = true
				}
				kept := []Record{}
				for _, r := range current {
					if !remove[Text(r["id"])] {
						kept = append(kept, r)
					}
				}
				current = append(kept, replacement)
				archive = append(archive, sources...)
				totalCompacted += len(sources)
				result := map[string]any{"domain": d, "type": candidate.Type, "count": len(sources)}
				rememberOrder(result, []string{"domain", "type", "count"})
				results = append(results, result)
			}
			if len(current) != len(records) {
				return WriteRecords(path, current)
			}
			return nil
		})
		if e != nil {
			return e
		}
		if len(archive) > 0 {
			if e = ArchiveRecords(a.Cwd, d, archive, time.Now(), "compacted", a.Registry); e != nil {
				return e
			}
		}
	}
	if a.JSONMode {
		data := map[string]any{"action": "auto", "compacted": totalCompacted, "results": results}
		if len(warnings) > 0 {
			data["warnings"] = warnings
		}
		a.OK("compact", data)
	} else {
		a.Human("\n" + Style("brand", "✓") + " " + Style("brand", fmt.Sprintf("Auto-compacted %d records across %d groups", totalCompacted, len(results))))
		for _, r := range results {
			a.Human(Style("dim", fmt.Sprintf("  %s/%s: %v records → 1", Text(r["domain"]), Text(r["type"]), r["count"])))
		}
		if !a.Quiet {
			for _, w := range warnings {
				a.Warning(w)
			}
		}
	}
	return nil
}

func (a *App) compactResolveReplacement(cfg Config, domain, typ string, sources []Record, fallback func() (Record, error)) (Record, error) {
	payload := map[string]any{"domain": domain, "type": typ, "records": sources}
	rememberOrder(payload, []string{"domain", "type", "records"})
	hook, err := RunHooks(a.Cwd, cfg, "pre-compact", payload, a.Err)
	if err != nil {
		return nil, err
	}
	if hook.Blocked {
		return nil, fmt.Errorf("%s", writeFallback(hook.BlockReason, "pre-compact hook blocked the compaction"))
	}
	if hook.RanAny {
		if raw := writeObject(Obj(hook.Payload)["replacement"]); raw != nil {
			candidate := CloneRecord(Record(raw))
			if len(List(candidate["supersedes"])) == 0 {
				candidate["supersedes"] = compactSourceIDs(sources)
			}
			compactDropID(candidate)
			if issues := ValidateRecord(candidate, a.Registry); len(issues) > 0 {
				return nil, fmt.Errorf("pre-compact hook produced an invalid record: %s", writeValidation(issues))
			}
			candidate["id"] = GenerateRecordID(candidate, a.Registry)
			return candidate, nil
		}
	}
	if fallback != nil {
		return fallback()
	}
	return MergeRecords(sources, a.Registry)
}
func compactDropID(record Record) {
	keys := []string{}
	for _, key := range ObjectKeys(record) {
		if key != "id" {
			keys = append(keys, key)
		}
	}
	delete(record, "id")
	rememberOrder(map[string]any(record), keys)
}
func compactSourceIDs(records []Record) []string {
	ids := []string{}
	for _, r := range records {
		if v := Text(r["id"]); v != "" {
			ids = append(ids, v)
		}
	}
	return ids
}

func MergeRecords(records []Record, registry *Registry) (Record, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("Cannot merge empty record list")
	}
	first := records[0]
	def := registry.Types[Text(first["type"])]
	if def == nil {
		return nil, fmt.Errorf("Unknown record type: %s", Text(first["type"]))
	}
	if def.Compact == "manual" {
		return nil, fmt.Errorf("Type %q has compact strategy \"manual\" — use `luuna-atlas compact --apply` to compact manually.", def.Name)
	}
	tags, files := []string{}, []string{}
	for _, r := range records {
		tags = append(tags, List(r["tags"])...)
		if def.ExtractsFiles {
			files = append(files, List(r[def.FilesField])...)
		}
	}
	tags = writeUnique(tags)
	files = writeUnique(files)
	var result Record
	switch def.Compact {
	case "concat":
		result = Record{"type": def.Name, "classification": "foundational", "recorded_at": NowISO(), "supersedes": compactSourceIDs(records)}
		if len(tags) > 0 {
			result["tags"] = tags
		}
		if def.ExtractsFiles && len(files) > 0 {
			result[def.FilesField] = files
		}
		for _, field := range def.Required {
			values := []string{}
			for _, r := range records {
				if v, ok := r[field].(string); ok {
					values = append(values, v)
				}
			}
			if field == "name" || field == "title" {
				longest := ""
				if len(values) > 0 {
					longest = values[0]
				}
				for _, v := range values {
					if formatLen(v) > formatLen(longest) {
						longest = v
					}
				}
				result[field] = longest
			} else {
				result[field] = strings.Join(values, "\n\n")
			}
		}
		fieldOrder := []string{"type", "classification", "recorded_at", "supersedes", "tags"}
		if def.ExtractsFiles {
			fieldOrder = append(fieldOrder, def.FilesField)
		}
		rememberOrder(map[string]any(result), Unique(append(fieldOrder, def.Required...)))
	case "keep_latest", "merge_outcomes":
		sorted := append([]Record{}, records...)
		sort.SliceStable(sorted, func(i, j int) bool {
			return ParseDate(Text(sorted[i]["recorded_at"])).After(ParseDate(Text(sorted[j]["recorded_at"])))
		})
		result = CloneRecord(sorted[0])
		fieldOrder := ObjectKeys(result)
		result["classification"] = "foundational"
		result["recorded_at"] = NowISO()
		result["supersedes"] = compactSourceIDs(records)
		if len(tags) > 0 {
			result["tags"] = tags
		} else {
			delete(result, "tags")
		}
		if def.ExtractsFiles && len(files) > 0 {
			result[def.FilesField] = files
		}
		rememberOrder(map[string]any(result), Unique(append(fieldOrder, "classification", "recorded_at", "supersedes", "tags", def.FilesField)))
		compactDropID(result)
		if def.Compact == "merge_outcomes" {
			all := []Record{}
			for _, r := range records {
				all = append(all, writeRecordsValue(r["outcomes"])...)
			}
			if len(all) > 0 {
				result["outcomes"] = all
			}
		}
	default:
		return nil, fmt.Errorf("Unknown compact strategy: %s", def.Compact)
	}
	rememberOrder(map[string]any(result), ObjectKeys(result))
	result["id"] = GenerateRecordID(result, registry)
	return result, nil
}

func (a *App) compactApply(domain string, i Invocation) error {
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	if !writeHasDomain(cfg, domain) {
		return fmt.Errorf("Domain %q not found in config.", domain)
	}
	if !i.Has("records") {
		return fmt.Errorf("--records is required for --apply.")
	}
	path, err := ExpertisePath(a.Cwd, domain)
	if err != nil {
		return err
	}
	toArchive := []Record{}
	err = WithFileLock(path, func() error {
		records, e := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
		if e != nil {
			return e
		}
		indices := []int{}
		for _, id := range SplitCSV(i.String("records")) {
			n, e := ResolveRecordID(records, id)
			if e != nil {
				return e
			}
			indices = append(indices, n)
		}
		if len(indices) < 2 {
			return fmt.Errorf("Compaction requires at least 2 records.")
		}
		typ := i.String("type")
		if typ == "" {
			typ = Text(records[indices[0]]["type"])
		}
		if typ == "" {
			typ = "convention"
		}
		sources := []Record{}
		for _, n := range indices {
			sources = append(sources, records[n])
		}
		payload := map[string]any{"domain": domain, "type": typ, "records": sources}
		rememberOrder(payload, []string{"domain", "type", "records"})
		hook, e := RunHooks(a.Cwd, cfg, "pre-compact", payload, a.Err)
		if e != nil {
			return e
		}
		if hook.Blocked {
			return fmt.Errorf("%s", writeFallback(hook.BlockReason, "pre-compact hook blocked the compaction"))
		}
		if !a.JSONMode {
			for _, w := range hook.Warnings {
				a.Warning(w)
			}
		}
		var replacement Record
		if hook.RanAny {
			if raw := writeObject(Obj(hook.Payload)["replacement"]); raw != nil {
				replacement = CloneRecord(Record(raw))
				if len(List(replacement["supersedes"])) == 0 {
					replacement["supersedes"] = compactSourceIDs(sources)
				}
				compactDropID(replacement)
				if issues := ValidateRecord(replacement, a.Registry); len(issues) > 0 {
					return fmt.Errorf("pre-compact hook produced an invalid record: %s", writeValidation(issues))
				}
				replacement["id"] = GenerateRecordID(replacement, a.Registry)
			}
		}
		if replacement == nil {
			replacement = Record{"type": typ, "classification": "foundational", "recorded_at": NowISO()}
			replacementOrder := []string{"type"}
			if typ == "convention" {
				content := i.String("content")
				if !i.Has("content") {
					content = i.String("description")
				}
				if content == "" {
					return fmt.Errorf("Replacement convention requires --content or --description.")
				}
				replacement["content"] = content
				replacementOrder = append(replacementOrder, "content")
			} else {
				fields := map[string][]string{"pattern": {"name", "description"}, "failure": {"description", "resolution"}, "decision": {"title", "rationale"}, "reference": {"name", "description"}, "guide": {"name", "description"}}[typ]
				if len(fields) == 0 {
					return fmt.Errorf("Unknown record type %q.", typ)
				}
				missing := false
				for _, f := range fields {
					if i.String(f) == "" {
						missing = true
					}
					replacement[f] = i.String(f)
				}
				replacementOrder = append(replacementOrder, fields...)
				if missing {
					flags := []string{}
					for _, f := range fields {
						flags = append(flags, "--"+f)
					}
					return fmt.Errorf("Replacement %s requires %s.", typ, strings.Join(flags, " and "))
				}
			}
			if ids := compactSourceIDs(sources); len(ids) > 0 {
				replacement["supersedes"] = ids
			}
			rememberOrder(map[string]any(replacement), append(replacementOrder, "classification", "recorded_at", "supersedes"))
			replacement["id"] = GenerateRecordID(replacement, a.Registry)
			if issues := ValidateRecord(replacement, a.Registry); len(issues) > 0 {
				return fmt.Errorf("Replacement record failed validation: %s", writeValidation(issues))
			}
		}
		remove := map[int]bool{}
		for _, n := range indices {
			remove[n] = true
		}
		remaining := []Record{}
		for n, r := range records {
			if !remove[n] {
				remaining = append(remaining, r)
			}
		}
		remaining = append(remaining, replacement)
		if e = WriteRecords(path, remaining); e != nil {
			return e
		}
		toArchive = sources
		if a.JSONMode {
			a.OK("compact", map[string]any{"action": "applied", "domain": domain, "removed": len(indices), "replacement": replacement})
		} else {
			a.writeSuccess(fmt.Sprintf("Compacted %d %s records into 1 in %s", len(indices), typ, domain))
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(toArchive) > 0 {
		return ArchiveRecords(a.Cwd, domain, toArchive, time.Now(), "compacted", a.Registry)
	}
	return nil
}

func compactDaysSince(t time.Time) int { return int(math.Floor(time.Since(t).Hours() / 24)) }
