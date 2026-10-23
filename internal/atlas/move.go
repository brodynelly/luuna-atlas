package atlas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) runMove(i Invocation) error {
	source, id, target := i.Positionals[0], i.Positionals[1], i.Positionals[2]
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	if source == target {
		return fmt.Errorf("Source and target domain are the same — nothing to move.")
	}
	for _, d := range []string{source, target} {
		if !writeHasDomain(cfg, d) {
			available := strings.Join(DomainNames(cfg), ", ")
			if available == "" {
				available = "(none)"
			}
			return fmt.Errorf("Domain %q not found in config. Available domains: %s", d, available)
		}
	}
	sourcePath, err := ExpertisePath(a.Cwd, source)
	if err != nil {
		return err
	}
	targetPath, err := ExpertisePath(a.Cwd, target)
	if err != nil {
		return err
	}
	var rs []Record
	err = WithFileLock(sourcePath, func() error { var e error; rs, e = ReadRecords(sourcePath, a.Registry, a.AllowUnknownTypes); return e })
	if err != nil {
		return err
	}
	n, err := ResolveRecordID(rs, id)
	if err != nil {
		return err
	}
	r := rs[n]
	rid := writeIDText(r, id)
	if Text(r["status"]) == "archived" {
		return fmt.Errorf("Record %s is archived. Run `luuna-atlas restore %s` first.", rid, rid)
	}
	domainCfg := Obj(Obj(cfg["domains"])[target])
	allowed := List(domainCfg["allowed_types"])
	if len(allowed) > 0 && !writeContains(allowed, Text(r["type"])) && !i.Bool("force") {
		return fmt.Errorf("Type %q is not in target domain %q allowed_types (%s). Pass --force to override, or adjust config.yaml.", Text(r["type"]), target, strings.Join(allowed, ", "))
	}
	missing := writeMissingFields(r, List(domainCfg["required_fields"]))
	if len(missing) > 0 {
		return fmt.Errorf("Record is missing field(s) required by target domain %q: %s. Edit the record (`luuna-atlas edit %s`) before moving.", target, writeQuoted(missing), rid)
	}
	if issues := ValidateRecord(r, a.Registry); len(issues) > 0 {
		return fmt.Errorf("Record fails schema validation: %s. Edit the record before moving.", writeValidation(issues))
	}
	incoming := []map[string]any{}
	entries, _ := os.ReadDir(filepath.Dir(sourcePath))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		p := filepath.Join(filepath.Dir(sourcePath), entry.Name())
		if p == sourcePath || p == targetPath {
			continue
		}
		others, e := ReadRecords(p, a.Registry, true)
		if e != nil {
			continue
		}
		for _, other := range others {
			for _, field := range []string{"relates_to", "supersedes"} {
				if writeContains(List(other[field]), Text(r["id"])) {
					incoming = append(incoming, map[string]any{"domain": strings.TrimSuffix(entry.Name(), ".jsonl"), "id": writeID(other), "field": field})
				}
			}
		}
	}
	summary := func(record Record) map[string]any {
		return map[string]any{"id": writeID(record), "type": record["type"], "summary": RecordSummary(record, a.Registry)}
	}
	if i.Bool("dry-run") {
		if a.JSONMode {
			a.OK("move", map[string]any{"dryRun": true, "sourceDomain": source, "targetDomain": target, "record": summary(r), "incomingReferences": incoming})
		} else {
			a.Human(fmt.Sprintf("%s %s %s %s: %s", Style("yellow", "[DRY RUN]"), Style("brand", "Would move "+Text(r["type"])), Style("accent", rid), Style("brand", fmt.Sprintf("from %s → %s", source, target)), RecordSummary(r, a.Registry)))
			if len(incoming) > 0 {
				a.Human(Style("dim", fmt.Sprintf("  %d inbound reference(s) detected; ID is preserved so links remain valid.", len(incoming))))
			}
		}
		return nil
	}
	pre, err := RunHooks(a.Cwd, cfg, "pre-record", map[string]any{"domain": target, "record": r}, a.Err)
	if err != nil {
		return err
	}
	if pre.Blocked {
		return fmt.Errorf("%s", writeFallback(pre.BlockReason, "pre-record hook blocked the move"))
	}
	if pre.RanAny {
		if mutated := writeObject(Obj(pre.Payload)["record"]); mutated != nil {
			r = Record(mutated)
			if issues := ValidateRecord(r, a.Registry); len(issues) > 0 {
				return fmt.Errorf("pre-record hook produced an invalid record: %s", writeValidation(issues))
			}
		}
	}
	err = WithFileLock(targetPath, func() error {
		if e := AppendRecord(targetPath, r, a.Registry); e != nil {
			return e
		}
		return WithFileLock(sourcePath, func() error {
			current, e := ReadRecords(sourcePath, a.Registry, a.AllowUnknownTypes)
			if e != nil {
				return e
			}
			kept := []Record{}
			for _, v := range current {
				if Text(v["id"]) != Text(r["id"]) {
					kept = append(kept, v)
				}
			}
			return WriteRecords(sourcePath, kept)
		})
	})
	if err != nil {
		return err
	}
	post, err := RunHooks(a.Cwd, cfg, "post-record", map[string]any{"domain": target, "record": r, "action": "created"}, a.Err)
	if err != nil {
		return err
	}
	warnings := append(append([]string{}, pre.Warnings...), post.Warnings...)
	if a.JSONMode {
		data := map[string]any{"sourceDomain": source, "targetDomain": target, "record": summary(r), "incomingReferences": incoming}
		if len(warnings) > 0 {
			data["warnings"] = warnings
		}
		a.OK("move", data)
	} else {
		a.Human(fmt.Sprintf("%s %s %s %s: %s", Style("brand", "✓"), Style("brand", "Moved "+Text(r["type"])), Style("accent", writeIDText(r, "")), Style("brand", fmt.Sprintf("from %s → %s", source, target)), RecordSummary(r, a.Registry)))
		if len(incoming) > 0 {
			a.Human(Style("yellow", fmt.Sprintf("  %d inbound reference(s) found; ID preserved so existing links still resolve:", len(incoming))))
			for _, ref := range incoming {
				a.Human(Style("dim", fmt.Sprintf("    %s/%s via %s", Text(ref["domain"]), writeFallback(Text(ref["id"]), "(no id)"), Text(ref["field"]))))
			}
		}
		for _, w := range warnings {
			a.Human(Style("yellow", "  warning: "+w))
		}
	}
	return nil
}
