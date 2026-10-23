package atlas

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

var tokenizeRE = regexp.MustCompile(`[^\w\s-]`)

func Tokenize(text string) []string {
	return strings.Fields(tokenizeRE.ReplaceAllString(JSLower(text), " "))
}
func ExtractRecordText(r Record) (string, map[string]string, []string) {
	fields := map[string]string{}
	order := []string{}
	parts := []string{}
	add := func(name string) {
		value := r[name]
		s := ""
		if raw, ok := value.(string); ok {
			s = raw
		} else if value != nil {
			s = strings.Join(List(value), " ")
		}
		if strings.TrimSpace(s) != "" {
			fields[name] = s
			order = append(order, name)
			parts = append(parts, s)
		}
	}
	switch Text(r["type"]) {
	case "pattern", "reference":
		add("name")
		add("description")
		add("files")
	case "convention":
		add("content")
	case "failure":
		add("description")
		add("resolution")
	case "decision":
		add("title")
		add("rationale")
	case "guide":
		add("name")
		add("description")
	}
	add("tags")
	return strings.Join(parts, " "), fields, order
}
func SearchBM25(records []Record, query string) []BM25Result {
	out := []BM25Result{}
	queryTokens := Tokenize(query)
	if len(records) == 0 || len(queryTokens) == 0 {
		return out
	}
	type document struct {
		record Record
		tokens []string
		fields map[string]string
		order  []string
	}
	docs := []document{}
	df := map[string]float64{}
	total := 0
	for _, r := range records {
		all, fields, order := ExtractRecordText(r)
		tokens := Tokenize(all)
		docs = append(docs, document{r, tokens, fields, order})
		total += len(tokens)
		for _, t := range Unique(tokens) {
			df[t]++
		}
	}
	avg := float64(total) / float64(len(docs))
	if avg == 0 {
		return out
	}
	idf := map[string]float64{}
	for term, n := range df {
		idf[term] = math.Log((float64(len(docs))-n+.5)/(n+.5) + 1)
	}
	for _, doc := range docs {
		freq := map[string]float64{}
		for _, term := range doc.tokens {
			freq[term]++
		}
		score := 0.0
		for _, q := range queryTokens {
			tf := freq[q]
			score += idf[q] * (tf * 2.5) / (tf + 1.5*(1-.75+.75*(float64(len(doc.tokens))/avg)))
		}
		if score > 0 {
			matched := []string{}
			for _, field := range doc.order {
				tokens := Tokenize(doc.fields[field])
				for _, q := range queryTokens {
					if Contains(tokens, q) {
						matched = append(matched, field)
						break
					}
				}
			}
			out = append(out, BM25Result{Record: doc.record, Score: score, MatchedFields: matched})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
func SearchRecords(records []Record, query string, boostFactor float64) []Record {
	hits := SearchBM25(records, query)
	if boostFactor > 0 {
		for i := range hits {
			hits[i].Score = ApplyConfirmationBoost(hits[i].Score, hits[i].Record, boostFactor)
		}
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	}
	out := []Record{}
	for _, hit := range hits {
		out = append(out, hit.Record)
	}
	return out
}
func SuccessCount(r Record) int {
	n := 0
	for _, o := range RecordList(r["outcomes"]) {
		if Text(o["status"]) == "success" {
			n++
		}
	}
	return n
}
func FailureCount(r Record) int {
	n := 0
	for _, o := range RecordList(r["outcomes"]) {
		if Text(o["status"]) == "failure" {
			n++
		}
	}
	return n
}
func TotalApplications(r Record) int { return len(AnyList(r["outcomes"])) }
func ConfirmationScore(r Record) float64 {
	score := 0.0
	for _, o := range RecordList(r["outcomes"]) {
		switch Text(o["status"]) {
		case "success":
			score++
		case "partial":
			score += .5
		}
	}
	return score
}
func SuccessRate(r Record) float64 {
	n := TotalApplications(r)
	if n == 0 {
		return 0
	}
	return ConfirmationScore(r) / float64(n)
}
func ApplyConfirmationBoost(score float64, r Record, factor float64) float64 {
	confirmation := ConfirmationScore(r)
	if confirmation == 0 {
		return score
	}
	return score * (1 + factor*confirmation)
}
func SortByConfirmationScore(records []Record) []Record {
	out := append([]Record{}, records...)
	sort.SliceStable(out, func(i, j int) bool { return ConfirmationScore(out[i]) > ConfirmationScore(out[j]) })
	return out
}
func FindNearDuplicates(existing []Record, candidate Record, reg *Registry) []NearDuplicate {
	out := []NearDuplicate{}
	body := RecordBodyText(candidate, reg)
	tokenSet := func(s string) []string {
		out := []string{}
		for _, t := range Tokenize(s) {
			if len(t) >= 3 {
				out = append(out, t)
			}
		}
		return Unique(out)
	}
	tokens := tokenSet(body)
	if len(tokens) == 0 {
		return out
	}
	superseded := List(candidate["supersedes"])
	anchors := qualityAnchors(candidate)
	hits := SearchBM25(existing, body)
	if len(hits) > 10 {
		hits = hits[:10]
	}
	for _, hit := range hits {
		r := hit.Record
		if Contains(superseded, Text(r["id"])) {
			continue
		}
		other := tokenSet(RecordBodyText(r, reg))
		shared := 0
		for _, t := range tokens {
			if Contains(other, t) {
				shared++
			}
		}
		similarity := float64(shared) / float64(len(Unique(append(append([]string{}, tokens...), other...))))
		sharedAnchors := []string{}
		for _, v := range qualityAnchors(r) {
			if Contains(anchors, v) {
				sharedAnchors = append(sharedAnchors, v)
			}
		}
		threshold := .6
		if len(sharedAnchors) > 0 {
			threshold = .45
		}
		if similarity >= threshold {
			out = append(out, NearDuplicate{Record: r, Similarity: similarity, SharedAnchors: sharedAnchors})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Similarity > out[j].Similarity })
	return out
}
func qualityAnchors(r Record) []string {
	out := append(append([]string{}, List(r["files"])...), List(r["dir_anchors"])...)
	if f := Text(Obj(r["evidence"])["file"]); f != "" {
		out = append(out, f)
	}
	return out
}
func HasRuleSignal(content string) bool {
	lower := JSLower(content)
	for _, word := range []string{"because", "must not", "do not", "don't", "avoid", "always ", "never ", "prefer", "required for", "reason:", "rationale", "so that", "otherwise"} {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}
func CheckRecordQuality(r Record, autoCommit bool) []string {
	out := []string{}
	typ := Text(r["type"])
	ev := Obj(r["evidence"])
	grounded := len(qualityAnchors(r)) > 0 || Text(ev["issue"]) != "" || EvidenceMissions(ev) != "" || Text(ev["gh"]) != "" || Text(ev["linear"]) != "" || Text(ev["bead"]) != "" || (Text(ev["commit"]) != "" && !autoCommit)
	if (typ == "convention" || typ == "pattern") && !grounded {
		out = append(out, typ+" has no file/dir anchor or evidence; add --files, --dir-anchor, or an --evidence-* flag so it can be scoped and verified.")
	}
	if typ == "convention" && !HasRuleSignal(Text(r["content"])) {
		out = append(out, `convention reads like a restatement of code (no rule words such as "because", "avoid", "never", "prefer"); state the rule and why, or skip it.`)
	}
	return out
}
