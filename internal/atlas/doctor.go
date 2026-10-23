package atlas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type doctorCheck struct {
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Fixable bool     `json:"fixable"`
	Details []string `json:"details"`
}
type doctorLine struct {
	Domain string
	LineNo int
	Line   string
	Parsed any
	Valid  bool
}

func doctorMake(name, status, message string, fixable bool, details []string) doctorCheck {
	if details == nil {
		details = []string{}
	}
	return doctorCheck{Name: name, Status: status, Message: message, Fixable: fixable, Details: details}
}
func (a *App) doctorConfig() doctorCheck {
	dir, err := AtlasDir(a.Cwd)
	if err != nil {
		return doctorMake("config", "fail", "Config error: "+err.Error(), false, nil)
	}
	if _, e := os.Stat(dir); e != nil {
		return doctorMake("config", "fail", "No .luuna/atlas/ directory found", false, nil)
	}
	if _, err = ReadConfig(a.Cwd); err != nil {
		return doctorMake("config", "fail", "Config error: "+err.Error(), false, nil)
	}
	return doctorMake("config", "pass", "Config is valid", false, nil)
}
func (a *App) doctorLines(cfg Config) []doctorLine {
	out := []doctorLine{}
	for _, domain := range DomainNames(cfg) {
		path, e := ExpertisePath(a.Cwd, domain)
		if e != nil {
			continue
		}
		b, e := os.ReadFile(path)
		if e != nil {
			continue
		}
		for n, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parsed, e := ParseJSON([]byte(line))
			out = append(out, doctorLine{Domain: domain, LineNo: n + 1, Line: line, Parsed: parsed, Valid: e == nil})
		}
	}
	return out
}
func (a *App) runDoctor(i Invocation) error {
	configCheck := a.doctorConfig()
	if configCheck.Status == "fail" {
		if a.JSONMode {
			a.Emit(map[string]any{"success": false, "command": "doctor", "checks": []doctorCheck{configCheck}, "summary": map[string]int{"pass": 0, "warn": 0, "fail": 1}})
		} else {
			a.Human("Atlas Doctor")
			fmt.Fprintln(a.Err, "  "+Style("red", "✗")+" "+Style("red", configCheck.Message))
			a.Human("\n0 passed, 0 warnings, 1 failed")
		}
		a.ExitCode = 1
		return nil
	}
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	checks := []doctorCheck{configCheck}
	lines := a.doctorLines(cfg)
	details := []string{}
	for _, line := range lines {
		if !line.Valid {
			details = append(details, fmt.Sprintf("%s:%d - Invalid JSON", line.Domain, line.LineNo))
		}
	}
	check := doctorMake("jsonl-integrity", "pass", "All JSONL lines are valid JSON", true, nil)
	if len(details) > 0 {
		check = doctorMake("jsonl-integrity", "fail", fmt.Sprintf("%d invalid JSON line(s) found", len(details)), true, details)
	}
	checks = append(checks, check)
	details = []string{}
	for _, line := range lines {
		if !line.Valid {
			continue
		}
		raw := writeObject(line.Parsed)
		_, legacy := raw["outcome"]
		_, modern := raw["outcomes"]
		if legacy && !modern {
			details = append(details, fmt.Sprintf("%s:%d - legacy \"outcome\" field (singular); should be \"outcomes[]\"", line.Domain, line.LineNo))
		}
	}
	check = doctorMake("legacy-outcome", "pass", `No legacy "outcome" fields on disk`, true, nil)
	if len(details) > 0 {
		check = doctorMake("legacy-outcome", "warn", fmt.Sprintf("%d record(s) with legacy \"outcome\" field on disk", len(details)), true, details)
	}
	checks = append(checks, check)
	details = []string{}
	for _, line := range lines {
		if !line.Valid {
			continue
		}
		raw := writeObject(line.Parsed)
		if typ, ok := raw["type"].(string); ok {
			def := a.Registry.Types[typ]
			if def == nil {
				continue
			}
			ApplyAliases(raw, def.Aliases)
		}
		issues := ValidateRecord(Record(raw), a.Registry)
		if len(issues) > 0 {
			details = append(details, fmt.Sprintf("%s:%d - %s", line.Domain, line.LineNo, writeValidation(issues)))
		}
	}
	check = doctorMake("schema-validation", "pass", "All records pass schema validation", true, nil)
	if len(details) > 0 {
		check = doctorMake("schema-validation", "fail", fmt.Sprintf("%d record(s) failed schema validation", len(details)), true, details)
	}
	checks = append(checks, check)
	details = []string{}
	for _, line := range lines {
		if !line.Valid {
			continue
		}
		raw := writeObject(line.Parsed)
		typ, ok := raw["type"].(string)
		if ok && a.Registry.Types[typ] == nil {
			idPart := ""
			if id, ok := raw["id"].(string); ok {
				idPart = " [" + id + "]"
			}
			details = append(details, fmt.Sprintf("%s:%d%s - unknown type %q", line.Domain, line.LineNo, idPart, typ))
		}
	}
	check = doctorMake("unknown-types", "pass", "All record types are registered", false, nil)
	if len(details) > 0 {
		check = doctorMake("unknown-types", "fail", fmt.Sprintf("%d record(s) with unregistered types", len(details)), false, details)
	}
	checks = append(checks, check)
	domainRows := map[string][]Record{}
	counts := map[string]int{}
	for _, name := range a.Registry.Order {
		counts[name] = 0
	}
	for _, domain := range DomainNames(cfg) {
		path, e := ExpertisePath(a.Cwd, domain)
		if e != nil {
			return e
		}
		rows, e := ReadRecords(path, a.Registry, true)
		if e != nil {
			return e
		}
		domainRows[domain] = rows
		for _, r := range rows {
			typ := Text(r["type"])
			if _, ok := counts[typ]; ok {
				counts[typ]++
			}
		}
	}
	builtins, customs, disabled := 0, 0, 0
	details = []string{}
	for _, name := range a.Registry.Order {
		def := a.Registry.Types[name]
		label := "custom"
		if def.Builtin {
			label = "built-in"
			builtins++
		} else {
			customs++
		}
		if def.Disabled {
			label += ", disabled"
			disabled++
		}
		plural := "s"
		if counts[name] == 1 {
			plural = ""
		}
		details = append(details, fmt.Sprintf("%s (%s): %d record%s", name, label, counts[name], plural))
	}
	message := fmt.Sprintf("%d type(s) registered: %d built-in, %d custom", builtins+customs, builtins, customs)
	if disabled > 0 {
		message += fmt.Sprintf(", %d disabled", disabled)
	}
	checks = append(checks, doctorMake("type-registry", "pass", message, false, details))
	details = []string{}
	for _, domain := range DomainNames(cfg) {
		for _, issue := range FindIncompatibleRequiredFields(cfg, domain, a.Registry) {
			details = append(details, fmt.Sprintf("domain %q requires field %q, but no allowed type accepts it (allowed: %s). Declare a custom_type with %q in required/optional, or remove %q from required_fields.", domain, issue.Field, strings.Join(issue.AllowedTypes, ", "), issue.Field, issue.Field))
		}
	}
	check = doctorMake("domain-rules-compatibility", "pass", "All domain required_fields are compatible with allowed types", false, nil)
	if len(details) > 0 {
		check = doctorMake("domain-rules-compatibility", "fail", fmt.Sprintf("%d domain required_field(s) incompatible with allowed types", len(details)), false, details)
	}
	checks = append(checks, check)
	conformance := []string{}
	violations := []string{}
	totalRecords, totalBad := 0, 0
	for _, domain := range DomainNames(cfg) {
		allowed, required := GetAllowedTypes(cfg, domain), GetRequiredFields(cfg, domain)
		total, bad := 0, 0
		for _, line := range lines {
			if line.Domain != domain || !line.Valid {
				continue
			}
			raw := writeObject(line.Parsed)
			if raw == nil {
				continue
			}
			total++
			typ := "<unknown>"
			if t, ok := raw["type"].(string); ok {
				typ = t
			}
			id := "<no-id>"
			if v, ok := raw["id"].(string); ok {
				id = v
			}
			recordBad := false
			if len(allowed) > 0 && !Contains(allowed, typ) {
				recordBad = true
				violations = append(violations, fmt.Sprintf("%s:%d [%s] type %q not in allowed_types [%s]", domain, line.LineNo, id, typ, strings.Join(allowed, ", ")))
			}
			missing := FindMissingDomainFields(Record(raw), required)
			if len(missing) > 0 {
				recordBad = true
				violations = append(violations, fmt.Sprintf("%s:%d [%s] (%s) missing required field(s): %s", domain, line.LineNo, id, typ, strings.Join(missing, ", ")))
			}
			if recordBad {
				bad++
			}
		}
		ruleLabel := "no rules"
		if len(allowed)+len(required) > 0 {
			ruleLabel = "rules"
		}
		plural := "s"
		if bad == 1 {
			plural = ""
		}
		conformance = append(conformance, fmt.Sprintf("%s (%s): %d/%d conforming, %d violation%s", domain, ruleLabel, total-bad, total, bad, plural))
		totalRecords += total
		totalBad += bad
	}
	checks = append(checks, doctorMake("domain-conformance", "pass", fmt.Sprintf("%d/%d record(s) conform to domain rules", totalRecords-totalBad, totalRecords), false, conformance))
	check = doctorMake("domain-violations", "pass", "All records conform to domain rules", false, nil)
	if len(violations) > 0 {
		check = doctorMake("domain-violations", "fail", fmt.Sprintf("%d record(s) violate domain rules (allowed_types / required_fields)", totalBad), false, violations)
	}
	checks = append(checks, check)
	details = []string{}
	now := time.Now()
	shelf := Obj(ConfigValue(cfg, "classification_defaults", "shelf_life"))
	for _, domain := range DomainNames(cfg) {
		for _, r := range domainRows[domain] {
			if IsRecordStale(r, now, shelf) {
				details = append(details, fmt.Sprintf("%s: stale %s (%s)", domain, Text(r["type"]), Text(r["classification"])))
			}
		}
	}
	check = doctorMake("stale-records", "pass", "No stale records", true, nil)
	if len(details) > 0 {
		check = doctorMake("stale-records", "warn", fmt.Sprintf("%d stale record(s) found", len(details)), true, details)
	}
	checks = append(checks, check)
	atlasDir, err := AtlasDir(a.Cwd)
	if err != nil {
		return err
	}
	expertiseDir := filepath.Join(atlasDir, "expertise")
	details = []string{}
	entries, _ := os.ReadDir(expertiseDir)
	for _, entry := range entries {
		file := entry.Name()
		if strings.HasSuffix(file, ".jsonl") {
			domain := strings.Replace(file, ".jsonl", "", 1)
			if !writeHasDomain(cfg, domain) {
				details = append(details, fmt.Sprintf("File %q exists but domain %q is not in config", file, domain))
			}
		}
	}
	for _, domain := range DomainNames(cfg) {
		path, e := ExpertisePath(a.Cwd, domain)
		if e != nil {
			return e
		}
		if _, e = os.Stat(path); e != nil {
			details = append(details, fmt.Sprintf("Domain %q in config but no JSONL file exists", domain))
		}
	}
	check = doctorMake("orphaned-domains", "pass", "No orphaned domains", true, nil)
	if len(details) > 0 {
		check = doctorMake("orphaned-domains", "warn", fmt.Sprintf("%d orphaned domain issue(s)", len(details)), true, details)
	}
	checks = append(checks, check)
	details = []string{}
	for _, domain := range DomainNames(cfg) {
		records := domainRows[domain]
		for n := 1; n < len(records); n++ {
			if duplicate := FindDuplicate(records[:n], records[n], a.Registry); duplicate >= 0 {
				details = append(details, fmt.Sprintf("%s: duplicate %s at index %d (matches #%d)", domain, Text(records[n]["type"]), n+1, duplicate+1))
			}
		}
	}
	check = doctorMake("duplicates", "pass", "No duplicates", false, nil)
	if len(details) > 0 {
		check = doctorMake("duplicates", "warn", fmt.Sprintf("%d duplicate record(s) found", len(details)), false, details)
	}
	checks = append(checks, check)
	details = []string{}
	for _, domain := range DomainNames(cfg) {
		for _, r := range domainRows[domain] {
			id := ""
			if v := Text(r["id"]); v != "" {
				id = "[" + v + "]"
			}
			for _, f := range []struct{ key, label string }{{"files", "files[] path not found"}, {"dir_anchors", "dir_anchors[] path not found"}} {
				for _, p := range List(r[f.key]) {
					if !doctorPathExists(a.Cwd, p) {
						details = append(details, fmt.Sprintf("%s%s: %s: %s", domain, id, f.label, p))
					}
				}
			}
			if p := Text(Obj(r["evidence"])["file"]); p != "" && !doctorPathExists(a.Cwd, p) {
				details = append(details, fmt.Sprintf("%s%s: evidence.file not found: %s", domain, id, p))
			}
		}
	}
	check = doctorMake("file-anchors", "pass", "All file anchors resolve to existing paths", true, nil)
	if len(details) > 0 {
		check = doctorMake("file-anchors", "warn", fmt.Sprintf("%d broken file anchor(s) found", len(details)), true, details)
	}
	checks = append(checks, check)
	details = []string{}
	worst := "pass"
	governance := Obj(cfg["governance"])
	for _, domain := range DomainNames(cfg) {
		n := len(domainRows[domain])
		if float64(n) >= Number(governance["hard_limit"]) {
			details = append(details, fmt.Sprintf("%s: %d records (over hard limit of %v)", domain, n, governance["hard_limit"]))
			worst = "fail"
		} else if float64(n) >= Number(governance["warn_entries"]) {
			details = append(details, fmt.Sprintf("%s: %d records (over warn threshold of %v)", domain, n, governance["warn_entries"]))
			if worst != "fail" {
				worst = "warn"
			}
		} else if float64(n) >= Number(governance["max_entries"]) {
			details = append(details, fmt.Sprintf("%s: %d records (approaching limit of %v)", domain, n, governance["max_entries"]))
			if worst != "fail" {
				worst = "warn"
			}
		}
	}
	check = doctorMake("governance", "pass", "All domains within governance limits", false, nil)
	if len(details) > 0 {
		check = doctorMake("governance", worst, fmt.Sprintf("%d domain(s) over governance thresholds", len(details)), false, details)
	}
	checks = append(checks, check)
	anchorValue := ConfigValue(cfg, "decay", "anchor_validity")
	check = doctorMake("decay-config", "pass", "decay.anchor_validity uses defaults", false, nil)
	if anchorValue != nil {
		errors := writeAnchorConfigErrors(Obj(anchorValue))
		if len(errors) > 0 {
			plural := "s"
			if len(errors) == 1 {
				plural = ""
			}
			check = doctorMake("decay-config", "fail", fmt.Sprintf("decay.anchor_validity is invalid (%d issue%s)", len(errors), plural), false, errors)
		} else {
			check = doctorMake("decay-config", "pass", "decay.anchor_validity is valid", false, nil)
		}
	}
	checks = append(checks, check)
	scripts := []string{}
	for _, s := range List(ConfigValue(cfg, "hooks", "pre-compact")) {
		if strings.TrimSpace(s) != "" {
			scripts = append(scripts, s)
		}
	}
	check = doctorMake("compact-summarizer", "pass", "compact-summarizer: not configured (mechanical merge in use)", false, nil)
	if len(scripts) > 0 {
		plural := "s"
		if len(scripts) == 1 {
			plural = ""
		}
		check = doctorMake("compact-summarizer", "pass", fmt.Sprintf("compact-summarizer: %d pre-compact hook%s registered", len(scripts), plural), false, scripts)
	}
	checks = append(checks, check)
	checks = append(checks, doctorMake("upgrade", "pass", "Version "+Version+" (unable to check registry)", false, nil))
	summary := map[string]int{"pass": 0, "warn": 0, "fail": 0}
	for _, c := range checks {
		summary[c.Status]++
	}
	fixed := []string{}
	if i.Bool("fix") {
		fixed, err = a.doctorFix(checks, cfg)
		if err != nil {
			return err
		}
	}
	if a.JSONMode {
		data := map[string]any{"success": summary["fail"] == 0, "command": "doctor", "checks": checks, "summary": summary}
		if i.Bool("fix") {
			data["fixed"] = fixed
		}
		a.Emit(data)
	} else {
		a.Human("Atlas Doctor")
		for _, c := range checks {
			icon := Style("brand", "✓")
			if c.Status == "warn" {
				icon = Style("yellow", "!")
			} else if c.Status == "fail" {
				icon = Style("red", "✗")
			}
			a.Human("  " + icon + " " + c.Message)
			if c.Status != "pass" {
				for _, detail := range c.Details {
					a.Human("      " + detail)
				}
			}
		}
		a.Human(fmt.Sprintf("\n%d passed, %d warning(s), %d failed", summary["pass"], summary["warn"], summary["fail"]))
		if len(fixed) > 0 {
			a.Human("\n" + Style("brand", "Fixed:"))
			for _, f := range fixed {
				a.Human("  " + Style("brand", "✓") + " " + f)
			}
		}
	}
	if summary["fail"] > 0 {
		a.ExitCode = 1
	}
	return nil
}
func doctorPathExists(cwd, path string) bool {
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	_, e := os.Stat(path)
	return e == nil
}

func (a *App) doctorFix(checks []doctorCheck, cfg Config) ([]string, error) {
	fixed := []string{}
	for _, check := range checks {
		if check.Status == "pass" || !check.Fixable {
			continue
		}
		switch check.Name {
		case "jsonl-integrity", "legacy-outcome":
			for _, domain := range DomainNames(cfg) {
				path, e := ExpertisePath(a.Cwd, domain)
				if e != nil {
					return nil, e
				}
				e = WithFileLock(path, func() error {
					bytes, e := os.ReadFile(path)
					if e != nil {
						return nil
					}
					out := []string{}
					changed := 0
					for _, line := range strings.Split(string(bytes), "\n") {
						trim := strings.TrimSpace(line)
						if trim == "" {
							continue
						}
						parsed, e := ParseJSON([]byte(trim))
						if check.Name == "jsonl-integrity" {
							if e != nil {
								changed++
								continue
							}
							out = append(out, trim)
							continue
						}
						if e != nil {
							out = append(out, trim)
							continue
						}
						raw := writeObject(parsed)
						legacy, hasLegacy := raw["outcome"]
						_, modern := raw["outcomes"]
						if hasLegacy && legacy != nil && !modern {
							rewritten := CloneRecord(Record(raw))
							delete(rewritten, "outcome")
							o := Record{}
							old := Obj(legacy)
							for _, field := range []string{"status", "duration", "test_results", "agent"} {
								if v, ok := old[field]; ok {
									o[field] = v
								}
							}
							rewritten["outcomes"] = []Record{o}
							b, e := MarshalJSON(rewritten)
							if e != nil {
								return e
							}
							out = append(out, string(b))
							changed++
						} else {
							out = append(out, trim)
						}
					}
					if changed > 0 {
						body := strings.Join(out, "\n")
						if len(out) > 0 {
							body += "\n"
						}
						if e = os.WriteFile(path, []byte(body), 0666); e != nil {
							return e
						}
						if check.Name == "jsonl-integrity" {
							fixed = append(fixed, fmt.Sprintf("Removed %d invalid JSON line(s) from %s", changed, domain))
						} else {
							fixed = append(fixed, fmt.Sprintf("Migrated %d legacy \"outcome\" field(s) to \"outcomes[]\" in %s", changed, domain))
						}
					}
					return nil
				})
				if e != nil {
					return nil, e
				}
			}
		case "schema-validation", "stale-records", "file-anchors":
			for _, domain := range DomainNames(cfg) {
				path, e := ExpertisePath(a.Cwd, domain)
				if e != nil {
					return nil, e
				}
				e = WithFileLock(path, func() error {
					records, e := ReadRecords(path, a.Registry, true)
					if e != nil {
						return e
					}
					kept := []Record{}
					changed := 0
					for _, r := range records {
						if check.Name == "schema-validation" {
							if a.Registry.Types[Text(r["type"])] != nil && len(ValidateRecord(r, a.Registry)) > 0 {
								changed++
								continue
							}
							kept = append(kept, r)
						} else if check.Name == "stale-records" {
							if IsRecordStale(r, time.Now(), Obj(ConfigValue(cfg, "classification_defaults", "shelf_life"))) {
								changed++
								continue
							}
							kept = append(kept, r)
						} else {
							nr := CloneRecord(r)
							for _, field := range []string{"files", "dir_anchors"} {
								if _, ok := nr[field]; !ok {
									continue
								}
								before := List(nr[field])
								remaining := []string{}
								for _, p := range before {
									if doctorPathExists(a.Cwd, p) {
										remaining = append(remaining, p)
									}
								}
								if len(remaining) < len(before) {
									changed += len(before) - len(remaining)
									if len(remaining) > 0 {
										nr[field] = remaining
									} else {
										delete(nr, field)
									}
								}
							}
							if evidence := Obj(nr["evidence"]); Text(evidence["file"]) != "" && !doctorPathExists(a.Cwd, Text(evidence["file"])) {
								changed++
								delete(evidence, "file")
								nr["evidence"] = evidence
							}
							kept = append(kept, nr)
						}
					}
					if changed > 0 {
						if e = WriteRecords(path, kept); e != nil {
							return e
						}
						switch check.Name {
						case "schema-validation":
							fixed = append(fixed, fmt.Sprintf("Removed %d invalid record(s) from %s", changed, domain))
						case "stale-records":
							fixed = append(fixed, fmt.Sprintf("Pruned %d stale record(s) from %s", changed, domain))
						case "file-anchors":
							fixed = append(fixed, fmt.Sprintf("Removed %d broken file anchor(s) from %s", changed, domain))
						}
					}
					return nil
				})
				if e != nil {
					return nil, e
				}
			}
		case "orphaned-domains":
			dir, e := AtlasDir(a.Cwd)
			if e != nil {
				return nil, e
			}
			entries, _ := os.ReadDir(filepath.Join(dir, "expertise"))
			domains := Obj(cfg["domains"])
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".jsonl") {
					domain := strings.Replace(entry.Name(), ".jsonl", "", 1)
					if _, ok := domains[domain]; !ok {
						keys := ObjectKeys(domains)
						domains[domain] = map[string]any{}
						rememberOrder(domains, append(keys, domain))
						fixed = append(fixed, fmt.Sprintf("Added orphaned domain %q to config", domain))
					}
				}
			}
			for _, domain := range DomainNames(cfg) {
				path, e := ExpertisePath(a.Cwd, domain)
				if e != nil {
					return nil, e
				}
				if _, e = os.Stat(path); e != nil {
					if e = os.WriteFile(path, []byte{}, 0666); e != nil {
						return nil, e
					}
					fixed = append(fixed, fmt.Sprintf("Created missing JSONL file for domain %q", domain))
				}
			}
			if len(fixed) > 0 {
				if e = WriteConfig(a.Cwd, cfg); e != nil {
					return nil, e
				}
			}
		}
	}
	return fixed, nil
}
