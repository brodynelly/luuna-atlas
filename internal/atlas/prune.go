package atlas

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func writeNextTier(tier string) string {
	if tier == "foundational" {
		return "tactical"
	}
	if tier == "tactical" {
		return "observational"
	}
	return ""
}
func writeAnchorConfigErrors(cfg map[string]any) []string {
	out := []string{}
	for _, field := range []string{"threshold", "grace_days"} {
		v, ok := cfg[field]
		if !ok {
			continue
		}
		n := Number(v)
		validType := false
		switch v.(type) {
		case int, int64, int32, float64, float32:
			validType = true
		}
		if !validType || math.IsNaN(n) || math.IsInf(n, 0) {
			out = append(out, fmt.Sprintf("%s must be a finite number (got %s)", field, JSONPretty(v)))
			continue
		}
		if field == "threshold" && (n < 0 || n > 1) {
			out = append(out, fmt.Sprintf("threshold must be between 0 and 1 (got %v)", n))
		}
		if field == "grace_days" && n < 0 {
			out = append(out, fmt.Sprintf("grace_days must be >= 0 (got %v)", n))
		}
	}
	return out
}
func (a *App) runPrune(i Invocation) error {
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	now := time.Now()
	shelf := Obj(ConfigValue(cfg, "classification_defaults", "shelf_life"))
	anchorCfg := Obj(ConfigValue(cfg, "decay", "anchor_validity"))
	if errors := writeAnchorConfigErrors(anchorCfg); len(errors) > 0 {
		return fmt.Errorf("Invalid decay.anchor_validity config: %s. Edit .luuna/atlas/config.yaml.", strings.Join(errors, "; "))
	}
	threshold := NumberDefault(anchorCfg["threshold"], .5)
	grace := NumberDefault(anchorCfg["grace_days"], 7)
	live := []DomainRecords{}
	for _, d := range DomainNames(cfg) {
		path, e := ExpertisePath(a.Cwd, d)
		if e != nil {
			return e
		}
		records, e := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
		if e != nil {
			return e
		}
		live = append(live, DomainRecords{Domain: d, Records: records})
	}
	superseded, cycles := CollectSupersededIDs(live)
	if len(cycles) > 0 && !a.JSONMode && !a.Quiet {
		a.Warning(fmt.Sprintf("supersession cycle detected for %d record(s); cycle members will not be demoted. Run `luuna-atlas doctor` for details.", len(cycles)))
	}
	validity := map[string]AnchorValidityResult{}
	if i.Bool("check-anchors") {
		for _, d := range live {
			for _, r := range d.Records {
				if id := Text(r["id"]); id != "" {
					validity[id] = AnchorValidity(r, a.Cwd)
				}
			}
		}
	}
	anchorHit := func(r Record) *AnchorValidityResult {
		if !i.Bool("check-anchors") || !PassedAnchorGrace(r, now, grace) {
			return nil
		}
		var v AnchorValidityResult
		var ok bool
		if id := Text(r["id"]); id != "" {
			v, ok = validity[id]
			if !ok {
				return nil
			}
		} else {
			v = AnchorValidity(r, a.Cwd)
		}
		if v.ValidFraction == nil || *v.ValidFraction >= threshold {
			return nil
		}
		return &v
	}
	candidates := []map[string]any{}
	candidateDomains := map[string]bool{}
	for _, d := range live {
		stale, demote, anchorDecay := []Record{}, []Record{}, []Record{}
		for _, r := range d.Records {
			if IsRecordStale(r, now, shelf) {
				stale = append(stale, r)
				continue
			}
			if superseded[Text(r["id"])] && Text(r["id"]) != "" {
				demote = append(demote, r)
			}
			if anchorHit(r) != nil {
				anchorDecay = append(anchorDecay, r)
			}
		}
		if len(stale)+len(demote)+len(anchorDecay) > 0 {
			candidate := map[string]any{"domain": d.Domain, "stale": stale, "demote": demote, "anchor_decay": anchorDecay}
			rememberOrder(candidate, []string{"domain", "stale", "demote", "anchor_decay"})
			candidates = append(candidates, candidate)
			candidateDomains[d.Domain] = true
		}
	}
	if !i.Bool("dry-run") && len(candidates) > 0 {
		hook, e := RunHooks(a.Cwd, cfg, "pre-prune", map[string]any{"candidates": candidates}, a.Err)
		if e != nil {
			return e
		}
		if hook.Blocked {
			return fmt.Errorf("%s", writeFallback(hook.BlockReason, "pre-prune hook blocked"))
		}
		if !a.JSONMode {
			for _, w := range hook.Warnings {
				a.Warning(w)
			}
		}
	}
	results := []map[string]any{}
	actions := []map[string]any{}
	totalPruned, totalDemoted, totalAnchor, totalSupersession := 0, 0, 0, 0
	for _, d := range DomainNames(cfg) {
		if !candidateDomains[d] {
			continue
		}
		path, e := ExpertisePath(a.Cwd, d)
		if e != nil {
			return e
		}
		archived := []Record{}
		domainActions := []map[string]any{}
		var result map[string]any
		e = WithFileLock(path, func() error {
			records, e := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
			if e != nil {
				return e
			}
			if len(records) == 0 {
				return nil
			}
			kept := []Record{}
			pruned, demoted, anchorCount, supCount := 0, 0, 0, 0
			for _, r := range records {
				from := Text(r["classification"])
				if IsRecordStale(r, now, shelf) {
					pruned++
					archived = append(archived, r)
					action := map[string]any{"domain": d, "type": r["type"], "from": from, "to": "archived", "reasons": []string{"stale"}}
					if v, ok := r["id"]; ok {
						action["id"] = v
					}
					rememberOrder(action, []string{"domain", "id", "type", "from", "to", "reasons"})
					domainActions = append(domainActions, action)
					continue
				}
				sup := Text(r["id"]) != "" && superseded[Text(r["id"])]
				anchor := anchorHit(r)
				if !sup && anchor == nil {
					kept = append(kept, r)
					continue
				}
				reasons := []string{}
				if sup {
					reasons = append(reasons, "superseded")
				}
				if anchor != nil {
					reasons = append(reasons, "anchor_decay")
				}
				to := writeNextTier(from)
				if i.Bool("aggressive") {
					to = ""
				}
				action := map[string]any{"domain": d, "type": r["type"], "from": from, "reasons": reasons}
				if v, ok := r["id"]; ok {
					action["id"] = v
				}
				if anchor != nil {
					anchors := map[string]any{"valid_fraction": *anchor.ValidFraction, "valid": anchor.Valid, "total": anchor.Total, "broken": anchor.Broken}
					rememberOrder(anchors, []string{"valid_fraction", "valid", "total", "broken"})
					action["anchors"] = anchors
				}
				if to == "" {
					pruned++
					bottom := CloneRecord(r)
					bottom["archive_reason"] = strings.Join(reasons, "+")
					archived = append(archived, bottom)
					action["to"] = "archived"
				} else {
					nr := CloneRecord(r)
					fieldOrder := ObjectKeys(nr)
					nr["classification"] = to
					if sup {
						nr["supersession_demoted_at"] = ISO(now)
						supCount++
					}
					if anchor != nil {
						nr["anchor_decay_demoted_at"] = ISO(now)
						anchorCount++
					}
					rememberOrder(map[string]any(nr), Unique(append(fieldOrder, "classification", "supersession_demoted_at", "anchor_decay_demoted_at")))
					kept = append(kept, nr)
					demoted++
					action["to"] = to
				}
				rememberOrder(action, []string{"domain", "id", "type", "from", "to", "reasons", "anchors"})
				domainActions = append(domainActions, action)
			}
			if pruned > 0 || demoted > 0 {
				if !i.Bool("dry-run") {
					if e = WriteRecords(path, kept); e != nil {
						return e
					}
				}
				result = map[string]any{"domain": d, "before": len(records), "pruned": pruned, "demoted": demoted, "anchor_demoted": anchorCount, "supersession_demoted": supCount, "after": len(kept)}
				rememberOrder(result, []string{"domain", "before", "pruned", "demoted", "anchor_demoted", "supersession_demoted", "after"})
			}
			return nil
		})
		if e != nil {
			return e
		}
		if result != nil {
			if !i.Bool("dry-run") && !i.Bool("hard") && len(archived) > 0 {
				if e = ArchiveRecords(a.Cwd, d, archived, now, "stale", a.Registry); e != nil {
					return e
				}
			}
			results = append(results, result)
			totalPruned += int(Number(result["pruned"]))
			totalDemoted += int(Number(result["demoted"]))
			totalAnchor += int(Number(result["anchor_demoted"]))
			totalSupersession += int(Number(result["supersession_demoted"]))
			actions = append(actions, domainActions...)
		}
	}
	if a.JSONMode {
		data := map[string]any{"dryRun": i.Bool("dry-run"), "hard": i.Bool("hard"), "aggressive": i.Bool("aggressive"), "checkAnchors": i.Bool("check-anchors"), "totalPruned": totalPruned, "totalDemoted": totalDemoted, "totalAnchorDemoted": totalAnchor, "totalSupersessionDemoted": totalSupersession, "results": results}
		if i.Bool("explain") {
			explanations := []map[string]any{}
			for _, action := range actions {
				for _, r := range List(action["reasons"]) {
					if r != "stale" {
						explanations = append(explanations, action)
						break
					}
				}
			}
			data["explanations"] = explanations
		}
		a.OK("prune", data)
		return nil
	}
	if totalPruned == 0 && totalDemoted == 0 {
		a.Human(Style("brand", "No stale or superseded records found. All records are current."))
		return nil
	}
	label, demoteLabel, prefix := "Archived", "Demoted", ""
	if i.Bool("hard") {
		label = "Deleted"
	}
	if i.Bool("dry-run") {
		label = "Would archive"
		if i.Bool("hard") {
			label = "Would delete"
		}
		demoteLabel = "Would demote"
		prefix = Style("yellow", "[DRY RUN] ")
	}
	if !a.Quiet {
		for _, r := range results {
			p, dm := int(Number(r["pruned"])), int(Number(r["demoted"]))
			body := ""
			if p > 0 && dm > 0 {
				body = fmt.Sprintf("%s %s, demoted %s", label, Style("red", fmt.Sprint(p)), Style("yellow", fmt.Sprint(dm)))
			} else if p > 0 {
				body = fmt.Sprintf("%s %s", label, Style("red", fmt.Sprint(p)))
			} else {
				body = fmt.Sprintf("%s %s", demoteLabel, Style("yellow", fmt.Sprint(dm)))
			}
			fmt.Fprintf(a.Out, "%s%s: %s of %v records (%v remaining)\n", prefix, Style("cyan", Text(r["domain"])), body, r["before"], r["after"])
			for _, action := range actions {
				if Text(action["domain"]) != Text(r["domain"]) {
					continue
				}
				fmt.Fprintf(a.Out, "  %s [%s]: %s → %s (%s)\n", Style("dim", writeFallback(Text(action["id"]), "(no id)")), Text(action["type"]), Text(action["from"]), Text(action["to"]), strings.Join(List(action["reasons"]), " + "))
				if anchors, ok := action["anchors"]; ok && i.Bool("explain") {
					av := Obj(anchors)
					fmt.Fprintf(a.Out, "      anchors: %v/%v valid (%.0f%%)\n", av["valid"], av["total"], Number(av["valid_fraction"])*100)
					switch broken := av["broken"].(type) {
					case []Anchor:
						for _, b := range broken {
							fmt.Fprintf(a.Out, "        %s %s: %s\n", Style("red", "✗"), b.Kind, b.Path)
						}
					}
				}
			}
		}
	}
	totals := []string{}
	if totalPruned > 0 {
		noun := "records"
		if totalPruned == 1 {
			noun = "record"
		}
		totals = append(totals, fmt.Sprintf("%s %d stale %s", strings.ToLower(label), totalPruned, noun))
	}
	if totalDemoted > 0 {
		noun := "records"
		if totalDemoted == 1 {
			noun = "record"
		}
		breakdown := []string{}
		if totalSupersession > 0 {
			breakdown = append(breakdown, fmt.Sprintf("%d superseded", totalSupersession))
		}
		if totalAnchor > 0 {
			breakdown = append(breakdown, fmt.Sprintf("%d anchor-decayed", totalAnchor))
		}
		suffix, tag := "", ""
		if len(breakdown) > 1 {
			suffix = " (" + strings.Join(breakdown, ", ") + ")"
		} else if totalSupersession > 0 {
			tag = "superseded "
		} else if totalAnchor > 0 {
			tag = "anchor-decayed "
		}
		totals = append(totals, fmt.Sprintf("%s %d %s%s%s", strings.ToLower(demoteLabel), totalDemoted, tag, noun, suffix))
	}
	separator := "\n"
	if a.Quiet {
		separator = ""
	}
	fmt.Fprintf(a.Out, "%s%s%s\n", separator, prefix, Style("bold", "Total: "+strings.Join(totals, "; ")+"."))
	if !a.Quiet && !i.Bool("hard") && !i.Bool("dry-run") && totalPruned > 0 {
		fmt.Fprintln(a.Out, Style("dim", "Records moved to .luuna/atlas/archive/. Restore with `luuna-atlas restore <id>` or use `--hard` next time to permanently delete."))
	}
	return nil
}
