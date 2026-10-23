package atlas

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const HookBudget = 500

type InjectRequest struct {
	File, Cwd, Session, Tool string
	Now                      time.Time
	Registry                 *Registry
	AllowUnknownTypes        bool
}

var sessionUnsafeRE = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func FindRepoRoot(cwd string) string {
	dir, _ := filepath.Abs(cwd)
	for {
		if setupExists(filepath.Join(dir, ".git")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			abs, _ := filepath.Abs(cwd)
			return abs
		}
		dir = parent
	}
}
func hookRepoRelative(file, cwd string) string {
	root := FindRepoRoot(cwd)
	if !filepath.IsAbs(file) {
		file = filepath.Join(cwd, file)
	}
	file, _ = filepath.Abs(file)
	rel, e := filepath.Rel(root, file)
	if e != nil || rel == "" || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return ""
	}
	return filepath.ToSlash(rel)
}
func hookSessionFile(dir, session string) string {
	return filepath.Join(dir, "sessions", sessionUnsafeRE.ReplaceAllString(session, "_")+".txt")
}
func recordHookInjection(dir string, req InjectRequest, rel string, ids []string) error {
	sessions := filepath.Join(dir, "sessions")
	if e := os.MkdirAll(sessions, 0777); e != nil {
		return e
	}
	ignore := filepath.Join(dir, ".gitignore")
	if !setupExists(ignore) {
		if e := os.WriteFile(ignore, []byte("*\n"), 0666); e != nil {
			return e
		}
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	if req.Session != "" {
		path := hookSessionFile(dir, req.Session)
		if !setupExists(path) {
			entries, e := os.ReadDir(sessions)
			if e != nil {
				return e
			}
			for _, entry := range entries {
				p := filepath.Join(sessions, entry.Name())
				s, e := os.Stat(p)
				if e != nil {
					return e
				}
				if now.Sub(s.ModTime()) > 7*24*time.Hour {
					if e := os.Remove(p); e != nil {
						return e
					}
				}
			}
		}
		f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
		if e != nil {
			return e
		}
		_, e = f.WriteString(strings.Join(ids, "\n") + "\n")
		closeErr := f.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
	}
	var session any
	if req.Session != "" {
		session = req.Session
	}
	line := map[string]any{"ts": ISO(now), "session": session, "tool": req.Tool, "files": []string{rel}, "ids": ids}
	rememberOrder(line, []string{"ts", "session", "tool", "files", "ids"})
	b, e := MarshalJSON(line)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(dir, "usage.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if e != nil {
		return e
	}
	_, e = f.Write(append(b, '\n'))
	closeErr := f.Close()
	if e != nil {
		return e
	}
	return closeErr
}
func hookBudget(domains []DomainRecords, registry *Registry, stale map[string]bool) []Record {
	type candidate struct {
		R     Record
		Index int
	}
	original := []candidate{}
	for _, d := range domains {
		for _, r := range d.Records {
			original = append(original, candidate{r, len(original)})
		}
	}
	ranked := append([]candidate{}, original...)
	class := map[string]int{"foundational": 0, "tactical": 1, "observational": 2}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i].R, ranked[j].R
		af, bf := Text(a["type"]) == "failure", Text(b["type"]) == "failure"
		if af != bf {
			return af
		}
		as, bs := stale[Text(a["id"])], stale[Text(b["id"])]
		if as != bs {
			return !as
		}
		ac, bc := ConfirmationScore(a), ConfirmationScore(b)
		if ac != bc {
			return ac > bc
		}
		at, bt := class[Text(a["classification"])], class[Text(b["classification"])]
		if at != bt {
			return at < bt
		}
		return ParseDate(Text(a["recorded_at"])).After(ParseDate(Text(b["recorded_at"])))
	})
	kept := map[int]bool{}
	used := 0
	for _, c := range ranked {
		cost := int(math.Ceil(float64(formatLen(FormatRecordIndex(c.R, registry, false))) / 4))
		pinned := Text(c.R["type"]) == "failure"
		if pinned || used+cost <= HookBudget {
			kept[c.Index] = true
			used += cost
		}
	}
	out := []Record{}
	for _, c := range original {
		if kept[c.Index] {
			out = append(out, c.R)
		}
	}
	rank := func(r Record) int {
		n := 1
		if Text(r["type"]) == "failure" {
			n = 0
		}
		if stale[Text(r["id"])] {
			n += 2
		}
		return n
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}
func InjectForFile(req InjectRequest) (string, error) {
	dir, e := AtlasDir(req.Cwd)
	if e != nil {
		return "", e
	}
	if !setupExists(filepath.Join(dir, "config.yaml")) {
		return "", nil
	}
	rel := hookRepoRelative(req.File, req.Cwd)
	if rel == "" || strings.HasPrefix(rel, ".luuna/atlas/") {
		return "", nil
	}
	cfg, e := ReadConfig(req.Cwd)
	if e != nil {
		return "", e
	}
	registry := req.Registry
	if registry == nil {
		registry = GetRegistry()
	}
	seen := map[string]bool{}
	state := filepath.Join(dir, "state")
	if req.Session != "" {
		b, _ := os.ReadFile(hookSessionFile(state, req.Session))
		for _, id := range strings.Split(string(b), "\n") {
			if id != "" {
				seen[id] = true
			}
		}
	}
	all := []DomainRecords{}
	for _, domain := range DomainNames(cfg) {
		path, e := ExpertisePath(req.Cwd, domain)
		if e != nil {
			return "", e
		}
		records, e := ReadRecords(path, registry, req.AllowUnknownTypes)
		if e != nil {
			return "", e
		}
		all = append(all, DomainRecords{Domain: domain, Records: records})
	}
	superseded, _ := CollectSupersededIDs(all)
	scoped := []DomainRecords{}
	flat := []Record{}
	for _, d := range all {
		records := []Record{}
		for _, r := range d.Records {
			id := Text(r["id"])
			if id != "" && !superseded[id] && !seen[id] && MatchesFileAnchors(r, []string{rel}) {
				records = append(records, r)
				flat = append(flat, r)
			}
		}
		if len(records) > 0 {
			scoped = append(scoped, DomainRecords{Domain: d.Domain, Records: records})
		}
	}
	if len(scoped) == 0 {
		return "", nil
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	shelf := Obj(Obj(cfg["classification_defaults"])["shelf_life"])
	stale := PrimeStaleIDs(FindStaleRecords(flat, FindRepoRoot(req.Cwd), now, shelf))
	records := hookBudget(scoped, registry, stale)
	if len(records) == 0 {
		return "", nil
	}
	ids := []string{}
	lines := []string{}
	for _, r := range records {
		ids = append(ids, Text(r["id"]))
		lines = append(lines, FormatRecordIndex(r, registry, stale[Text(r["id"])]))
	}
	if e := recordHookInjection(state, req, rel, ids); e != nil {
		return "", e
	}
	return "luuna-atlas: records anchored to " + rel + " (`luuna-atlas show <id>` for the full record)\n" + strings.Join(lines, "\n"), nil
}
func HookOutput(raw, fallbackCwd string, registry *Registry) string {
	return hookOutput(raw, fallbackCwd, registry, false)
}
func hookOutput(raw, fallbackCwd string, registry *Registry, allowUnknown bool) string {
	inputAny, e := ParseJSON([]byte(raw))
	if e != nil {
		return ""
	}
	input := Obj(inputAny)
	if event := Text(input["hook_event_name"]); event != "" && event != "PreToolUse" {
		return ""
	}
	cwd := Text(input["cwd"])
	if cwd == "" {
		cwd = fallbackCwd
	}
	extra := map[string]any{"hookEventName": "PreToolUse"}
	if reason := GuardDecision(input, cwd); reason != "" {
		extra["permissionDecision"] = "deny"
		extra["permissionDecisionReason"] = reason
		rememberOrder(extra, []string{"hookEventName", "permissionDecision", "permissionDecisionReason"})
	} else {
		tool := Text(input["tool_name"])
		key := fileToolFields[tool]
		if tool == "Read" {
			key = "file_path"
		}
		if key == "" {
			return ""
		}
		file := Text(Obj(input["tool_input"])[key])
		if file == "" {
			return ""
		}
		context, e := InjectForFile(InjectRequest{File: file, Cwd: cwd, Session: Text(input["session_id"]), Tool: tool, Registry: registry, AllowUnknownTypes: allowUnknown})
		if e != nil || context == "" {
			return ""
		}
		extra["additionalContext"] = context
		rememberOrder(extra, []string{"hookEventName", "additionalContext"})
	}
	b, e := MarshalJSON(map[string]any{"hookSpecificOutput": extra})
	if e != nil {
		return ""
	}
	return string(b) + "\n"
}
func (a *App) runHook(_ Invocation) error {
	raw, e := io.ReadAll(a.In)
	if e != nil {
		return nil
	}
	fmt.Fprint(a.Out, hookOutput(string(raw), a.Cwd, a.Registry, a.AllowUnknownTypes))
	return nil
}
