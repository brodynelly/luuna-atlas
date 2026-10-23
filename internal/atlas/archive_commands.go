package atlas

import (
	"fmt"
	"strings"
	"time"
)

func (a *App) runArchive(i Invocation) error {
	domain := i.Positionals[0]
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	if !a.writeDomainCheck(cfg, domain, "archive") {
		return nil
	}
	reason := strings.TrimSpace(i.String("reason"))
	if reason == "" {
		return fmt.Errorf("--reason must not be empty.")
	}
	hasID := len(i.Positionals) > 1
	if !hasID && !i.Has("records") {
		if a.JSONMode {
			return fmt.Errorf("Must provide a record ID or --records.")
		}
		return fmt.Errorf("must provide a record ID or --records.")
	}
	if hasID && i.Has("records") {
		if a.JSONMode {
			return fmt.Errorf("Cannot combine a record ID with --records. Use only one mode.")
		}
		return fmt.Errorf("cannot combine a record ID with --records. Use only one mode.")
	}
	ids := SplitCSV(i.String("records"))
	if hasID {
		ids = []string{i.Positionals[1]}
	}
	if len(ids) == 0 {
		return fmt.Errorf("--records requires at least one ID.")
	}
	reason = "manual: " + reason
	path, err := ExpertisePath(a.Cwd, domain)
	if err != nil {
		return err
	}
	archived, kept := []Record{}, []Record{}
	err = WithFileLock(path, func() error {
		records, e := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
		if e != nil {
			return e
		}
		indices := map[int]bool{}
		for _, id := range ids {
			n, e := ResolveRecordID(records, id)
			if e != nil {
				return e
			}
			indices[n] = true
		}
		for n, r := range records {
			if indices[n] {
				archived = append(archived, r)
			} else {
				kept = append(kept, r)
			}
		}
		if !i.Bool("dry-run") {
			return WriteRecords(path, kept)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !i.Bool("dry-run") && len(archived) > 0 {
		if err = ArchiveRecords(a.Cwd, domain, archived, time.Now(), reason, a.Registry); err != nil {
			return err
		}
	}
	if a.JSONMode {
		infos := []map[string]any{}
		for _, r := range archived {
			data := a.writeDeletedInfo(r)
			data["reason"] = reason
			infos = append(infos, data)
		}
		a.OK("archive", map[string]any{"domain": domain, "dryRun": i.Bool("dry-run"), "archived": infos, "kept": len(kept)})
	} else {
		prefix, verb := "", "Archived"
		if i.Bool("dry-run") {
			prefix = Style("yellow", "[DRY RUN]") + " "
			verb = "Would archive"
		}
		for _, r := range archived {
			rid := ""
			if v := Text(r["id"]); v != "" {
				rid = " " + Style("accent", v)
			}
			a.Human(fmt.Sprintf("%s%s%s %s (%s): %s", prefix, Style("brand", verb+" "+Text(r["type"])), rid, Style("brand", "from "+domain), reason, RecordSummary(r, a.Registry)))
		}
		if !i.Bool("dry-run") && len(archived) > 1 {
			a.Human(Style("brand", fmt.Sprintf("✓ Archived %d records from %s", len(archived), domain)))
		}
	}
	return nil
}

func (a *App) runRestore(i Invocation) error {
	id := i.Positionals[0]
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	type match struct {
		domain string
		record Record
	}
	matches := []match{}
	for _, domain := range DomainNames(cfg) {
		path, e := ArchivePath(a.Cwd, domain)
		if e != nil {
			return e
		}
		records, e := ReadArchive(path, a.Registry)
		if e != nil {
			return e
		}
		n, e := ResolveRecordID(records, id)
		if e == nil {
			matches = append(matches, match{domain, records[n]})
		}
	}
	if len(matches) == 0 {
		return fmt.Errorf("Archived record %q not found in any domain. Run `luuna-atlas search --archived <query>` to browse archives.", id)
	}
	if len(matches) > 1 {
		where := []string{}
		for _, m := range matches {
			where = append(where, fmt.Sprintf("%s (%s)", m.domain, Text(m.record["id"])))
		}
		return fmt.Errorf("Identifier %q matches archived records in multiple domains: %s. Use a longer prefix.", id, strings.Join(where, ", "))
	}
	target := matches[0]
	rid := Text(target.record["id"])
	if rid == "" {
		return fmt.Errorf("Archived record has no id; cannot restore safely.")
	}
	path, err := ExpertisePath(a.Cwd, target.domain)
	if err != nil {
		return err
	}
	existing, err := ReadRecords(path, a.Registry, a.AllowUnknownTypes)
	if err != nil {
		return err
	}
	for _, r := range existing {
		if Text(r["id"]) == rid {
			return fmt.Errorf("Live record %q already exists in domain %q (classification: %s). Delete or rename the live record before restoring, or run `luuna-atlas search --archived %s` to inspect the archived copy.", rid, target.domain, Text(r["classification"]), rid)
		}
	}
	removed, err := RemoveFromArchive(a.Cwd, target.domain, rid, a.Registry)
	if err != nil {
		return err
	}
	if removed == nil {
		return fmt.Errorf("Archived record %q disappeared between read and write (concurrent restore?).", rid)
	}
	reason := writeFallback(Text(removed["archive_reason"]), "manual")
	live := StripArchiveFields(removed)
	conflict, err := RestoreToExpertise(path, live, a.Registry)
	if err != nil {
		return err
	}
	if conflict != nil {
		if err = ArchiveRecords(a.Cwd, target.domain, []Record{StripArchiveFields(removed)}, time.Now(), reason, a.Registry); err != nil {
			return err
		}
		return fmt.Errorf("Live record %q appeared in domain %q while restore was in flight (classification: %s). Re-archived; nothing changed.", rid, target.domain, Text(conflict["classification"]))
	}
	if a.JSONMode {
		a.OK("restore", map[string]any{"domain": target.domain, "id": writeID(live), "type": live["type"], "summary": RecordSummary(live, a.Registry), "archive_reason": reason})
	} else {
		a.Human(fmt.Sprintf("%s %s %s %s (archived: %s): %s", Style("brand", "✓"), Style("brand", "Restored "+Text(live["type"])), Style("accent", rid), Style("brand", "to "+target.domain), reason, RecordSummary(live, a.Registry)))
	}
	return nil
}
