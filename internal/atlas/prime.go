package atlas

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

type loadedPrimeDomain struct {
	Domain  string
	Records []Record
	Updated *time.Time
}

func primeTokens(s string) int { return (len(utf16.Encode([]rune(s))) + 3) / 4 }
func primeEstimate(r Record, registry *Registry) string {
	if registry.Types[Text(r["type"])] == nil {
		return "[" + Text(r["type"]) + "]"
	}
	return FormatRecordIndex(r, registry, false)
}
func primeTrust(r Record, cfg Config) float64 {
	weights := Obj(Obj(cfg["prime"])["tier_weights"])
	base := map[string]float64{"foundational": 50, "tactical": 20, "observational": 10}
	class := Text(r["classification"])
	return ConfirmationScore(r)*NumberDefault(weights["star"], 100) + NumberDefault(weights[class], base[class])
}
func primeSort(records []Record, cfg Config) []Record {
	out := append([]Record{}, records...)
	sort.SliceStable(out, func(i, j int) bool {
		af, bf := Text(out[i]["type"]) == "failure", Text(out[j]["type"]) == "failure"
		if af != bf {
			return af
		}
		return primeTrust(out[i], cfg) > primeTrust(out[j], cfg)
	})
	return out
}
func primeBudget(loaded []loadedPrimeDomain, budget int, registry *Registry, files []string, stale map[string]bool) ([]loadedPrimeDomain, int, int) {
	type candidate struct {
		domain, index int
		key           [5]float64
		cost          int
		pinned        bool
	}
	all := []candidate{}
	for di, d := range loaded {
		for ri, r := range d.Records {
			failure := Text(r["type"]) == "failure"
			anchored := files != nil && MatchesFileAnchors(r, files)
			tier := 3.
			if failure {
				tier = 2
			}
			if anchored {
				tier = 1
				if failure {
					tier = 0
				}
			}
			isStale := 0.
			if stale[Text(r["id"])] {
				isStale = 1
			}
			class, classified := map[string]float64{"foundational": 0, "tactical": 1, "observational": 2}[Text(r["classification"])]
			if !classified {
				class = -1
			}
			all = append(all, candidate{di, ri, [5]float64{tier, isStale, -ConfirmationScore(r), class, -float64(ParseDate(Text(r["recorded_at"])).UnixMilli())}, primeTokens(primeEstimate(r, registry)), tier == 0})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		for k := range all[i].key {
			if all[i].key[k] != all[j].key[k] {
				return all[i].key[k] < all[j].key[k]
			}
		}
		return false
	})
	keep := map[[2]int]bool{}
	used := 0
	for _, v := range all {
		if v.pinned || used+v.cost <= budget {
			keep[[2]int{v.domain, v.index}] = true
			used += v.cost
		}
	}
	out := []loadedPrimeDomain{}
	dropped, droppedDomains := 0, 0
	for di, d := range loaded {
		records := []Record{}
		domainDrops := 0
		for ri, r := range d.Records {
			if keep[[2]int{di, ri}] {
				records = append(records, r)
			} else {
				dropped++
				domainDrops++
			}
		}
		if domainDrops > 0 {
			droppedDomains++
		}
		if len(records) > 0 {
			out = append(out, loadedPrimeDomain{d.Domain, records, d.Updated})
		}
	}
	return out, dropped, droppedDomains
}
func primeAnnotation(r Record, ctx *ActiveContext) string {
	if ctx != nil {
		for _, f := range List(r["files"]) {
			if FileMatchesAny(f, ctx.ChangedFiles) {
				return "why: file match (" + f + ")"
			}
		}
		for _, d := range List(r["dir_anchors"]) {
			for _, f := range ctx.ChangedFiles {
				if FileLivesUnderDir(f, d) {
					return "why: file match (" + d + ")"
				}
			}
		}
		for _, tracker := range []string{"missions", "gh", "linear", "bead"} {
			if active := ctx.Trackers[tracker]; active != "" && active == Text(Obj(r["evidence"])[tracker]) {
				return "why: in-progress " + tracker + ":" + active
			}
		}
	}
	score := ConfirmationScore(r)
	if score > 0 {
		return fmt.Sprintf("why: ★%s confirmations", Text(score))
	}
	date := ParseDate(Text(r["recorded_at"]))
	if !date.IsZero() {
		days := int(math.Floor(time.Since(date).Hours() / 24))
		if days < 7 {
			if days <= 0 {
				return "why: recorded today"
			}
			return fmt.Sprintf("why: recorded %dd ago", days)
		}
	}
	return "why: applies broadly (no anchors)"
}
func (a *App) primeOutput(output string, i Invocation) error {
	if path := i.String("export"); path != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(a.Cwd, path)
		}
		if err := os.WriteFile(path, []byte(output+"\n"), 0666); err != nil {
			return err
		}
		if !a.JSONMode {
			a.Human("✓ Exported to " + i.String("export"))
		}
	} else if output != "" {
		fmt.Fprintln(a.Out, output)
	}
	return nil
}

func (a *App) runPrime(i Invocation) error {
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	format := "compact"
	if a.Format != "" {
		format = a.Format
	} else if i.Bool("full") {
		format = "markdown"
	}
	if i.Bool("manifest") && i.Bool("full") {
		return fmt.Errorf("Cannot combine --manifest with --full.")
	}
	if i.Bool("dry-run") && i.Bool("manifest") {
		return fmt.Errorf("Cannot combine --dry-run with --manifest. Manifest mode lists domains, not records; --dry-run previews which records would be primed.")
	}
	requested := Unique(append(append([]string{}, i.Positionals...), i.Strings("domain")...))
	excluded := i.Strings("exclude-domain")
	scoped := len(requested) > 0 || len(excluded) > 0 || i.Bool("context") || len(i.Strings("files")) > 0
	if i.Bool("manifest") && scoped {
		return fmt.Errorf("--manifest cannot be combined with scoping arguments. Manifest mode lists available domains; use `luuna-atlas prime <domain>` or `luuna-atlas prime --files <path>` to load records.")
	}
	for _, d := range requested {
		if !a.knownDomain(cfg, d, true) {
			return nil
		}
	}
	for _, d := range excluded {
		if _, ok := Obj(cfg["domains"])[d]; !ok {
			a.Error("prime", fmt.Sprintf("Excluded domain %q not found in config. Available domains: %s", d, strings.Join(DomainNames(cfg), ", ")))
			if !a.JSONMode {
				fmt.Fprintf(a.Err, "Hint: Run `luuna-atlas add %s` to create this domain, or check .luuna/atlas/config.yaml\n", d)
			}
			return nil
		}
	}
	domains := DomainNames(cfg)
	if len(requested) > 0 {
		domains = requested
	}
	target := []string{}
	for _, d := range domains {
		if !Contains(excluded, d) {
			target = append(target, d)
		}
	}
	var files []string
	if i.Bool("context") {
		if !IsGitRepo(a.Cwd) {
			return fmt.Errorf("Not in a git repository. --context requires git.")
		}
		files = GetChangedFiles(a.Cwd, "HEAD~1")
		if len(files) == 0 {
			if a.JSONMode {
				a.Error("prime", "No changed files found. Nothing to filter by.")
				a.ExitCode = 0
			} else {
				fmt.Fprintln(a.Out, "No changed files found. Nothing to filter by.")
			}
			return nil
		}
	} else if len(i.Strings("files")) > 0 {
		files = i.Strings("files")
	}
	budget := 4000
	if s := i.String("budget"); s != "" {
		budget, err = strictPositive(s)
		if err != nil {
			return fmt.Errorf("--budget must be a positive integer (got %q).", s)
		}
	}
	budgetEnabled := !a.JSONMode && (!i.Has("limit") || i.Bool("limit"))
	mode := Text(Obj(cfg["prime"])["default_mode"])
	if i.Bool("manifest") {
		mode = "manifest"
	}
	if i.Bool("full") {
		mode = "full"
	}
	manifest := mode == "manifest" && !scoped && !i.Bool("dry-run") && !i.Bool("records-only")
	loaded := []loadedPrimeDomain{}
	domainRecords := []DomainRecords{}
	for _, domain := range target {
		records, e := a.readDomain(domain)
		if e != nil {
			return e
		}
		path, _ := ExpertisePath(a.Cwd, domain)
		loaded = append(loaded, loadedPrimeDomain{domain, records, fileModTime(path)})
		domainRecords = append(domainRecords, DomainRecords{domain, records})
	}
	superseded, _ := CollectSupersededIDs(domainRecords)
	for n := range loaded {
		live := []Record{}
		for _, r := range loaded[n].Records {
			if !superseded[Text(r["id"])] {
				live = append(live, r)
			}
		}
		loaded[n].Records = live
	}
	var active *ActiveContext
	if !manifest && !scoped && !a.JSONMode && !i.Bool("all") && files == nil && IsGitRepo(a.Cwd) {
		work := ResolveActiveWork(a.Cwd, nil)
		ctx := ActiveContext{ChangedFiles: GetActiveFiles(a.Cwd), Trackers: map[string]string{"missions": work.Missions, "gh": work.GH, "linear": work.Linear, "bead": work.Bead}}
		signal := len(ctx.ChangedFiles) > 0
		for _, v := range ctx.Trackers {
			if v != "" {
				signal = true
			}
		}
		if signal {
			active = &ctx
		}
	}
	if files != nil {
		filtered := []loadedPrimeDomain{}
		for _, d := range loaded {
			d.Records = FilterByContext(d.Records, files)
			if a.JSONMode || len(d.Records) > 0 {
				filtered = append(filtered, d)
			}
		}
		loaded = filtered
	} else if active != nil && !manifest {
		total, kept := 0, 0
		for n := range loaded {
			total += len(loaded[n].Records)
			loaded[n].Records = FilterByActiveContext(loaded[n].Records, *active)
			kept += len(loaded[n].Records)
		}
		sources := []string{}
		if len(active.ChangedFiles) > 0 {
			sources = append(sources, "git status")
		}
		trackers := []string{}
		for _, t := range []string{"missions", "gh", "linear", "bead"} {
			if v := active.Trackers[t]; v != "" {
				trackers = append(trackers, t+":"+v)
			}
		}
		if len(trackers) > 0 {
			sources = append(sources, "active-work ("+strings.Join(trackers, ", ")+")")
		}
		fmt.Fprintf(a.Err, "prime: scoped to %d of %d records based on %s; run with --all for the full corpus\n", kept, total, strings.Join(sources, " + "))
	}
	contractBlock := ""
	if !a.JSONMode && !i.Bool("dry-run") && !i.Bool("records-only") {
		contractBlock = FormatProjectContract(cfg, a.Registry, format)
	}
	if manifest {
		manifestDomains := []ManifestDomain{}
		for _, d := range loaded {
			manifestDomains = append(manifestDomains, ManifestDomain{Domain: d.Domain, Count: len(d.Records), LastUpdated: d.Updated, TypeCounts: ComputeTypeCounts(d.Records)})
		}
		output := ""
		if a.JSONMode {
			output = JSONPretty(BuildManifestPayload(manifestDomains, Obj(cfg["governance"])))
		} else {
			output = FormatManifest(manifestDomains, Obj(cfg["governance"]), format)
			if format != "plain" {
				if footer := SessionEndReminder(format, cfg); footer != "" {
					output += "\n\n" + footer
				}
			}
		}
		if contractBlock != "" {
			output = contractBlock + "\n\n" + output
		}
		return a.primeOutput(output, i)
	}
	all := []Record{}
	for n := range loaded {
		loaded[n].Records = primeSort(loaded[n].Records, cfg)
		all = append(all, loaded[n].Records...)
	}
	annotationContext := active
	if files != nil {
		annotationContext = &ActiveContext{ChangedFiles: files, Trackers: map[string]string{}}
	}
	stale := map[string]bool{}
	if !a.JSONMode {
		for _, entry := range FindStaleRecords(all, a.Cwd, time.Now(), Obj(Obj(cfg["classification_defaults"])["shelf_life"])) {
			for _, reason := range entry.Reasons {
				if reason.Kind != "shelf_life" {
					stale[Text(entry.Record["id"])] = true
				}
			}
		}
	}
	if i.Bool("dry-run") {
		selected := loaded
		if budgetEnabled {
			selected, _, _ = primeBudget(loaded, budget, a.Registry, files, stale)
		}
		rows := []any{}
		tokens := 0
		for _, d := range selected {
			for _, r := range d.Records {
				cost := primeTokens(primeEstimate(r, a.Registry))
				rows = append(rows, map[string]any{"id": Text(r["id"]), "type": r["type"], "domain": d.Domain, "tokens": cost})
				tokens += cost
			}
		}
		var used, total any
		if budgetEnabled {
			used = float64(tokens) / float64(budget)
			total = budget
		}
		return a.primeOutput(JSONPretty(map[string]any{"wouldPrime": rows, "totalTokens": tokens, "budgetUsed": used, "budgetTotal": total}), i)
	}
	hookDomains := []any{}
	for _, d := range loaded {
		hookDomains = append(hookDomains, map[string]any{"domain": d.Domain, "records": d.Records})
	}
	hook, err := RunHooks(a.Cwd, cfg, "pre-prime", map[string]any{"domains": hookDomains}, a.Err)
	if err != nil {
		return err
	}
	if hook.Blocked {
		reason := hook.BlockReason
		if reason == "" {
			reason = "pre-prime hook blocked output"
		}
		return fmt.Errorf("%s", reason)
	}
	if !a.JSONMode {
		for _, warning := range hook.Warnings {
			a.Warning(warning)
		}
	}
	if hook.RanAny {
		mutated := map[string][]Record{}
		for _, d := range AnyList(Obj(hook.Payload)["domains"]) {
			raw := Obj(d)
			if domain, ok := raw["domain"].(string); ok {
				switch records := raw["records"].(type) {
				case []any:
					mutated[domain] = RecordList(records)
				case []Record:
					mutated[domain] = records
				}
			}
		}
		for n := range loaded {
			if records, ok := mutated[loaded[n].Domain]; ok {
				loaded[n].Records = records
			}
		}
	}
	for n := range loaded {
		loaded[n].Records = primeSort(loaded[n].Records, cfg)
	}
	if a.JSONMode {
		rows := []any{}
		for _, d := range loaded {
			if files == nil || len(d.Records) > 0 {
				rows = append(rows, map[string]any{"domain": d.Domain, "entry_count": len(d.Records), "records": d.Records})
			}
		}
		return a.primeOutput(JSONPretty(map[string]any{"type": "expertise", "domains": rows}), i)
	}
	dropped, droppedDomains := 0, 0
	if budgetEnabled {
		loaded, dropped, droppedDomains = primeBudget(loaded, budget, a.Registry, files, stale)
	}
	sections := []string{}
	for _, d := range loaded {
		annotations := map[string]string{}
		for _, r := range d.Records {
			if id := Text(r["id"]); id != "" {
				annotations[id] = primeAnnotation(r, annotationContext)
				if stale[id] {
					annotations[id] += "; stale?"
				}
			}
		}
		sections = append(sections, FormatDomain(d.Domain, d.Records, d.Updated, a.Registry, FormatOptions{Format: format, Full: i.Bool("full") || a.Verbose, Annotations: annotations, StaleIDs: stale}))
	}
	output := ""
	if i.Bool("records-only") {
		output = strings.Join(sections, "\n\n")
	} else {
		output = FormatPrime(sections, format)
	}
	if dropped > 0 {
		suffix, ds := "s", "s"
		if dropped == 1 {
			suffix = ""
		}
		if droppedDomains == 1 {
			ds = ""
		}
		domainPart := ""
		if droppedDomains > 0 {
			domainPart = fmt.Sprintf(" across %d domain%s", droppedDomains, ds)
		}
		output += fmt.Sprintf("\n\n... and %d more record%s%s (use --budget <n> to show more)", dropped, suffix, domainPart)
	}
	if format != "plain" && !i.Bool("records-only") {
		if reminder := SessionEndReminder(format, cfg); reminder != "" {
			output += "\n\n" + reminder
		}
	}
	if contractBlock != "" {
		output = contractBlock + "\n\n" + output
	}
	return a.primeOutput(output, i)
}

func (a *App) runStale(i Invocation) error {
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return err
	}
	domains := DomainNames(cfg)
	if len(i.Positionals) > 0 {
		domains = i.Positionals
	}
	loaded := []DomainRecords{}
	for _, d := range domains {
		if _, ok := Obj(cfg["domains"])[d]; !ok {
			return fmt.Errorf("Domain %q not found in config.", d)
		}
		r, e := a.readDomain(d)
		if e != nil {
			return e
		}
		loaded = append(loaded, DomainRecords{d, r})
	}
	superseded, _ := CollectSupersededIDs(loaded)
	all := []Record{}
	for _, d := range loaded {
		for _, r := range d.Records {
			if !superseded[Text(r["id"])] {
				all = append(all, r)
			}
		}
	}
	reasons := map[string][]StaleReason{}
	for _, entry := range FindStaleRecords(all, a.Cwd, time.Now(), Obj(Obj(cfg["classification_defaults"])["shelf_life"])) {
		reasons[Text(entry.Record["id"])] = entry.Reasons
	}
	rows := []any{}
	for _, d := range loaded {
		for _, r := range d.Records {
			id := Text(r["id"])
			if rs := reasons[id]; id != "" && len(rs) > 0 && !superseded[id] {
				rows = append(rows, map[string]any{"id": id, "domain": d.Domain, "type": r["type"], "reasons": rs})
			}
		}
	}
	if a.JSONMode {
		a.OK("stale", map[string]any{"records": rows})
		return nil
	}
	if len(rows) == 0 {
		a.Human("No stale records.")
		return nil
	}
	for _, row := range rows {
		r := Obj(row)
		parts := []string{}
		for _, reason := range r["reasons"].([]StaleReason) {
			parts = append(parts, reason.Detail)
		}
		fmt.Fprintf(a.Out, "%s (%s, %s) %s\n", Text(r["id"]), Text(r["domain"]), Text(r["type"]), strings.Join(parts, "; "))
	}
	fmt.Fprintf(a.Out, "\n%d stale record(s). Check with `luuna-atlas show <id>`; refresh via `luuna-atlas edit`, confirm via `luuna-atlas outcome`, or replace with `luuna-atlas record --supersedes <id>`.\n", len(rows))
	return nil
}
