package atlas

import (
	"bufio"
	"fmt"
	"strings"
)

func (a *App) writeConfirm(prompt string) bool {
	fmt.Fprint(a.Out, prompt+" (y/N): ")
	line, _ := bufio.NewReader(a.In).ReadString('\n')
	answer := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"))
	return answer == "y" || answer == "yes"
}
func (a *App) writePrintDeleted(r Record, domain string, dry bool) {
	prefix, verb := "", "Deleted"
	if dry {
		prefix = Style("yellow", "[DRY RUN]") + " "
		verb = "Would delete"
	}
	rid := ""
	if id := Text(r["id"]); id != "" {
		rid = " " + Style("accent", id)
	}
	a.Human(fmt.Sprintf("%s%s%s %s: %s", prefix, Style("brand", verb+" "+Text(r["type"])), rid, Style("brand", "from "+domain), RecordSummary(r, a.Registry)))
}
func (a *App) writeDeletedInfo(r Record) map[string]any {
	return map[string]any{"id": writeID(r), "type": r["type"], "summary": RecordSummary(r, a.Registry)}
}

func (a *App) runDelete(i Invocation) error {
	domain := i.Positionals[0]
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	if !a.writeDomainCheck(cfg, domain, "delete") {
		return nil
	}
	hasID := len(i.Positionals) > 1
	modes := 0
	for _, b := range []bool{hasID, i.Has("records"), i.Has("all-except")} {
		if b {
			modes++
		}
	}
	if modes == 0 {
		if a.JSONMode {
			return fmt.Errorf("Must provide a record ID, --records, or --all-except.")
		}
		return fmt.Errorf("must provide a record ID, --records, or --all-except.")
	}
	if modes > 1 {
		if a.JSONMode {
			return fmt.Errorf("Cannot combine a record ID with --records or --all-except. Use only one mode.")
		}
		return fmt.Errorf("cannot combine a record ID with --records or --all-except. Use only one mode.")
	}
	ids := []string{}
	keepMode := i.Has("all-except")
	if hasID {
		ids = []string{i.Positionals[1]}
	} else if keepMode {
		ids = SplitCSV(i.String("all-except"))
		if len(ids) == 0 {
			return fmt.Errorf("--all-except requires at least one ID to keep.")
		}
	} else {
		ids = SplitCSV(i.String("records"))
		if len(ids) == 0 {
			return fmt.Errorf("--records requires at least one ID.")
		}
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
		selected := map[int]bool{}
		for _, id := range ids {
			n, e := ResolveRecordID(records, id)
			if e != nil {
				return e
			}
			selected[n] = true
		}
		deleted, kept := []Record{}, []Record{}
		for n, r := range records {
			remove := selected[n]
			if keepMode {
				remove = !remove
			}
			if remove {
				deleted = append(deleted, r)
			} else {
				kept = append(kept, r)
			}
		}
		if !i.Bool("dry-run") {
			if e = WriteRecords(path, kept); e != nil {
				return e
			}
		}
		if a.JSONMode {
			if hasID && !i.Bool("dry-run") {
				data := a.writeDeletedInfo(deleted[0])
				data["domain"] = domain
				a.OK("delete", data)
			} else {
				infos := []map[string]any{}
				for _, r := range deleted {
					infos = append(infos, a.writeDeletedInfo(r))
				}
				a.OK("delete", map[string]any{"domain": domain, "dryRun": i.Bool("dry-run"), "deleted": infos, "kept": len(kept)})
			}
		} else {
			if hasID && !i.Bool("dry-run") {
				r := deleted[0]
				rid := ""
				if id := Text(r["id"]); id != "" {
					rid = " " + Style("accent", id)
				}
				a.Human(fmt.Sprintf("%s %s%s %s: %s", Style("brand", "✓"), Style("brand", "Deleted "+Text(r["type"])), rid, Style("brand", "from "+domain), RecordSummary(r, a.Registry)))
			} else {
				for _, r := range deleted {
					a.writePrintDeleted(r, domain, i.Bool("dry-run"))
				}
				if !i.Bool("dry-run") && len(deleted) > 1 {
					a.Human(Style("brand", fmt.Sprintf("✓ Deleted %d records from %s", len(deleted), domain)))
				}
			}
		}
		return nil
	})
}

func (a *App) runDeleteDomain(i Invocation) error {
	domain := i.Positionals[0]
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	if !writeHasDomain(cfg, domain) {
		if a.JSONMode {
			a.writeDomainCheck(cfg, domain, "delete-domain")
		} else {
			a.ExitCode = 1
			fmt.Fprintln(a.Err, Style("red", fmt.Sprintf("Error: domain %q not found in config.", domain)))
			fmt.Fprintln(a.Err, Style("red", fmt.Sprintf("Hint: Run `luuna-atlas add %s` to create it, or check `luuna-atlas status` for existing domains.", domain)))
		}
		return nil
	}
	path, err := ExpertisePath(a.Cwd, domain)
	if err != nil {
		return err
	}
	records, err := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
	if err != nil {
		return err
	}
	if i.Bool("dry-run") {
		if a.JSONMode {
			a.OK("delete-domain", map[string]any{"domain": domain, "dryRun": true, "recordCount": len(records)})
		} else {
			plural := "s"
			if len(records) == 1 {
				plural = ""
			}
			a.Human(fmt.Sprintf("%s Would delete domain %s (%d record%s) and its expertise file.", Style("yellow", "[DRY RUN]"), Style("accent", domain), len(records), plural))
		}
		return nil
	}
	if !a.JSONMode && !i.Bool("yes") {
		plural := "s"
		if len(records) == 1 {
			plural = ""
		}
		if !a.writeConfirm(fmt.Sprintf("This will delete domain %q (%d record%s) and its expertise file. Continue?", domain, len(records), plural)) {
			fmt.Fprintln(a.Out, Style("yellow", "Cancelled."))
			return nil
		}
	}
	if err = WithFileLock(path, func() error { return RemoveDomain(a.Cwd, domain) }); err != nil {
		return err
	}
	if a.JSONMode {
		a.OK("delete-domain", map[string]any{"domain": domain, "deletedFile": true, "recordCount": len(records)})
	} else {
		a.Human(Style("brand", "✓") + " " + Style("brand", "Removed domain") + " " + Style("accent", domain) + " and deleted expertise file.")
	}
	return nil
}
