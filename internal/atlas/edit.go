package atlas

import (
	"fmt"
	"strings"
)

func (a *App) runEdit(i Invocation) error {
	domain, id := i.Positionals[0], i.Positionals[1]
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	if !a.writeDomainCheck(cfg, domain, "edit") {
		return nil
	}
	path, err := ExpertisePath(a.Cwd, domain)
	if err != nil {
		return err
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
		fieldOrder := ObjectKeys(r)
		set := func(field string, value any) {
			if _, exists := r[field]; !exists {
				fieldOrder = append(fieldOrder, field)
			}
			r[field] = value
		}
		if i.Has("classification") {
			set("classification", i.String("classification"))
		}
		for _, key := range []string{"relates-to", "supersedes"} {
			if i.Has(key) {
				set(strings.ReplaceAll(key, "-", "_"), SplitCSV(i.String(key)))
			}
		}
		outcome, e := writeOutcomeFlags(i)
		if e != nil {
			return e
		}
		if outcome != nil {
			set("outcomes", append(writeRecordsValue(r["outcomes"]), outcome))
		}
		if def := a.Registry.Types[Text(r["type"])]; def != nil {
			for _, f := range append(append([]string{}, def.Required...), def.Optional...) {
				flag := strings.ReplaceAll(f, "_", "-")
				if i.Has(flag) {
					if def.ExtractsFiles && f == def.FilesField {
						set(f, SplitCSV(i.String(flag)))
					} else {
						set(f, i.String(flag))
					}
				}
			}
		}
		rememberOrder(map[string]any(r), fieldOrder)
		if issues := ValidateRecord(r, a.Registry); len(issues) > 0 {
			if a.JSONMode {
				return fmt.Errorf("Updated record failed schema validation: %s", writeValidation(issues))
			}
			a.ExitCode = 1
			fmt.Fprintln(a.Err, Style("red", "Error: updated record failed schema validation:"))
			for _, issue := range issues {
				fmt.Fprintln(a.Err, Style("red", fmt.Sprintf("  %s %s", issue.Path, issue.Message)))
			}
			return nil
		}
		records[n] = r
		if e = WriteRecords(path, records); e != nil {
			return e
		}
		if a.JSONMode {
			result := map[string]any{"domain": domain, "id": writeID(r), "type": r["type"], "record": r}
			rememberOrder(result, []string{"domain", "id", "type", "record"})
			a.OK("edit", result)
		} else {
			rid := ""
			if v := Text(r["id"]); v != "" {
				rid = " " + Style("accent", v)
			}
			a.Human(Style("brand", "✓") + " " + Style("brand", "Updated "+Text(r["type"])) + rid + " " + Style("brand", "in "+domain))
		}
		return nil
	})
}
