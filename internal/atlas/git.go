package atlas

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func Git(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	b, e := cmd.Output()
	return string(b), e
}
func IsGitRepo(cwd string) bool {
	_, e := Git(cwd, "rev-parse", "--is-inside-work-tree")
	return e == nil
}
func GetCurrentCommit(cwd string) string {
	out, e := Git(cwd, "rev-parse", "HEAD")
	if e != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
func GetChangedFiles(cwd, since string) []string {
	files := []string{}
	for _, args := range [][]string{{"diff", "--name-only", "--end-of-options", since}, {"diff", "--name-only", "--cached"}, {"diff", "--name-only"}} {
		raw, e := Git(cwd, args...)
		if e != nil {
			continue
		}
		for _, f := range strings.Split(strings.TrimSpace(raw), "\n") {
			if f != "" {
				files = append(files, f)
			}
		}
	}
	files = Unique(files)
	sort.Strings(files)
	return files
}
func GetContextFiles(cwd string) []string {
	files := []string{}
	for _, args := range [][]string{{"diff", "--name-only", "--cached"}, {"diff", "--name-only"}} {
		raw, e := Git(cwd, args...)
		if e != nil {
			continue
		}
		for _, f := range strings.Split(strings.TrimSpace(raw), "\n") {
			if f != "" {
				files = append(files, f)
			}
		}
	}
	files = Unique(files)
	sort.Strings(files)
	return files
}
func GetActiveFiles(cwd string) []string {
	out, e := Git(cwd, "status", "--porcelain", "-uall")
	if e != nil {
		return []string{}
	}
	files := []string{}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		path := line[3:]
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		if path != "" {
			files = append(files, path)
		}
	}
	files = Unique(files)
	sort.Strings(files)
	return files
}
func FileMatchesAny(file string, changed []string) bool {
	suffix := func(longer, shorter string) bool {
		return len(longer) > len(shorter) && strings.HasSuffix(longer, "/"+shorter)
	}
	for _, f := range changed {
		if f == file || suffix(f, file) || suffix(file, f) {
			return true
		}
	}
	return false
}
func normalizeDirAnchor(input string) string {
	v := strings.TrimSpace(input)
	if v == "" || v == "." {
		return ""
	}
	v = regexp.MustCompile(`\\+`).ReplaceAllString(v, "/")
	v = strings.TrimRight(v, "/")
	v = strings.TrimPrefix(v, "./")
	v = strings.TrimRight(v, "/")
	if v == "." {
		return ""
	}
	return v
}
func NormalizeDirAnchor(input string) (string, error) {
	v := strings.TrimSpace(input)
	if v != "" {
		if regexp.MustCompile(`^[a-zA-Z]:[\\/]`).MatchString(v) || strings.HasPrefix(v, "/") || strings.HasPrefix(v, `\`) {
			return "", fmt.Errorf("dir-anchor %q is an absolute path. Use a project-root-relative path like \"src/utils\".", input)
		}
		if Contains(strings.Split(regexp.MustCompile(`\\+`).ReplaceAllString(v, "/"), "/"), "..") {
			return "", fmt.Errorf("dir-anchor %q contains \"..\" (parent traversal). Anchors must stay inside the project root.", input)
		}
	}
	return normalizeDirAnchor(input), nil
}
func NormalizeDirAnchors(inputs []string) ([]string, error) {
	out := []string{}
	for _, input := range inputs {
		v, e := NormalizeDirAnchor(input)
		if e != nil {
			return nil, e
		}
		if v != "" {
			out = append(out, v)
		}
	}
	return Unique(out), nil
}
func FileLivesUnderDir(file, dir string) bool {
	f := regexp.MustCompile(`\\+`).ReplaceAllString(file, "/")
	d := normalizeDirAnchor(dir)
	return d == "" || f == d || strings.HasPrefix(f, d+"/")
}
func InferDirAnchors(files []string) []string {
	counts := map[string]int{}
	for _, raw := range files {
		f := regexp.MustCompile(`\\+`).ReplaceAllString(raw, "/")
		idx := strings.LastIndex(f, "/")
		if idx > 0 {
			counts[f[:idx]]++
		}
	}
	out := []string{}
	for d, n := range counts {
		if n >= 3 {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}
func HasFileAnchors(r Record) bool {
	return len(List(r["files"])) > 0 || len(List(r["dir_anchors"])) > 0
}
func MatchesFileAnchors(r Record, files []string) bool {
	for _, f := range List(r["files"]) {
		if FileMatchesAny(f, files) {
			return true
		}
	}
	for _, dir := range List(r["dir_anchors"]) {
		for _, file := range files {
			if FileLivesUnderDir(file, dir) {
				return true
			}
		}
	}
	return false
}
func FilterByContext(records []Record, files []string) []Record {
	out := []Record{}
	for _, r := range records {
		if !HasFileAnchors(r) || MatchesFileAnchors(r, files) {
			out = append(out, r)
		}
	}
	return out
}
func ActiveContextHasSignal(ctx ActiveContext) bool {
	if len(ctx.ChangedFiles) > 0 {
		return true
	}
	for _, v := range ctx.Trackers {
		if v != "" {
			return true
		}
	}
	return false
}
func FilterByActiveContext(records []Record, ctx ActiveContext) []Record {
	out := []Record{}
	for _, r := range records {
		ev := Obj(r["evidence"])
		matches := false
		for _, tracker := range []string{"missions", "gh", "linear", "bead"} {
			v := Text(ev[tracker])
			if tracker == "missions" {
				v = EvidenceMissions(ev)
			}
			if ctx.Trackers[tracker] != "" && v == ctx.Trackers[tracker] {
				matches = true
			}
		}
		if matches || !HasFileAnchors(r) || MatchesFileAnchors(r, ctx.ChangedFiles) {
			out = append(out, r)
		}
	}
	return out
}
func readTrackerRows(path string) []Record {
	data, e := os.ReadFile(path)
	if e != nil {
		return []Record{}
	}
	out := []Record{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		v, e := ParseJSON([]byte(line))
		if e == nil {
			out = append(out, Record(Obj(v)))
		}
	}
	return out
}
func inProgressIDs(path string) []string {
	out := []string{}
	for _, r := range readTrackerRows(path) {
		if Text(r["status"]) == "in_progress" {
			if id := Text(r["id"]); id != "" {
				out = append(out, id)
			}
		}
	}
	out = Unique(out)
	sort.Strings(out)
	return out
}
func trackerMatches(re *regexp.Regexp, text string, group int) []string {
	out := []string{}
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		if group < len(m) && m[group] != "" {
			out = append(out, m[group])
		}
	}
	out = Unique(out)
	sort.Strings(out)
	return out
}
func ResolveActiveWork(cwd string, overrides map[string]string) ActiveWorkResult {
	result := ActiveWorkResult{Warnings: []string{}}
	branch, _ := Git(cwd, "rev-parse", "--abbrev-ref", "HEAD")
	branch = strings.TrimSpace(branch)
	if branch == "HEAD" {
		branch = ""
	}
	type hit struct {
		matches []string
		source  string
	}
	hits := map[string]hit{}
	missions := inProgressIDs(filepath.Join(ResolveMissionsDir(cwd), "issues.jsonl"))
	source := "in_progress"
	if len(missions) == 0 {
		missions = trackerMatches(regexp.MustCompile(`\b([a-z][a-z0-9]*-[a-f0-9]{4,8})\b`), branch, 1)
		source = "branch"
	}
	hits["missions"] = hit{missions, source}
	gh := []string{}
	source = "gh-cli"
	if branch != "" {
		cmd := exec.Command("gh", "pr", "view", "--json", "number", "--jq", ".number")
		cmd.Dir = cwd
		if data, e := cmd.Output(); e == nil {
			v := strings.TrimSpace(string(data))
			if regexp.MustCompile(`^\d+$`).MatchString(v) {
				gh = []string{"#" + v}
			}
		}
	}
	if len(gh) == 0 {
		source = "branch"
		for _, v := range trackerMatches(regexp.MustCompile(`(?:^|[^a-zA-Z0-9])(?:gh-|#)(\d+)\b`), branch, 1) {
			gh = append(gh, "#"+v)
		}
	}
	hits["gh"] = hit{gh, source}
	hits["linear"] = hit{trackerMatches(regexp.MustCompile(`\b([A-Z]{2,}[A-Z0-9]*-\d+)\b`), branch, 1), "branch"}
	bead := inProgressIDs(filepath.Join(cwd, ".beads", "issues.jsonl"))
	source = "in_progress"
	if len(bead) == 0 {
		source = "branch"
		bead = trackerMatches(regexp.MustCompile(`(?i)\b((?:bd|bead)-\d+)\b`), branch, 1)
		for i := range bead {
			bead[i] = JSLower(bead[i])
		}
		sort.Strings(bead)
	}
	hits["bead"] = hit{bead, source}
	values := map[string]string{}
	for _, tracker := range []string{"missions", "gh", "linear", "bead"} {
		if overrides[tracker] != "" {
			values[tracker] = overrides[tracker]
			continue
		}
		h := hits[tracker]
		if len(h.matches) == 1 {
			values[tracker] = h.matches[0]
		} else if len(h.matches) > 1 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("active-work: multiple %s candidates [%s] from %s; pass --evidence-%s <id> to disambiguate", tracker, strings.Join(h.matches, ", "), h.source, tracker))
		}
	}
	result.Missions = values["missions"]
	result.GH = values["gh"]
	result.Linear = values["linear"]
	result.Bead = values["bead"]
	return result
}
