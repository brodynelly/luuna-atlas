package atlas

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

func (a *App) runSync(i Invocation) error {
	if !IsGitRepo(a.Cwd) {
		if !a.JSONMode {
			a.Error("sync", "not in a git repository.")
			return nil
		}
		return fmt.Errorf("Not in a git repository. Run this command from within a git repository.")
	}
	validate := !i.Has("validate") || i.Bool("validate")
	if validate {
		cfg, e := ReadConfig(a.Cwd)
		if e != nil {
			return a.syncFailure(fmt.Sprintf("Validation error: %s", e))
		}
		a.Registry, e = NewRegistry(cfg)
		if e != nil {
			return a.syncFailure(fmt.Sprintf("Validation error: %s", e))
		}
		report, e := a.validateCorpus(cfg, true)
		if e != nil {
			return a.syncFailure(fmt.Sprintf("Validation error: %s", e))
		}
		if len(report.Errors) > 0 {
			if a.JSONMode {
				a.Emit(map[string]any{"success": false, "command": "sync", "validated": false, "committed": false, "errors": report.Errors})
			} else {
				fmt.Fprintln(a.Err, Style("red", "Validation failed:"))
				for _, row := range report.Errors {
					fmt.Fprintln(a.Err, Style("red", fmt.Sprintf("  %s:%d - %s", row.Domain, row.Line, row.Message)))
				}
				fmt.Fprintln(a.Err, Style("red", "\nFix errors and retry, or use --no-validate to skip."))
			}
			a.ExitCode = 1
			return nil
		}
		if !a.JSONMode {
			a.Human(Style("brand", "✓") + " " + Style("brand", fmt.Sprintf("Validated %d records", report.TotalRecords)))
		}
	}
	if IsInsideWorktree(a.Cwd) {
		message := "Skipped commit: running inside a git worktree. Changes are written to the main repo's .luuna/atlas/."
		if a.JSONMode {
			a.OK("sync", map[string]any{"validated": validate, "committed": false, "message": message})
		} else {
			a.Human(Style("brand", "ℹ") + " " + message)
		}
		return nil
	}
	status, _ := Git(a.Cwd, "status", "--porcelain", ".luuna/atlas/")
	if strings.TrimSpace(status) == "" {
		if a.JSONMode {
			a.OK("sync", map[string]any{"validated": validate, "committed": false, "message": "No changes to commit"})
		} else {
			a.Human("No .luuna/atlas/ changes to commit.")
		}
		return nil
	}
	message := i.String("message")
	if !i.Has("message") {
		message = "luuna-atlas: update expertise"
	}
	if _, e := Git(a.Cwd, "add", ".luuna/atlas/"); e != nil {
		return a.syncFailure(fmt.Sprintf("Git error: %s", gitCommandFailure([]string{"add", ".luuna/atlas/"}, e)))
	}
	if _, e := Git(a.Cwd, "commit", "-m", message); e != nil {
		return a.syncFailure(fmt.Sprintf("Git error: %s", gitCommandFailure([]string{"commit", "-m", message}, e)))
	}
	if a.JSONMode {
		a.OK("sync", map[string]any{"validated": validate, "committed": true, "message": message})
	} else {
		a.Human(Style("brand", "✓") + " " + Style("brand", "Committed .luuna/atlas/ changes: \""+message+"\""))
	}
	return nil
}

func gitCommandFailure(args []string, err error) error {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Errorf("Command failed: git %s\n%s", strings.Join(args, " "), exit.Stderr)
	}
	return err
}

func (a *App) syncFailure(message string) error {
	a.ExitCode = 1
	if a.JSONMode {
		a.Error("sync", message)
	} else {
		fmt.Fprintln(a.Err, Style("red", message))
	}
	return nil
}

func (a *App) runLearn(i Invocation) error {
	if !IsGitRepo(a.Cwd) {
		if !a.JSONMode {
			a.Error("learn", "not in a git repository.")
			return nil
		}
		return fmt.Errorf("Not in a git repository. Run this command from within a git repository.")
	}
	dir, e := AtlasDir(a.Cwd)
	if e != nil {
		return e
	}
	if !exists(dir) {
		if a.JSONMode {
			a.OK("learn", map[string]any{"changedFiles": []any{}, "suggestedDomains": []any{}, "unmatchedFiles": []any{}, "message": "No .luuna/atlas/ directory found — skipping"})
		}
		return nil
	}
	files := GetChangedFiles(a.Cwd, i.String("since"))
	if len(files) == 0 {
		if a.JSONMode {
			a.OK("learn", map[string]any{"changedFiles": []any{}, "suggestedDomains": []any{}, "unmatchedFiles": []any{}, "message": "No changed files found"})
		} else {
			fmt.Fprintln(a.Out, "No changed files found. Nothing to learn from.")
		}
		return nil
	}
	cfg, e := ReadConfig(a.Cwd)
	if e != nil {
		return e
	}
	matches := []map[string]any{}
	matched := map[string]bool{}
	for _, d := range DomainNames(cfg) {
		records, e := a.readDomain(d)
		if e != nil {
			return e
		}
		anchors := []string{}
		for _, r := range records {
			def := a.Registry.Types[Text(r["type"])]
			if def != nil && def.ExtractsFiles {
				anchors = append(anchors, List(r[def.FilesField])...)
			}
		}
		hits := []string{}
		for _, f := range files {
			for _, anchor := range anchors {
				if f == anchor || strings.HasSuffix(f, anchor) || strings.HasSuffix(anchor, f) {
					hits = append(hits, f)
					matched[f] = true
					break
				}
			}
		}
		if len(hits) > 0 {
			matches = append(matches, map[string]any{"domain": d, "matchCount": len(hits), "files": hits})
		}
	}
	sort.SliceStable(matches, func(x, y int) bool { return Number(matches[x]["matchCount"]) > Number(matches[y]["matchCount"]) })
	unmatched := []string{}
	for _, f := range files {
		if !matched[f] {
			unmatched = append(unmatched, f)
		}
	}
	if a.JSONMode {
		a.OK("learn", map[string]any{"changedFiles": files, "suggestedDomains": matches, "unmatchedFiles": unmatched})
		return nil
	}
	fmt.Fprintln(a.Out, Style("bold", "\nSession learnings check\n"))
	fmt.Fprintln(a.Out, Style("cyan", fmt.Sprintf("Changed files (%d):", len(files))))
	for _, f := range files {
		fmt.Fprintln(a.Out, "  "+f)
	}
	if len(matches) > 0 {
		fmt.Fprintln(a.Out, Style("cyan", "\nSuggested domains:"))
		for _, m := range matches {
			label := "files match"
			if Number(m["matchCount"]) == 1 {
				label = "file matches"
			}
			fmt.Fprintf(a.Out, "  %s (%d %s existing records)\n", Style("bold", Text(m["domain"])), int(Number(m["matchCount"])), label)
		}
	}
	if len(unmatched) > 0 {
		fmt.Fprintln(a.Out, Style("yellow", "\nUnmatched files (no domain association):"))
		for _, f := range unmatched {
			fmt.Fprintln(a.Out, "  "+f)
		}
	}
	fmt.Fprintln(a.Out, Style("dim", "\nRecord learnings with:"))
	fmt.Fprintln(a.Out, Style("dim", `  luuna-atlas record <domain> --type <type> --description "..."`))
	fmt.Fprintln(a.Out)
	return nil
}

type DiffEntry struct {
	Domain  string   `json:"domain"`
	Added   []Record `json:"added"`
	Removed []Record `json:"removed"`
}

func ParseExpertiseDiff(text string) []DiffEntry {
	entries := map[string]*DiffEntry{}
	domain := ""
	pattern := regexp.MustCompile(`\.luuna/atlas/expertise/([^/]+)\.jsonl`)
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			if m := pattern.FindStringSubmatch(line); m != nil {
				domain = m[1]
				if entries[domain] == nil {
					entries[domain] = &DiffEntry{domain, []Record{}, []Record{}}
				}
			}
			continue
		}
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "@@") || domain == "" || len(line) < 2 {
			continue
		}
		if line[0] != '+' && line[0] != '-' {
			continue
		}
		v, e := ParseJSON([]byte(line[1:]))
		if e != nil {
			continue
		}
		if line[0] == '+' {
			entries[domain].Added = append(entries[domain].Added, Record(Obj(v)))
		} else {
			entries[domain].Removed = append(entries[domain].Removed, Record(Obj(v)))
		}
	}
	out := []DiffEntry{}
	for _, entry := range entries {
		if len(entry.Added)+len(entry.Removed) > 0 {
			out = append(out, *entry)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out
}
func (a *App) runDiff(i Invocation) error {
	if !IsGitRepo(a.Cwd) {
		if !a.JSONMode {
			a.Error("diff", "not in a git repository.")
			return nil
		}
		return fmt.Errorf("Not in a git repository. Run this command from within a git repository.")
	}
	since := i.String("since")
	text, _ := Git(a.Cwd, "diff", "--end-of-options", since, "--", ".luuna/atlas/expertise/")
	entries := ParseExpertiseDiff(text)
	if len(entries) == 0 {
		if a.JSONMode {
			a.OK("diff", map[string]any{"since": since, "domains": []any{}, "message": "No expertise changes found."})
		} else {
			fmt.Fprintln(a.Out, "No expertise changes found.")
		}
		return nil
	}
	if a.JSONMode {
		a.OK("diff", map[string]any{"since": since, "domains": entries})
		return nil
	}
	fmt.Fprintln(a.Out, Style("bold", fmt.Sprintf("\nExpertise changes since %s\n", since)))
	for _, d := range entries {
		n := len(d.Added) + len(d.Removed)
		suffix := "s"
		if n == 1 {
			suffix = ""
		}
		a.Human(Style("cyan", fmt.Sprintf("%s (%d change%s):", d.Domain, n, suffix)))
		for _, r := range d.Added {
			a.Human(Style("green", fmt.Sprintf("  + [%s] %s %s", Text(r["type"]), Style("accent", Text(r["id"])), RecordSummary(r, a.Registry))))
		}
		for _, r := range d.Removed {
			a.Human(Style("red", fmt.Sprintf("  - [%s] %s %s", Text(r["type"]), Style("accent", Text(r["id"])), RecordSummary(r, a.Registry))))
		}
		a.Human("")
	}
	return nil
}
