package atlas

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const ArchiveBanner = "# ARCHIVED — not for active use. Run `luuna-atlas restore <id>` to revive."

func ReadArchive(path string, reg *Registry) ([]Record, error) { return ReadRecords(path, reg, true) }
func WriteArchive(path string, records []Record) error {
	if e := os.MkdirAll(filepath.Dir(path), 0777); e != nil {
		return e
	}
	lines := []string{ArchiveBanner}
	for _, r := range records {
		b, e := MarshalJSON(r)
		if e != nil {
			return e
		}
		lines = append(lines, string(b))
	}
	return WriteFileAtomic(path, []byte(strings.Join(lines, "\n")+"\n"), 0666)
}
func ArchiveRecords(cwd, domain string, records []Record, now time.Time, reason string, reg *Registry) error {
	if len(records) == 0 {
		return nil
	}
	path, e := ArchivePath(cwd, domain)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0777); e != nil {
		return e
	}
	stamped := []Record{}
	for _, r := range records {
		clone := CloneRecord(r)
		fieldOrder := ObjectKeys(clone)
		clone["status"] = "archived"
		if clone["archived_at"] == nil {
			clone["archived_at"] = ISO(now)
		}
		if clone["archive_reason"] == nil {
			clone["archive_reason"] = reason
		}
		rememberOrder(map[string]any(clone), Unique(append(fieldOrder, "status", "archived_at", "archive_reason")))
		stamped = append(stamped, clone)
	}
	return WithFileLock(path, func() error {
		existing, e := ReadArchive(path, reg)
		if e != nil {
			return e
		}
		return WriteArchive(path, append(existing, stamped...))
	})
}
func StripArchiveFields(r Record) Record {
	copy := CloneRecord(r)
	delete(copy, "status")
	delete(copy, "archived_at")
	delete(copy, "archive_reason")
	return copy
}
func RemoveFromArchive(cwd, domain, id string, reg *Registry) (Record, error) {
	path, e := ArchivePath(cwd, domain)
	if e != nil {
		return nil, e
	}
	var removed Record
	e = WithFileLock(path, func() error {
		records, e := ReadArchive(path, reg)
		if e != nil {
			return e
		}
		for i, r := range records {
			if Text(r["id"]) == id {
				removed = r
				return WriteArchive(path, append(records[:i], records[i+1:]...))
			}
		}
		return nil
	})
	return removed, e
}
func RestoreToExpertise(path string, r Record, reg *Registry) (Record, error) {
	var conflict Record
	e := WithFileLock(path, func() error {
		existing, e := ReadRecords(path, reg, false)
		if e != nil {
			return e
		}
		if r["id"] != nil {
			for _, row := range existing {
				if Text(row["id"]) == Text(r["id"]) {
					conflict = row
					return nil
				}
			}
		}
		return WriteRecords(path, append(existing, r))
	})
	return conflict, e
}
func IsRecordStale(r Record, now time.Time, shelf map[string]any) bool {
	class := Text(r["classification"])
	if class == "foundational" {
		return false
	}
	at := ParseDate(Text(r["recorded_at"]))
	if at.IsZero() {
		return false
	}
	age := math.Floor(now.Sub(at).Hours() / 24)
	switch class {
	case "tactical":
		return age > NumberDefault(shelf["tactical"], 14)
	case "observational":
		return age > NumberDefault(shelf["observational"], 30)
	}
	return false
}
func RecordAnchors(r Record) []Anchor {
	out := []Anchor{}
	for _, f := range List(r["files"]) {
		if f != "" {
			out = append(out, Anchor{Kind: "file", Path: f})
		}
	}
	for _, d := range List(r["dir_anchors"]) {
		if d != "" {
			out = append(out, Anchor{Kind: "dir", Path: d})
		}
	}
	if file := Text(Obj(r["evidence"])["file"]); file != "" {
		out = append(out, Anchor{Kind: "evidence_file", Path: file})
	}
	return out
}
func AnchorValidity(r Record, cwd string) AnchorValidityResult {
	anchors := RecordAnchors(r)
	v := AnchorValidityResult{Total: len(anchors), Broken: []Anchor{}}
	for _, anchor := range anchors {
		path := anchor.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(cwd, path)
		}
		if exists(path) {
			v.Valid++
		} else {
			v.Broken = append(v.Broken, anchor)
		}
	}
	if v.Total > 0 {
		ratio := float64(v.Valid) / float64(v.Total)
		v.ValidFraction = &ratio
	}
	return v
}
func PassedAnchorGrace(r Record, now time.Time, grace float64) bool {
	t := ParseDate(Text(r["recorded_at"]))
	return !t.IsZero() && now.Sub(t).Hours()/24 > grace
}
func CollectSupersededIDs(domains []DomainRecords) (map[string]bool, map[string]bool) {
	graph := map[string][]string{}
	allEdges := [][2]string{}
	for _, domain := range domains {
		for _, r := range domain.Records {
			id := Text(r["id"])
			if id == "" || len(List(r["supersedes"])) == 0 {
				continue
			}
			if _, ok := graph[id]; !ok {
				graph[id] = []string{}
			}
			for _, target := range List(r["supersedes"]) {
				if target == id {
					continue
				}
				graph[id] = Unique(append(graph[id], target))
				if _, ok := graph[target]; !ok {
					graph[target] = []string{}
				}
				allEdges = append(allEdges, [2]string{id, target})
			}
		}
	}
	index := 0
	indices, low := map[string]int{}, map[string]int{}
	stack := []string{}
	onStack := map[string]bool{}
	cycles := map[string]bool{}
	var visit func(string)
	visit = func(v string) {
		indices[v] = index
		low[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range graph[v] {
			if _, seen := indices[w]; !seen {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && indices[w] < low[v] {
				low[v] = indices[w]
			}
		}
		if low[v] == indices[v] {
			component := []string{}
			for len(stack) > 0 {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				component = append(component, w)
				if w == v {
					break
				}
			}
			if len(component) > 1 {
				for _, w := range component {
					cycles[w] = true
				}
			}
		}
	}
	for v := range graph {
		if _, seen := indices[v]; !seen {
			visit(v)
		}
	}
	superseded := map[string]bool{}
	for _, edge := range allEdges {
		if !cycles[edge[1]] {
			superseded[edge[1]] = true
		}
	}
	return superseded, cycles
}
func FindStaleRecords(records []Record, cwd string, now time.Time, shelf map[string]any) []StaleEntry {
	order := []string{}
	reasons := map[string][]StaleReason{}
	byID := map[string]Record{}
	candidates := []Record{}
	for _, r := range records {
		id := Text(r["id"])
		if id == "" {
			continue
		}
		byID[id] = r
		local := []StaleReason{}
		valid := AnchorValidity(r, cwd)
		if len(valid.Broken) > 0 {
			paths := []string{}
			for _, b := range valid.Broken {
				paths = append(paths, b.Path)
			}
			local = append(local, StaleReason{Kind: "anchors_missing", Detail: "missing anchors: " + strings.Join(paths, ", ")})
		}
		if IsRecordStale(r, now, shelf) && ConfirmationScore(r) == 0 {
			age := int(math.Floor(now.Sub(ParseDate(Text(r["recorded_at"]))).Hours() / 24))
			local = append(local, StaleReason{Kind: "shelf_life", Detail: fmt.Sprintf("%s, %dd old, never confirmed", Text(r["classification"]), age)})
		}
		if len(local) > 0 {
			if _, seen := reasons[id]; !seen {
				order = append(order, id)
			}
			reasons[id] = local
		}
		if Text(Obj(r["evidence"])["commit"]) != "" && len(valid.Broken) == 0 && valid.Total > 0 {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) > 0 {
		oldest := now
		paths := []string{}
		for _, r := range candidates {
			t := ParseDate(Text(r["recorded_at"]))
			if !t.IsZero() && t.Before(oldest) {
				oldest = t
			}
			for _, a := range RecordAnchors(r) {
				paths = append(paths, a.Path)
			}
		}
		since := ISO(oldest.Add(-24 * time.Hour))
		history, e1 := Git(cwd, "log", "--format=%H", "--since="+since)
		args := []string{"log", "--no-renames", "--relative", "--name-only", "--format=%H", "--since=" + since, "--"}
		args = append(args, Unique(paths)...)
		raw, e2 := Git(cwd, args...)
		if e1 == nil && e2 == nil {
			commitOrder := strings.Fields(history)
			changes := map[string][]string{}
			current := ""
			shaRE := regexp.MustCompile(`^[0-9a-f]{40}$`)
			for _, line := range strings.Split(raw, "\n") {
				if line == "" {
					continue
				}
				if shaRE.MatchString(line) {
					current = line
					changes[current] = []string{}
				} else if current != "" {
					changes[current] = append(changes[current], line)
				}
			}
			for _, r := range candidates {
				sha := Text(Obj(r["evidence"])["commit"])
				idx := -1
				for i, h := range commitOrder {
					if strings.HasPrefix(h, sha) {
						idx = i
						break
					}
				}
				if idx < 0 {
					continue
				}
				touches := []string{}
				commits := 0
				for _, h := range commitOrder[:idx] {
					hits := []string{}
					for _, f := range changes[h] {
						for _, a := range RecordAnchors(r) {
							anchor := strings.TrimPrefix(a.Path, "./")
							if (a.Kind == "dir" && FileLivesUnderDir(f, anchor)) || (a.Kind != "dir" && f == anchor) {
								hits = append(hits, f)
								break
							}
						}
					}
					if len(hits) > 0 {
						commits++
						touches = Unique(append(touches, hits...))
					}
				}
				if commits >= 5 {
					id := Text(r["id"])
					if _, seen := reasons[id]; !seen {
						order = append(order, id)
					}
					first := ""
					if len(touches) > 0 {
						first = touches[0]
					}
					more := ""
					if len(touches) > 1 {
						more = fmt.Sprintf(" (+%d more)", len(touches)-1)
					}
					if len(sha) > 7 {
						sha = sha[:7]
					}
					reasons[id] = append(reasons[id], StaleReason{Kind: "anchors_changed", Detail: fmt.Sprintf("%s%s changed in %d commits since %s", first, more, commits, sha)})
				}
			}
		}
	}
	out := []StaleEntry{}
	for _, id := range order {
		out = append(out, StaleEntry{Record: byID[id], Reasons: reasons[id]})
	}
	return out
}
func PrimeStaleIDs(stale []StaleEntry) map[string]bool {
	out := map[string]bool{}
	for _, entry := range stale {
		for _, r := range entry.Reasons {
			if r.Kind == "anchors_changed" || r.Kind == "anchors_missing" {
				out[Text(entry.Record["id"])] = true
			}
		}
	}
	return out
}
func CalculateDomainHealth(records []Record, maxEntries float64, shelf map[string]any, registry *Registry) map[string]any {
	types := map[string]any{}
	if registry != nil {
		for _, name := range registry.Order {
			types[name] = 0
		}
	}
	classes := map[string]any{"foundational": 0, "tactical": 0, "observational": 0}
	stale := 0
	oldest, newest := "", ""
	for _, r := range records {
		t, c := Text(r["type"]), Text(r["classification"])
		types[t] = Number(types[t]) + 1
		classes[c] = Number(classes[c]) + 1
		if IsRecordStale(r, time.Now(), shelf) {
			stale++
		}
		recorded := Text(r["recorded_at"])
		if oldest == "" || recorded < oldest {
			oldest = recorded
		}
		if newest == "" || recorded > newest {
			newest = recorded
		}
	}
	var oldValue, newValue any
	if oldest != "" {
		oldValue = oldest
	}
	if newest != "" {
		newValue = newest
	}
	util := 0.0
	if maxEntries > 0 {
		util = math.Round(float64(len(records)) / maxEntries * 100)
	}
	return map[string]any{"governance_utilization": util, "stale_count": stale, "type_distribution": types, "classification_distribution": classes, "oldest_timestamp": oldValue, "newest_timestamp": newValue}
}
