package atlas

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) runAdd(i Invocation) error {
	dir, err := AtlasDir(a.Cwd)
	if err != nil {
		return err
	}
	if !exists(dir) {
		return fmt.Errorf("No .luuna/atlas/ directory found. Run `luuna-atlas init` first.")
	}
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	domain := i.Positionals[0]
	if _, ok := Obj(cfg["domains"])[domain]; ok {
		return fmt.Errorf("Domain %q already exists.", domain)
	}
	if err = AddDomain(a.Cwd, domain); err != nil {
		return err
	}
	if a.JSONMode {
		a.OK("add", map[string]any{"domain": domain})
	} else {
		a.Human(Style("green", fmt.Sprintf("Added domain %q.", domain)))
	}
	return nil
}

func filterRead(records []Record, i Invocation) []Record {
	out := []Record{}
	for _, r := range records {
		if t := i.String("type"); t != "" && Text(r["type"]) != t {
			continue
		}
		if c := i.String("classification"); c != "" && Text(r["classification"]) != c {
			continue
		}
		if f := i.String("file"); f != "" {
			match := false
			for _, p := range List(r["files"]) {
				if strings.Contains(JSLower(p), JSLower(f)) {
					match = true
				}
			}
			if !match {
				continue
			}
		}
		if tag := i.String("tag"); tag != "" {
			match := false
			for _, v := range List(r["tags"]) {
				if JSLower(v) == JSLower(tag) {
					match = true
				}
			}
			if !match {
				continue
			}
		}
		if status := i.String("outcome-status"); status != "" {
			match := false
			for _, v := range AnyList(r["outcomes"]) {
				if Text(Obj(v)["status"]) == status {
					match = true
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, r)
	}
	if i.Bool("sort-by-score") {
		sort.SliceStable(out, func(x, y int) bool { return ConfirmationScore(out[x]) > ConfirmationScore(out[y]) })
	}
	return out
}

func (a *App) knownDomain(cfg Config, domain string, hint bool) bool {
	if _, ok := Obj(cfg["domains"])[domain]; ok {
		return true
	}
	available := strings.Join(DomainNames(cfg), ", ")
	if available == "" && a.Command != "prime" && a.Command != "search" {
		available = "(none)"
	}
	a.Error(a.Command, fmt.Sprintf("Domain %q not found in config. Available domains: %s", domain, available))
	if hint && !a.JSONMode {
		fmt.Fprintf(a.Err, "Hint: Run `luuna-atlas add %s` to create this domain, or check .luuna/atlas/config.yaml\n", domain)
	}
	return false
}
func (a *App) renderFormat(i Invocation) string {
	if f := i.String("format"); f != "" {
		return f
	}
	if a.Format != "" {
		return a.Format
	}
	return "markdown"
}

func (a *App) runQuery(i Invocation) error {
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	domains := DomainNames(cfg)
	if !i.Bool("all") {
		if len(i.Positionals) == 0 {
			return fmt.Errorf("Please specify a domain or use --all to query all domains.")
		}
		if !a.knownDomain(cfg, i.Positionals[0], true) {
			return nil
		}
		domains = []string{i.Positionals[0]}
	}
	if len(domains) == 0 {
		if a.JSONMode {
			a.OK("query", map[string]any{"domains": []any{}})
		} else {
			fmt.Fprintln(a.Out, "No domains configured. Run `luuna-atlas add <domain>` to get started.")
		}
		return nil
	}
	results := []any{}
	sections := []string{}
	ids := []string{}
	format := a.renderFormat(i)
	for _, domain := range domains {
		records, e := a.readDomain(domain)
		if e != nil {
			return e
		}
		records = filterRead(records, i)
		results = append(results, map[string]any{"domain": domain, "records": records})
		if format == "ids" {
			for _, r := range records {
				if id := Text(r["id"]); id != "" {
					ids = append(ids, id)
				}
			}
		} else {
			path, _ := ExpertisePath(a.Cwd, domain)
			sections = append(sections, FormatDomain(domain, records, fileModTime(path), a.Registry, FormatOptions{Format: format}))
		}
	}
	if a.JSONMode {
		a.OK("query", map[string]any{"domains": results})
	} else if format == "ids" {
		if len(ids) > 0 {
			fmt.Fprintln(a.Out, strings.Join(ids, "\n"))
		}
	} else {
		fmt.Fprintln(a.Out, strings.Join(sections, "\n\n"))
	}
	return nil
}

func readArchiveRaw(path string) ([]Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return []Record{}, nil
	}
	out := []Record{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		value, e := ParseJSON([]byte(line))
		if e != nil {
			return nil, e
		}
		out = append(out, Record(Obj(value)))
	}
	return out, nil
}
func (a *App) runSearch(i Invocation) error {
	query := ""
	if len(i.Positionals) > 0 {
		query = i.Positionals[0]
	}
	hasFilter := false
	for _, key := range []string{"type", "domain", "tag", "classification", "file", "outcome-status"} {
		if i.String(key) != "" {
			hasFilter = true
		}
	}
	if query == "" && !hasFilter {
		return fmt.Errorf("Provide a search query or use --type, --domain, --tag, --classification, --file, or --outcome-status to filter.")
	}
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	domains := DomainNames(cfg)
	if d := i.String("domain"); d != "" {
		if !a.knownDomain(cfg, d, true) {
			return nil
		}
		domains = []string{d}
	}
	boost := NumberDefault(Obj(cfg["search"])["boost_factor"], .1)
	if i.Has("boost") && !i.Bool("boost") {
		boost = 0
	}
	results := []any{}
	sections := []string{}
	ids := []string{}
	total := 0
	format := a.renderFormat(i)
	label := "matching filters"
	if query != "" {
		label = "matching \"" + query + "\""
	}
	for _, domain := range domains {
		live, e := a.readDomain(domain)
		if e != nil {
			return e
		}
		live = filterRead(live, i)
		if query != "" {
			live = SearchRecords(live, query, boost)
		}
		if i.Bool("sort-by-score") {
			sort.SliceStable(live, func(x, y int) bool { return ConfirmationScore(live[x]) > ConfirmationScore(live[y]) })
		}
		archived := []Record{}
		if i.Bool("archived") {
			path, e := ArchivePath(a.Cwd, domain)
			if e != nil {
				return e
			}
			archived, e = readArchiveRaw(path)
			if e != nil {
				return e
			}
			archived = filterRead(archived, i)
			if query != "" {
				archived = SearchRecords(archived, query, boost)
			}
			if i.Bool("sort-by-score") {
				sort.SliceStable(archived, func(x, y int) bool { return ConfirmationScore(archived[x]) > ConfirmationScore(archived[y]) })
			}
		}
		if len(live)+len(archived) == 0 {
			continue
		}
		total += len(live) + len(archived)
		entry := map[string]any{"domain": domain, "matches": live}
		if i.Bool("archived") {
			entry["archived"] = archived
		}
		results = append(results, entry)
		for _, r := range append(append([]Record{}, live...), archived...) {
			if id := Text(r["id"]); id != "" {
				ids = append(ids, id)
			}
		}
		if len(live) > 0 {
			path, _ := ExpertisePath(a.Cwd, domain)
			sections = append(sections, FormatDomain(domain, live, fileModTime(path), a.Registry, FormatOptions{Format: format}))
		}
		if len(archived) > 0 {
			suffix := "s"
			if len(archived) == 1 {
				suffix = ""
			}
			lines := []string{fmt.Sprintf("## %s (archived, %d record%s)", domain, len(archived), suffix)}
			for _, r := range archived {
				date := Text(r["archived_at"])
				if len(date) > 10 {
					date = date[:10]
				}
				if date == "" {
					date = "unknown"
				}
				id := ""
				if Text(r["id"]) != "" {
					id = Text(r["id"]) + " "
				}
				reason := ""
				if Text(r["archive_reason"]) != "" {
					reason = " " + Text(r["archive_reason"])
				}
				lines = append(lines, fmt.Sprintf("- [ARCHIVED %s%s] %s[%s] %s", date, reason, id, Text(r["type"]), RecordSummary(r, a.Registry)))
			}
			sections = append(sections, strings.Join(lines, "\n"))
		}
	}
	if a.JSONMode {
		var q any
		if len(i.Positionals) > 0 {
			q = query
		}
		a.OK("search", map[string]any{"query": q, "total": total, "domains": results})
		return nil
	}
	if format == "ids" {
		if len(ids) > 0 {
			fmt.Fprintln(a.Out, strings.Join(ids, "\n"))
		} else {
			fmt.Fprintf(a.Out, "No records %s found.\n", label)
		}
		return nil
	}
	if len(sections) == 0 {
		fmt.Fprintf(a.Out, "No records %s found.\n", label)
	} else {
		fmt.Fprintln(a.Out, strings.Join(sections, "\n\n"))
		suffix := "es"
		if total == 1 {
			suffix = ""
		}
		fmt.Fprintf(a.Out, "\n%d match%s found.\n", total, suffix)
	}
	return nil
}

type shownRecord struct {
	Domain string `json:"domain"`
	Record Record `json:"record"`
}

func (a *App) allRecords(cfg Config, domains []string) ([]shownRecord, error) {
	out := []shownRecord{}
	if domains == nil {
		domains = DomainNames(cfg)
	}
	for _, d := range domains {
		records, e := a.readDomain(d)
		if e != nil {
			return nil, e
		}
		for _, r := range records {
			out = append(out, shownRecord{d, r})
		}
	}
	return out, nil
}
func (a *App) runShow(i Invocation) error {
	cfg, e := ReadConfig(a.Cwd)
	if e != nil {
		return e
	}
	all, e := a.allRecords(cfg, nil)
	if e != nil {
		return e
	}
	found := []shownRecord{}
	for _, id := range i.Positionals {
		prefix := "mx-" + strings.TrimPrefix(id, "mx-")
		exact, matches := []shownRecord{}, []shownRecord{}
		for _, item := range all {
			if Text(item.Record["id"]) == prefix {
				exact = append(exact, item)
			}
			if strings.HasPrefix(Text(item.Record["id"]), prefix) {
				matches = append(matches, item)
			}
		}
		if len(exact) > 0 {
			matches = exact
		}
		if len(matches) == 0 {
			return fmt.Errorf("Record %q not found. Run `luuna-atlas search` or `luuna-atlas prime` to see record IDs.", id)
		}
		if len(matches) > 1 {
			parts := []string{}
			for _, m := range matches {
				parts = append(parts, fmt.Sprintf("%s (%s)", Text(m.Record["id"]), m.Domain))
			}
			return fmt.Errorf("Ambiguous identifier %q matches %d records: %s. Use more characters to disambiguate.", id, len(matches), strings.Join(parts, ", "))
		}
		found = append(found, matches[0])
	}
	if a.JSONMode {
		a.OK("show", map[string]any{"records": found})
	} else {
		blocks := []string{}
		for _, item := range found {
			body := FormatTypeRecords([]Record{item.Record}, a.Registry, true)
			if body == "" {
				body = JSONPretty(item.Record)
			}
			blocks = append(blocks, "## "+item.Domain+"\n\n"+body)
		}
		fmt.Fprintln(a.Out, strings.Join(blocks, "\n\n"))
	}
	return nil
}

func strictPositive(s string) (int, error) {
	if !regexp.MustCompile(`^[0-9]+$`).MatchString(s) {
		return 0, fmt.Errorf("invalid")
	}
	n, e := strconv.Atoi(s)
	if e != nil || n <= 0 {
		return 0, fmt.Errorf("invalid")
	}
	return n, nil
}
func strictNonNegative(s string) (float64, error) {
	if !regexp.MustCompile(`^\d+(\.\d+)?$`).MatchString(s) {
		return 0, fmt.Errorf("invalid")
	}
	n, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsInf(n, 0) || math.IsNaN(n) || n < 0 {
		return 0, fmt.Errorf("invalid")
	}
	return n, nil
}
func (a *App) runRank(i Invocation) error {
	limit, e := strictPositive(i.String("limit"))
	if e != nil {
		return fmt.Errorf("--limit must be a positive integer (got %q).", i.String("limit"))
	}
	minScore, e := strictNonNegative(i.String("min-score"))
	if e != nil {
		return fmt.Errorf("--min-score must be a non-negative number (got %q).", i.String("min-score"))
	}
	cfg, e := ReadConfig(a.Cwd)
	if e != nil {
		return e
	}
	domains := DomainNames(cfg)
	scope := "all domains"
	if len(i.Positionals) > 0 {
		scope = i.Positionals[0]
		if !a.knownDomain(cfg, scope, true) {
			return nil
		}
		domains = []string{scope}
	}
	all, e := a.allRecords(cfg, domains)
	if e != nil {
		return e
	}
	ranked := []shownRecord{}
	for _, item := range all {
		if t := i.String("type"); t != "" && Text(item.Record["type"]) != t {
			continue
		}
		if ConfirmationScore(item.Record) >= minScore {
			ranked = append(ranked, item)
		}
	}
	sort.SliceStable(ranked, func(x, y int) bool { return ConfirmationScore(ranked[x].Record) > ConfirmationScore(ranked[y].Record) })
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	if a.JSONMode {
		rows := []any{}
		for _, item := range ranked {
			rows = append(rows, map[string]any{"domain": item.Domain, "id": item.Record["id"], "type": item.Record["type"], "score": ConfirmationScore(item.Record), "summary": RecordSummary(item.Record, a.Registry), "record": item.Record})
		}
		a.OK("rank", map[string]any{"count": len(rows), "records": rows})
		return nil
	}
	if len(ranked) == 0 {
		fmt.Fprintln(a.Out, "No records match the ranking criteria.")
		return nil
	}
	suffix := "s"
	if len(ranked) == 1 {
		suffix = ""
	}
	fmt.Fprintf(a.Out, "Top %d record%s by confirmation score (%s)\n\n", len(ranked), suffix, scope)
	for _, item := range ranked {
		id := ""
		if Text(item.Record["id"]) != "" {
			id = Style("accent", Text(item.Record["id"])) + "  "
		}
		score := strconv.FormatFloat(ConfirmationScore(item.Record), 'f', -1, 64)
		fmt.Fprintf(a.Out, "  %s%s%s%s%s\n", id, Style("green", fmt.Sprintf("%-6s", "★"+score)), Style("cyan", fmt.Sprintf("%-14s", item.Domain)), Style("yellow", fmt.Sprintf("%-14s", "["+Text(item.Record["type"])+"]")), RecordSummary(item.Record, a.Registry))
	}
	return nil
}

func (a *App) runReady(i Invocation) error {
	cfg, e := ReadConfig(a.Cwd)
	if e != nil {
		return e
	}
	limit, e := strictPositive(i.String("limit"))
	if e != nil {
		return fmt.Errorf("--limit must be a positive integer (got %q).", i.String("limit"))
	}
	domains := DomainNames(cfg)
	if d := i.String("domain"); d != "" {
		if _, ok := Obj(cfg["domains"])[d]; !ok {
			return fmt.Errorf("domain %q not found. Available: %s", d, strings.Join(domains, ", "))
		}
		domains = []string{d}
	}
	var cutoff time.Time
	if since := i.String("since"); since != "" {
		m := regexp.MustCompile(`^(\d+)(h|d|w)$`).FindStringSubmatch(since)
		if m == nil {
			return fmt.Errorf("Invalid duration: %q. Use format like \"24h\", \"7d\", \"2w\".", since)
		}
		n, _ := strconv.ParseFloat(m[1], 64)
		hours := n
		if m[2] == "d" {
			hours *= 24
		}
		if m[2] == "w" {
			hours *= 168
		}
		cutoff = time.Now().Add(-time.Duration(hours * float64(time.Hour)))
	}
	all, e := a.allRecords(cfg, domains)
	if e != nil {
		return e
	}
	sort.SliceStable(all, func(x, y int) bool {
		return ParseDate(Text(all[x].Record["recorded_at"])).After(ParseDate(Text(all[y].Record["recorded_at"])))
	})
	entries := []shownRecord{}
	for _, item := range all {
		if !cutoff.IsZero() && ParseDate(Text(item.Record["recorded_at"])).Before(cutoff) {
			continue
		}
		entries = append(entries, item)
		if len(entries) >= limit {
			break
		}
	}
	if a.JSONMode {
		rows := []any{}
		for _, item := range entries {
			rows = append(rows, map[string]any{"domain": item.Domain, "id": item.Record["id"], "type": item.Record["type"], "recorded_at": item.Record["recorded_at"], "summary": RecordSummary(item.Record, a.Registry), "record": item.Record})
		}
		a.OK("ready", map[string]any{"count": len(rows), "entries": rows})
		return nil
	}
	if len(entries) == 0 {
		fmt.Fprintln(a.Out, "No recent expertise records found.")
		return nil
	}
	header := fmt.Sprintf("Recent Expertise (last %d records)", len(entries))
	if i.String("since") != "" {
		header = "Recent Expertise (last " + i.String("since") + ")"
	}
	fmt.Fprintln(a.Out, header+"\n")
	for _, item := range entries {
		id := ""
		if Text(item.Record["id"]) != "" {
			id = Style("accent", Text(item.Record["id"])) + "  "
		}
		fmt.Fprintf(a.Out, "  %s%s%s%s%s\n", id, Style("dim", fmt.Sprintf("%-10s", FormatTimeAgo(ParseDate(Text(item.Record["recorded_at"]))))), Style("cyan", fmt.Sprintf("%-14s", item.Domain)), Style("yellow", fmt.Sprintf("%-14s", "["+Text(item.Record["type"])+"]")), RecordSummary(item.Record, a.Registry))
	}
	return nil
}

func (a *App) runStatus(i Invocation) error {
	dir, e := AtlasDir(a.Cwd)
	if e != nil {
		return e
	}
	if !exists(dir) {
		return fmt.Errorf("No .luuna/atlas/ directory found. Run `luuna-atlas init` first.")
	}
	cfg, e := ReadConfig(a.Cwd)
	if e != nil {
		return e
	}
	gov := Obj(cfg["governance"])
	shelf := Obj(Obj(cfg["classification_defaults"])["shelf_life"])
	rows := []any{}
	stats := []DomainStatus{}
	for _, domain := range DomainNames(cfg) {
		records, e := a.readDomain(domain)
		if e != nil {
			return e
		}
		types := map[string]int{}
		for _, name := range a.Registry.Order {
			types[name] = 0
		}
		classes := map[string]int{"foundational": 0, "tactical": 0, "observational": 0}
		stale := 0
		oldest, newest := "", ""
		for _, r := range records {
			types[Text(r["type"])]++
			classes[Text(r["classification"])]++
			if IsRecordStale(r, time.Now(), shelf) {
				stale++
			}
			date := Text(r["recorded_at"])
			if oldest == "" || date < oldest {
				oldest = date
			}
			if newest == "" || date > newest {
				newest = date
			}
		}
		var old, new *time.Time
		var oldJSON, newJSON any
		if oldest != "" {
			t := ParseDate(oldest)
			old = &t
			oldJSON = oldest
		}
		if newest != "" {
			t := ParseDate(newest)
			new = &t
			newJSON = newest
		}
		rotting := false
		var days *int
		if new != nil {
			age := int(math.Floor(time.Since(*new).Hours() / 24))
			if age > int(Number(shelf["observational"])) {
				rotting = true
				days = &age
			}
		}
		path, _ := ExpertisePath(a.Cwd, domain)
		mod := fileModTime(path)
		util := 0
		if Number(gov["max_entries"]) > 0 {
			util = int(math.Floor(float64(len(records))/Number(gov["max_entries"])*100 + .5))
		}
		health := map[string]any{"governance_utilization": util, "stale_count": stale, "type_distribution": types, "classification_distribution": classes, "oldest_timestamp": oldJSON, "newest_timestamp": newJSON}
		var modJSON, dayJSON any
		if mod != nil {
			modJSON = ISO(*mod)
		}
		if days != nil {
			dayJSON = *days
		}
		rows = append(rows, map[string]any{"domain": domain, "count": len(records), "lastUpdated": modJSON, "oldest_recorded": timeJSON(old), "newest_recorded": timeJSON(new), "rotting": rotting, "rotting_days": dayJSON, "health": health})
		stats = append(stats, DomainStatus{Domain: domain, Count: len(records), LastUpdated: mod, OldestRecorded: old, NewestRecorded: new, Rotting: rotting, RottingDays: days})
	}
	if a.JSONMode {
		a.OK("status", map[string]any{"domains": rows, "governance": gov, "shelf_life": shelf})
	} else {
		fmt.Fprintln(a.Out, FormatStatus(stats, gov))
	}
	return nil
}
func timeJSON(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ISO(*t)
}
