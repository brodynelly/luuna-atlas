package atlas

import (
	"fmt"
	"strings"
)

func (a *App) runOutcome(i Invocation) error {
	domain, id := i.Positionals[0], i.Positionals[1]
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	if !a.writeDomainCheck(cfg, domain, "outcome") {
		return nil
	}
	path, err := ExpertisePath(a.Cwd, domain)
	if err != nil {
		return err
	}
	if i.String("status") == "" {
		records, e := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
		if e != nil {
			return e
		}
		n, e := ResolveRecordID(records, id)
		if e != nil {
			return e
		}
		r := records[n]
		outcomes := writeRecordsValue(r["outcomes"])
		if a.JSONMode {
			data := map[string]any{"domain": domain, "outcomes": outcomes}
			if _, ok := r["id"]; ok {
				data["id"] = r["id"]
			}
			a.OK("outcome", data)
		} else if !a.Quiet {
			if len(outcomes) == 0 {
				a.Human(Style("dim", "No outcomes recorded for this record."))
			} else {
				a.Human(fmt.Sprintf("%s for %s (%d):", Style("brand", "Outcomes"), Style("accent", writeIDText(r, id)), len(outcomes)))
				for j, o := range outcomes {
					agent := ""
					if v := Text(o["agent"]); v != "" {
						agent = Style("dim", " ("+v+")")
					}
					a.Human(fmt.Sprintf("  %s %s%s", Style("dim", fmt.Sprintf("%d.", j+1)), writeStatusColor(Text(o["status"])), agent))
					for _, f := range []struct{ key, label string }{{"duration", "duration"}, {"test_results", "tests"}, {"notes", "notes"}, {"recorded_at", "recorded"}} {
						if v, ok := o[f.key]; ok && v != nil && v != "" {
							suffix := ""
							if f.key == "duration" {
								suffix = "ms"
							}
							a.Human(Style("dim", fmt.Sprintf("     %s: %v%s", f.label, v, suffix)))
						}
					}
				}
			}
		}
		return nil
	}
	return WithFileLock(path, func() error {
		records, e := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
		if e != nil {
			return e
		}
		n, e := ResolveRecordID(records, id)
		if e != nil {
			return e
		}
		r := CloneRecord(records[n])
		o := Record{"status": i.String("status"), "recorded_at": NowISO()}
		if i.Has("duration") {
			v, e := writeParseNumber(i.String("duration"), "duration")
			if e != nil {
				return e
			}
			o["duration"] = v
		}
		for _, f := range []string{"agent", "notes", "test-results"} {
			if v := i.String(f); v != "" {
				o[strings.ReplaceAll(f, "-", "_")] = v
			}
		}
		rememberOrder(map[string]any(o), []string{"status", "recorded_at", "duration", "agent", "notes", "test_results"})
		outcomes := append(writeRecordsValue(r["outcomes"]), o)
		fieldOrder := ObjectKeys(r)
		if _, exists := r["outcomes"]; !exists {
			fieldOrder = append(fieldOrder, "outcomes")
		}
		r["outcomes"] = outcomes
		rememberOrder(map[string]any(r), fieldOrder)
		records[n] = r
		if e = WriteRecords(path, records); e != nil {
			return e
		}
		if a.JSONMode {
			data := map[string]any{"action": "appended", "domain": domain, "outcome": o, "total_outcomes": len(outcomes)}
			if v, ok := r["id"]; ok {
				data["id"] = v
			}
			rememberOrder(data, []string{"action", "domain", "id", "outcome", "total_outcomes"})
			a.OK("outcome", data)
		} else {
			agent := ""
			if v := Text(o["agent"]); v != "" {
				agent = Style("dim", " ("+v+")")
			}
			a.Human(fmt.Sprintf("%s %s %s%s on %s", Style("brand", "✓"), Style("brand", "Outcome recorded:"), writeStatusColor(Text(o["status"])), agent, Style("accent", writeIDText(r, id))))
		}
		return nil
	})
}
