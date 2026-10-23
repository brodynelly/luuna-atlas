package atlas

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var auditTrackers = []string{"missions", "gh", "linear", "bead"}

func auditThresholds(cfg Config, domain string) map[string]any {
	out := map[string]any{"evidence_coverage": .5, "evidence_coverage_warn": .3, "floater_max": .2, "rule_density_min": .25, "rule_density_warn": .15, "max_records_per_domain": 200, "max_stale": 0}
	audit := Obj(cfg["audit"])
	for k, v := range Obj(audit["thresholds"]) {
		out[k] = v
	}
	for k, v := range Obj(Obj(audit["per_domain"])[domain]) {
		out[k] = v
	}
	return out
}
func auditFloater(r Record) bool {
	e := Obj(r["evidence"])
	return EvidenceMissions(e) == "" && Text(e["gh"]) == "" && Text(e["linear"]) == "" && Text(e["bead"]) == "" && Text(e["commit"]) == "" && len(List(r["relates_to"])) == 0
}
func auditMetric(value, pass, warn float64, low bool) map[string]any {
	verdict := "FAIL"
	if low {
		if value <= pass {
			verdict = "PASS"
		} else if value <= warn {
			verdict = "WARN"
		}
	} else {
		if value >= pass {
			verdict = "PASS"
		} else if value >= warn {
			verdict = "WARN"
		}
	}
	out := map[string]any{"verdict": verdict, "value": value, "threshold": pass}
	if !low {
		out["warn_threshold"] = warn
	}
	return out
}

type auditCitation struct {
	Tracker, ID string
	Count       int
}

func (a *App) computeAudit(cfg Config, i Invocation) (map[string]any, []shownRecord, error) {
	allDomains := DomainNames(cfg)
	ignored := Unique(append(List(Obj(cfg["audit"])["ignore_domains"]), SplitCSV(i.String("ignore-domains"))...))
	domains := []string{}
	for _, d := range allDomains {
		if scope := i.String("domain"); scope != "" {
			if d == scope {
				domains = append(domains, d)
			}
		} else if !Contains(ignored, d) {
			domains = append(domains, d)
		}
	}
	items := []shownRecord{}
	for _, d := range domains {
		path, e := ExpertisePath(a.Cwd, d)
		if e != nil {
			return nil, nil, e
		}
		records, e := ReadRecords(path, a.Registry, true)
		if e != nil {
			return nil, nil, e
		}
		for _, r := range records {
			items = append(items, shownRecord{d, r})
		}
	}
	total := len(items)
	thresholds := auditThresholds(cfg, i.String("domain"))
	perThresholds := map[string]any{}
	for _, d := range domains {
		perThresholds[d] = auditThresholds(cfg, d)
	}
	typeCounts := map[string]int{}
	typeOrder := []string{}
	withTracker := map[string]int{"missions": 0, "gh": 0, "linear": 0, "bead": 0}
	withCommit, withRelates, withAny, floaters := 0, 0, 0, 0
	citations := map[string]*auditCitation{}
	citationOrder := []string{}
	convTotal, convSignal := 0, 0
	for _, item := range items {
		r := item.Record
		t := Text(r["type"])
		if _, ok := typeCounts[t]; !ok {
			typeOrder = append(typeOrder, t)
		}
		typeCounts[t]++
		ev := Obj(r["evidence"])
		for _, tracker := range auditTrackers {
			if value := Text(ev[tracker]); value != "" {
				withTracker[tracker]++
				key := tracker + ":" + value
				if citations[key] == nil {
					citations[key] = &auditCitation{tracker, value, 0}
					citationOrder = append(citationOrder, key)
				}
				citations[key].Count++
			}
		}
		if Text(ev["commit"]) != "" {
			withCommit++
		}
		if len(List(r["relates_to"])) > 0 {
			withRelates++
		}
		if EvidenceMissions(ev) != "" || Text(ev["gh"]) != "" || Text(ev["linear"]) != "" || Text(ev["bead"]) != "" || Text(ev["commit"]) != "" {
			withAny++
		}
		if auditFloater(r) {
			floaters++
		}
		if t == "convention" {
			convTotal++
			if HasRuleSignal(Text(r["content"])) {
				convSignal++
			}
		}
	}
	sort.SliceStable(typeOrder, func(x, y int) bool { return typeCounts[typeOrder[x]] > typeCounts[typeOrder[y]] })
	mix := []any{}
	for _, t := range typeOrder {
		pct := 0
		if total > 0 {
			pct = 100 * typeCounts[t] / total
		}
		mix = append(mix, map[string]any{"type": t, "count": typeCounts[t], "pct": pct})
	}
	index := map[string]map[string]any{}
	missionsDir := ResolveMissionsDir(a.Cwd)
	if data, e := os.ReadFile(filepath.Join(missionsDir, "issues.jsonl")); e == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			v, e := ParseJSON([]byte(line))
			if e == nil {
				row := Obj(v)
				if id := Text(row["id"]); id != "" {
					index["missions:"+id] = row
				}
			}
		}
	}
	statusCounts, missionStatuses := map[string]int{}, map[string]int{}
	missing, missionMissing, missionUnique := 0, 0, 0
	missionOrder := []string{}
	for _, key := range citationOrder {
		c := citations[key]
		hit := index[key]
		if hit != nil {
			status := StringDefault(hit["status"], "?")
			statusCounts[status]++
			if c.Tracker == "missions" {
				missionStatuses[status]++
			}
		} else if c.Tracker == "missions" {
			missing++
			missionMissing++
		} else {
			statusCounts["unresolved"]++
		}
		if c.Tracker == "missions" {
			missionUnique++
			missionOrder = append(missionOrder, key)
		}
	}
	sortedCitations := append([]string{}, citationOrder...)
	sort.SliceStable(sortedCitations, func(x, y int) bool { return citations[sortedCitations[x]].Count > citations[sortedCitations[y]].Count })
	sort.SliceStable(missionOrder, func(x, y int) bool { return citations[missionOrder[x]].Count > citations[missionOrder[y]].Count })
	top := []any{}
	missionTop := []any{}
	for n, key := range sortedCitations {
		if n >= 8 {
			break
		}
		c := citations[key]
		hit := index[key]
		status, title := "unresolved", ""
		if c.Tracker == "missions" {
			status = "MISSING"
		}
		if hit != nil {
			status = StringDefault(hit["status"], "?")
			title = Text(hit["title"])
		}
		if len(title) > 55 {
			title = title[:55]
		}
		id := c.ID
		if c.Tracker == "gh" {
			id = "gh#" + strings.TrimPrefix(id, "#")
		} else if c.Tracker != "missions" {
			id = c.Tracker + ":" + id
		}
		top = append(top, map[string]any{"id": id, "tracker": c.Tracker, "count": c.Count, "status": status, "title": title})
	}
	for n, key := range missionOrder {
		if n >= 8 {
			break
		}
		c := citations[key]
		status, title := "MISSING", "MISSING"
		if hit := index[key]; hit != nil {
			status = StringDefault(hit["status"], "MISSING")
			title = StringDefault(hit["title"], "MISSING")
		}
		if len(title) > 55 {
			title = title[:55]
		}
		missionTop = append(missionTop, map[string]any{"id": c.ID, "count": c.Count, "status": status, "title": title})
	}
	var conventionQuality any
	if convTotal > 0 {
		conventionQuality = map[string]any{"total": convTotal, "with_rule_signal": convSignal, "likely_code_restatement": convTotal - convSignal}
	}
	groups := map[string][]Record{}
	domainOrder := []string{}
	for _, item := range items {
		if _, ok := groups[item.Domain]; !ok {
			domainOrder = append(domainOrder, item.Domain)
		}
		groups[item.Domain] = append(groups[item.Domain], item.Record)
	}
	byDomain := []map[string]any{}
	for _, domain := range domainOrder {
		records := groups[domain]
		counts := map[string]int{}
		first, last := "", ""
		for _, r := range records {
			counts[Text(r["type"])]++
			if ts := Text(r["recorded_at"]); ts != "" {
				if first == "" || ts < first {
					first = ts
				}
				if last == "" || ts > last {
					last = ts
				}
			}
		}
		var firstValue, lastValue any
		if first != "" {
			firstValue = first
		}
		if last != "" {
			lastValue = last
		}
		byDomain = append(byDomain, map[string]any{"domain": domain, "total": len(records), "type_counts": counts, "high_value_pct": 100 * (counts["failure"] + counts["decision"]) / len(records), "first_recorded_at": firstValue, "last_recorded_at": lastValue})
	}
	sort.SliceStable(byDomain, func(x, y int) bool { return Number(byDomain[x]["total"]) > Number(byDomain[y]["total"]) })
	weak := []string{}
	maxRecords, maxStale := []any{}, []any{}
	shelf := Obj(Obj(cfg["classification_defaults"])["shelf_life"])
	for _, row := range byDomain {
		domain := Text(row["domain"])
		if Number(row["total"]) >= 5 && Number(row["high_value_pct"]) < 20 {
			weak = append(weak, domain)
		}
		threshold := Obj(perThresholds[domain])
		rv, sv := "PASS", "PASS"
		if Number(row["total"]) > Number(threshold["max_records_per_domain"]) {
			rv = "WARN"
		}
		stale := 0
		for _, r := range groups[domain] {
			if IsRecordStale(r, time.Now(), shelf) {
				stale++
			}
		}
		if float64(stale) > Number(threshold["max_stale"]) {
			sv = "WARN"
		}
		maxRecords = append(maxRecords, map[string]any{"domain": domain, "count": row["total"], "verdict": rv})
		maxStale = append(maxStale, map[string]any{"domain": domain, "count": stale, "verdict": sv})
	}
	evRate, floaterRate := 0., 0.
	if total > 0 {
		evRate = float64(withAny) / float64(total)
		floaterRate = float64(floaters) / float64(total)
	}
	evMetric := auditMetric(evRate, Number(thresholds["evidence_coverage"]), Number(thresholds["evidence_coverage_warn"]), false)
	floaterMetric := auditMetric(floaterRate, Number(thresholds["floater_max"]), 2*Number(thresholds["floater_max"]), true)
	var ruleMetric any
	if convTotal > 0 {
		ruleMetric = auditMetric(float64(convSignal)/float64(convTotal), Number(thresholds["rule_density_min"]), Number(thresholds["rule_density_warn"]), false)
	}
	failures, warnings := []string{}, []string{}
	for _, metric := range []struct {
		name  string
		value any
	}{{"evidence_coverage", evMetric}, {"rule_density", ruleMetric}, {"floater_rate", floaterMetric}} {
		switch Text(Obj(metric.value)["verdict"]) {
		case "FAIL":
			failures = append(failures, metric.name)
		case "WARN":
			warnings = append(warnings, metric.name)
		}
	}
	for _, row := range maxRecords {
		if Text(Obj(row)["verdict"]) == "WARN" {
			warnings = append(warnings, "max_records_per_domain["+Text(Obj(row)["domain"])+"]")
		}
	}
	for _, row := range maxStale {
		if Text(Obj(row)["verdict"]) == "WARN" {
			warnings = append(warnings, "max_stale["+Text(Obj(row)["domain"])+"]")
		}
	}
	report := map[string]any{"repo": filepath.Base(filepath.Clean(a.Cwd)), "total_records": total, "domains": domains, "ignored_domains": ignored, "type_mix": mix, "evidence": map[string]any{"with_missions": withTracker["missions"], "with_tracker": withTracker, "with_any_tracker": withAny, "with_commit": withCommit, "with_relates": withRelates, "floaters": floaters}, "convention_quality": conventionQuality, "mission_citations": map[string]any{"unique": missionUnique, "status_counts": missionStatuses, "missing_in_missions": missionMissing, "top_cited": missionTop}, "tracker_citations": map[string]any{"unique": len(citations), "status_counts": statusCounts, "missing_in_index": missing, "top_cited": top}, "by_domain": byDomain, "weak_domains": weak, "thresholds": thresholds, "per_domain_thresholds": perThresholds, "signals": map[string]any{"evidence_coverage": evMetric, "rule_density": ruleMetric, "floater_rate": floaterMetric, "per_domain_max_records": maxRecords, "per_domain_max_stale": maxStale}, "failures": failures, "warnings": warnings}
	return report, items, nil
}
func auditSuggestions(items []shownRecord, cfg Config) map[string]any {
	archive, revise, attribute := []string{}, []string{}, []string{}
	shelf := Obj(Obj(cfg["classification_defaults"])["shelf_life"])
	for _, item := range items {
		r := item.Record
		id := Text(r["id"])
		if id == "" {
			continue
		}
		if Text(r["classification"]) != "foundational" && IsRecordStale(r, time.Now(), shelf) {
			archive = append(archive, id)
		}
		if Text(r["type"]) == "convention" && !HasRuleSignal(Text(r["content"])) {
			revise = append(revise, id)
		}
		if auditFloater(r) {
			attribute = append(attribute, id)
		}
	}
	groups := []any{}
	if len(archive) > 0 {
		groups = append(groups, map[string]any{"action": "archive", "headline": fmt.Sprintf("%d stale records past shelf life", len(archive)), "rationale": "These records are past their classification shelf life and are not foundational. Soft-archive them with `luuna-atlas prune` (or `luuna-atlas archive` once it lands in v0.10).", "record_ids": archive, "commands": []string{"luuna-atlas prune --ids " + strings.Join(archive, ",") + " --dry-run", "luuna-atlas prune --ids " + strings.Join(archive, ",")}})
	}
	if len(revise) > 0 {
		commands := []string{}
		for n, id := range revise {
			if n >= 10 {
				break
			}
			commands = append(commands, "luuna-atlas edit "+id+` --content "..."`)
		}
		groups = append(groups, map[string]any{"action": "revise", "headline": fmt.Sprintf("%d conventions lack rule-signal language", len(revise)), "rationale": "Conventions without rule-signal words (because, must not, avoid, always, never, …) often restate code rather than encode a rule. Revise the content or downgrade to a note.", "record_ids": revise, "commands": commands})
	}
	if len(attribute) > 0 {
		commands := []string{}
		for n, id := range attribute {
			if n >= 10 {
				break
			}
			commands = append(commands, "luuna-atlas edit "+id+" --evidence-missions <id>   # or --evidence-commit <sha> / --relates-to <mx-id>")
		}
		groups = append(groups, map[string]any{"action": "attribute", "headline": fmt.Sprintf("%d records have no evidence or relates_to link", len(attribute)), "rationale": "Floater records lack any tracker (missions/gh/linear/bead), commit, or relates_to link. Attribute them so future audits and pruning can follow the trail.", "record_ids": attribute, "commands": commands})
	}
	return map[string]any{"groups": groups}
}
func auditPct(value any) string { return fmt.Sprintf("%d%%", int(math.Floor(Number(value)*100+.5))) }
func auditVerdict(v string) string {
	style := "red"
	if v == "PASS" {
		style = "green"
	} else if v == "WARN" {
		style = "yellow"
	}
	return Style(style, v)
}
func auditHuman(report map[string]any) string {
	lines := []string{Style("brand", Style("bold", "=== atlas-audit: "+Text(report["repo"])+" ===")), "", fmt.Sprintf("total records: %d", int(Number(report["total_records"])))}
	if ignored := List(report["ignored_domains"]); len(ignored) > 0 {
		lines = append(lines, Style("muted", "(ignoring domains: "+strings.Join(ignored, ", ")+")"))
	}
	total := int(Number(report["total_records"]))
	if total == 0 {
		return strings.Join(append(lines, "", Style("muted", "no records — skipping metric computation")), "\n")
	}
	lines = append(lines, "", "type mix:")
	for _, value := range AnyList(report["type_mix"]) {
		r := Obj(value)
		lines = append(lines, fmt.Sprintf("  %s: %d (%d%%)", Text(r["type"]), int(Number(r["count"])), int(Number(r["pct"]))))
	}
	ev := Obj(report["evidence"])
	p := func(v any) int { return int(Number(v)) * 100 / total }
	lines = append(lines, "", "evidence coverage:", fmt.Sprintf("  with any tracker: %d (%d%%)", int(Number(ev["with_any_tracker"])), p(ev["with_any_tracker"])))
	trackerCounts := Obj(ev["with_tracker"])
	for _, t := range auditTrackers {
		if Number(trackerCounts[t]) > 0 {
			lines = append(lines, fmt.Sprintf("  - %s: %d (%d%%)", t, int(Number(trackerCounts[t])), p(trackerCounts[t])))
		}
	}
	lines = append(lines, fmt.Sprintf("  with commit-ev: %d (%d%%)", int(Number(ev["with_commit"])), p(ev["with_commit"])), fmt.Sprintf("  with relates_to: %d", int(Number(ev["with_relates"]))), fmt.Sprintf("  FLOATERS (no tracker/relates/commit): %d", int(Number(ev["floaters"]))))
	refs := Obj(report["tracker_citations"])
	if Number(refs["unique"]) > 0 {
		lines = append(lines, "", fmt.Sprintf("tracker-citation (unique ids cited: %d):", int(Number(refs["unique"]))))
		statuses := Obj(refs["status_counts"])
		keys := ObjectKeys(statuses)
		sort.SliceStable(keys, func(i, j int) bool { return Number(statuses[keys[i]]) > Number(statuses[keys[j]]) })
		for _, status := range keys {
			lines = append(lines, fmt.Sprintf("  %s: %d", status, int(Number(statuses[status]))))
		}
		if Number(refs["missing_in_index"]) > 0 {
			lines = append(lines, fmt.Sprintf("  not-found-in-missions.jsonl: %d", int(Number(refs["missing_in_index"]))))
		}
		lines = append(lines, fmt.Sprintf("  closed-citation rate: %d%%", int(Number(statuses["closed"])*100/Number(refs["unique"]))))
	}
	if quality := Obj(report["convention_quality"]); len(quality) > 0 {
		rate := int(Number(quality["with_rule_signal"]) * 100 / Number(quality["total"]))
		lines = append(lines, "", "convention quality:", fmt.Sprintf("  total conventions: %d", int(Number(quality["total"]))), fmt.Sprintf("  with rule-signal word: %d (%d%%)", int(Number(quality["with_rule_signal"])), rate), fmt.Sprintf("  likely code-restatement: %d (%d%%)", int(Number(quality["likely_code_restatement"])), 100-rate))
	}
	lines = append(lines, "", "per-domain mix (sorted by record count):", fmt.Sprintf("  %-22s %5s %4s %4s %4s %4s  high-value%%", "domain", "total", "fail", "dec", "pat", "conv"))
	byDomain := []map[string]any{}
	switch values := report["by_domain"].(type) {
	case []map[string]any:
		byDomain = values
	case []any:
		for _, v := range values {
			byDomain = append(byDomain, Obj(v))
		}
	}
	for _, d := range byDomain {
		counts := d["type_counts"].(map[string]int)
		lines = append(lines, fmt.Sprintf("  %-22s %5d %4d %4d %4d %4d  %3d%%", Text(d["domain"]), int(Number(d["total"])), counts["failure"], counts["decision"], counts["pattern"], counts["convention"], int(Number(d["high_value_pct"]))))
	}
	if len(AnyList(refs["top_cited"])) > 0 {
		lines = append(lines, "", "top-cited tracker ids:")
		for _, v := range AnyList(refs["top_cited"]) {
			r := Obj(v)
			lines = append(lines, fmt.Sprintf("  %s cited=%d status=%s  %s", Text(r["id"]), int(Number(r["count"])), Text(r["status"]), Text(r["title"])))
		}
	}
	dated := append([]map[string]any{}, byDomain...)
	sort.Slice(dated, func(i, j int) bool { return Text(dated[i]["domain"]) < Text(dated[j]["domain"]) })
	hasDate := false
	for _, d := range dated {
		if Text(d["first_recorded_at"]) != "" {
			if !hasDate {
				lines = append(lines, "", "domain age (first → last recorded_at):")
				hasDate = true
			}
			first, last := Text(d["first_recorded_at"]), Text(d["last_recorded_at"])
			if len(first) > 10 {
				first = first[:10]
			}
			if len(last) > 10 {
				last = last[:10]
			}
			lines = append(lines, fmt.Sprintf("  %-22s %s → %s  (%d records)", Text(d["domain"]), first, last, int(Number(d["total"]))))
		}
	}
	signals := Obj(report["signals"])
	lines = append(lines, "", Style("brand", "--- summary signals ---"))
	for _, metric := range []struct{ key, label, op string }{{"evidence_coverage", "evidence coverage", "≥"}, {"rule_density", "convention rule-density", "≥"}, {"floater_rate", "floater rate", "≤"}} {
		r := Obj(signals[metric.key])
		if len(r) > 0 {
			lines = append(lines, fmt.Sprintf("%s: %s  %s (target %s%s)", metric.label, auditPct(r["value"]), auditVerdict(Text(r["verdict"])), metric.op, auditPct(r["threshold"])))
		}
	}
	for _, metric := range []struct{ key, label string }{{"per_domain_max_records", "max_records_per_domain"}, {"per_domain_max_stale", "max_stale"}} {
		parts := []string{}
		for _, v := range AnyList(signals[metric.key]) {
			r := Obj(v)
			if Text(r["verdict"]) != "PASS" {
				parts = append(parts, Text(r["domain"])+"="+Text(r["count"]))
			}
		}
		if len(parts) > 0 {
			lines = append(lines, fmt.Sprintf("%s: %d over (%s)  %s", metric.label, len(parts), strings.Join(parts, ", "), auditVerdict("WARN")))
		}
	}
	weak := List(report["weak_domains"])
	weakJSON, _ := MarshalJSON(weak)
	lines = append(lines, fmt.Sprintf("weak-domains (≥5 records, high-value <20%%): %d  -> %s", len(weak), weakJSON))
	failures, warnings := List(report["failures"]), List(report["warnings"])
	lines = append(lines, "")
	if len(failures)+len(warnings) == 0 {
		lines = append(lines, Style("green", "✓ all signals PASS"))
	} else {
		if len(failures) > 0 {
			lines = append(lines, Style("red", "✗ FAIL:")+" "+strings.Join(failures, ", "))
		}
		if len(warnings) > 0 {
			lines = append(lines, Style("yellow", "! WARN:")+" "+strings.Join(warnings, ", "))
		}
	}
	return strings.Join(lines, "\n")
}
func auditSuggestHuman(payload map[string]any) string {
	groups := AnyList(payload["groups"])
	if len(groups) == 0 {
		return Style("green", "✓ no remediation suggestions — corpus looks clean")
	}
	lines := []string{}
	for _, value := range groups {
		g := Obj(value)
		lines = append(lines, "", Style("brand", Style("bold", "["+strings.ToUpper(Text(g["action"]))+"] "+Text(g["headline"]))), Style("muted", Text(g["rationale"])), "", "  record ids:")
		ids := List(g["record_ids"])
		preview := ids
		if len(preview) > 12 {
			preview = preview[:12]
		}
		tail := ""
		if len(ids) > len(preview) {
			tail = fmt.Sprintf(" … (+%d more)", len(ids)-len(preview))
		}
		lines = append(lines, "    "+strings.Join(preview, ", ")+tail, "", "  copy-paste commands:")
		for _, command := range List(g["commands"]) {
			lines = append(lines, "    "+Style("cyan", command))
		}
	}
	return strings.Join(lines, "\n")
}
func (a *App) runAudit(i Invocation) error {
	jsonMode := a.JSONMode || i.Bool("ci")
	dir, e := AtlasDir(a.Cwd)
	if e != nil {
		return e
	}
	if !exists(dir) {
		if jsonMode {
			a.JSONMode = true
		}
		return fmt.Errorf("No .luuna/atlas/ directory found. Run `luuna-atlas init` first.")
	}
	cfg, e := ReadConfig(a.Cwd)
	if e != nil {
		return e
	}
	if domain := i.String("domain"); domain != "" {
		if _, ok := Obj(cfg["domains"])[domain]; !ok {
			if jsonMode {
				a.JSONMode = true
			}
			return fmt.Errorf("Domain %q not found in config.", domain)
		}
	}
	report, items, e := a.computeAudit(cfg, i)
	if e != nil {
		return e
	}
	var suggestions map[string]any
	if i.Bool("suggest") {
		suggestions = auditSuggestions(items, cfg)
	}
	if jsonMode {
		success := !i.Bool("ci") || len(List(report["failures"])) == 0
		result := map[string]any{"success": success, "command": "audit", "report": report}
		if suggestions != nil {
			result["suggestions"] = suggestions
		}
		a.Emit(result)
		if !success {
			a.ExitCode = 1
		}
		return nil
	}
	fmt.Fprintln(a.Out, auditHuman(report))
	if suggestions != nil {
		fmt.Fprintln(a.Out, "\n"+Style("brand", Style("bold", "=== suggested remediations ===")))
		fmt.Fprintln(a.Out, auditSuggestHuman(suggestions))
	}
	return nil
}
