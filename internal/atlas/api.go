package atlas

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

type RecordOptions struct {
	Force bool
	Cwd   string
}
type RecordResult struct {
	Action string `json:"action"`
	Record Record `json:"record"`
}
type SearchOptions struct{ Domain, Type, Tag, Classification, File, Cwd string }
type SearchResult struct {
	Domain  string   `json:"domain"`
	Records []Record `json:"records"`
}
type QueryOptions struct{ Type, Classification, File, Cwd string }
type EditOptions struct{ Cwd string }
type OutcomeOptions struct{ Cwd string }
type AppendOutcomeResult struct {
	Record        Record `json:"record"`
	Outcome       Record `json:"outcome"`
	TotalOutcomes int    `json:"total_outcomes"`
}

func apiCwd(cwd string) string {
	if cwd != "" {
		return cwd
	}
	cwd, _ = os.Getwd()
	return cwd
}
func apiDomain(cwd, domain string) (Config, string, error) {
	cfg, e := ReadConfig(cwd)
	if e != nil {
		return nil, "", e
	}
	if _, ok := Obj(cfg["domains"])[domain]; !ok {
		names := strings.Join(DomainNames(cfg), ", ")
		if names == "" {
			names = "(none)"
		}
		return nil, "", fmt.Errorf("Domain \"%s\" not found in config. Available domains: %s", domain, names)
	}
	path, e := ExpertisePath(cwd, domain)
	return cfg, path, e
}
func RecordExpertise(domain string, record Record, options RecordOptions) (RecordResult, error) {
	cwd := apiCwd(options.Cwd)
	_, path, e := apiDomain(cwd, domain)
	if e != nil {
		return RecordResult{}, e
	}
	reg := GetRegistry()
	result := RecordResult{}
	e = WithFileLock(path, func() error {
		existing, e := ReadRecords(path, reg, false)
		if e != nil {
			return e
		}
		dup := FindDuplicate(existing, record, reg)
		if dup >= 0 && !options.Force {
			if Contains([]string{"pattern", "decision", "reference", "guide"}, Text(record["type"])) {
				existing[dup] = record
				if e = WriteRecords(path, existing); e != nil {
					return e
				}
				result = RecordResult{Action: "updated", Record: record}
				return nil
			}
			result = RecordResult{Action: "skipped", Record: existing[dup]}
			return nil
		}
		if e = AppendRecord(path, record, reg); e != nil {
			return e
		}
		result = RecordResult{Action: "created", Record: record}
		return nil
	})
	return result, e
}
func SearchExpertise(query string, options SearchOptions) ([]SearchResult, error) {
	cwd := apiCwd(options.Cwd)
	cfg, e := ReadConfig(cwd)
	if e != nil {
		return nil, e
	}
	domains := DomainNames(cfg)
	if options.Domain != "" {
		if _, ok := Obj(cfg["domains"])[options.Domain]; !ok {
			return nil, fmt.Errorf("Domain \"%s\" not found in config. Available domains: %s", options.Domain, strings.Join(domains, ", "))
		}
		domains = []string{options.Domain}
	}
	out := []SearchResult{}
	for _, domain := range domains {
		path, e := ExpertisePath(cwd, domain)
		if e != nil {
			return nil, e
		}
		records, e := ReadRecords(path, GetRegistry(), false)
		if e != nil {
			return nil, e
		}
		records = apiFilter(records, options.Type, options.Tag, options.Classification, options.File)
		matches := SearchRecords(records, query, 0)
		if len(matches) > 0 {
			out = append(out, SearchResult{Domain: domain, Records: matches})
		}
	}
	return out, nil
}
func QueryDomain(domain string, options QueryOptions) ([]Record, error) {
	cwd := apiCwd(options.Cwd)
	_, path, e := apiDomain(cwd, domain)
	if e != nil {
		return nil, e
	}
	records, e := ReadRecords(path, GetRegistry(), false)
	if e != nil {
		return nil, e
	}
	return apiFilter(records, options.Type, "", options.Classification, options.File), nil
}
func apiFilter(records []Record, typeName, tag, class, file string) []Record {
	out := []Record{}
	for _, r := range records {
		if typeName != "" && Text(r["type"]) != typeName {
			continue
		}
		if class != "" && Text(r["classification"]) != class {
			continue
		}
		if tag != "" {
			matched := false
			for _, t := range List(r["tags"]) {
				if JSLower(t) == JSLower(tag) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if file != "" {
			matched := false
			for _, f := range List(r["files"]) {
				if strings.Contains(JSLower(f), JSLower(file)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
func apiUpdate(domain, id, cwd string, mutate func(Record) Record) (Record, error) {
	_, path, e := apiDomain(cwd, domain)
	if e != nil {
		return nil, e
	}
	var result Record
	e = WithFileLock(path, func() error {
		records, e := ReadRecords(path, GetRegistry(), false)
		if e != nil {
			return e
		}
		idx, e := ResolveRecordID(records, id)
		if e != nil {
			return e
		}
		copy := Record{}
		for k, v := range records[idx] {
			copy[k] = v
		}
		rememberOrder(map[string]any(copy), ObjectKeys(records[idx]))
		result = mutate(copy)
		records[idx] = result
		return WriteRecords(path, records)
	})
	return result, e
}
func EditRecord(domain, id string, updates Record, options EditOptions) (Record, error) {
	return apiUpdate(domain, id, apiCwd(options.Cwd), func(record Record) Record {
		for _, field := range []string{"classification", "tags", "relates_to", "supersedes", "outcomes"} {
			if value, exists := updates[field]; exists {
				record[field] = value
			}
		}
		fields := []string{}
		switch Text(record["type"]) {
		case "convention":
			fields = []string{"content"}
		case "pattern", "reference":
			fields = []string{"name", "description", "files"}
		case "failure":
			fields = []string{"description", "resolution"}
		case "decision":
			fields = []string{"title", "rationale"}
		case "guide":
			fields = []string{"name", "description"}
		}
		for _, field := range fields {
			if value, exists := updates[field]; exists {
				record[field] = value
			}
		}
		return record
	})
}
func AppendOutcome(domain, id string, outcome Record, options OutcomeOptions) (AppendOutcomeResult, error) {
	o := Record{}
	for k, v := range outcome {
		o[k] = v
	}
	if o["recorded_at"] == nil {
		o["recorded_at"] = NowISO()
	}
	record, e := apiUpdate(domain, id, apiCwd(options.Cwd), func(record Record) Record { record["outcomes"] = append(AnyList(record["outcomes"]), o); return record })
	if e != nil {
		return AppendOutcomeResult{}, e
	}
	return AppendOutcomeResult{Record: record, Outcome: o, TotalOutcomes: len(AnyList(record["outcomes"]))}, nil
}

// RunAPIBridge is a private IPC surface for the unchanged TypeScript library facade.
// It deliberately uses the API's thinner validation boundary, not CLI command handlers.
func RunAPIBridge(request any) (any, error) {
	request = RestoreTransportNumbers(request)
	req := Obj(request)
	apiDefaults.Lock()
	previousDefaults := apiDefaults.config
	apiDefaults.config = nil
	if defaults, ok := req["defaults"].(map[string]any); ok {
		apiDefaults.config = Config(defaults)
	}
	apiDefaults.Unlock()
	defer func() { apiDefaults.Lock(); apiDefaults.config = previousDefaults; apiDefaults.Unlock() }()
	operation := Text(req["operation"])
	args := AnyList(req["args"])
	arg := func(i int) any {
		if i < len(args) {
			return args[i]
		}
		return nil
	}
	opts := func(i int) map[string]any { return Obj(arg(i)) }
	record := func(i int) Record { return Record(Obj(arg(i))) }
	switch operation {
	case "configSchema":
		return contract.ConfigSchema, nil
	case "recordSchema":
		return contract.RecordSchema, nil
	case "defaultConfig":
		return DefaultConfig(), nil
	case "recordExpertise":
		o := opts(2)
		return RecordExpertise(Text(arg(0)), record(1), RecordOptions{Force: Bool(o["force"]), Cwd: Text(o["cwd"])})
	case "recordExpertiseTransport":
		o := opts(2)
		r := record(1)
		result, err := RecordExpertise(Text(arg(0)), r, RecordOptions{Force: Bool(o["force"]), Cwd: Text(o["cwd"])})
		return mutationTransport(result, r, err), nil
	case "searchExpertise":
		o := opts(1)
		return SearchExpertise(Text(arg(0)), SearchOptions{Domain: Text(o["domain"]), Type: Text(o["type"]), Tag: Text(o["tag"]), Classification: Text(o["classification"]), File: Text(o["file"]), Cwd: Text(o["cwd"])})
	case "queryDomain":
		o := opts(1)
		return QueryDomain(Text(arg(0)), QueryOptions{Type: Text(o["type"]), Classification: Text(o["classification"]), File: Text(o["file"]), Cwd: Text(o["cwd"])})
	case "editRecord":
		return EditRecord(Text(arg(0)), Text(arg(1)), record(2), EditOptions{Cwd: Text(opts(3)["cwd"])})
	case "appendOutcome":
		return AppendOutcome(Text(arg(0)), Text(arg(1)), record(2), OutcomeOptions{Cwd: Text(opts(3)["cwd"])})
	case "readConfig":
		return ReadConfig(apiCwd(Text(arg(0))))
	case "getExpertisePath":
		return ExpertisePath(apiCwd(Text(arg(1))), Text(arg(0)))
	case "appendRecord":
		r := record(1)
		err := AppendRecord(Text(arg(0)), r, GetRegistry())
		return r, err
	case "appendRecordTransport":
		r := record(1)
		err := AppendRecord(Text(arg(0)), r, GetRegistry())
		return mutationTransport(nil, r, err), nil
	case "readExpertiseFile":
		return ReadRecords(Text(arg(0)), GetRegistry(), Bool(opts(1)["allowUnknownTypes"]))
	case "writeExpertiseFile":
		r := RecordList(arg(1))
		err := WriteRecords(Text(arg(0)), r)
		return r, err
	case "writeExpertiseFileTransport":
		r := RecordList(arg(1))
		err := WriteRecords(Text(arg(0)), r)
		return mutationTransport(nil, r, err), nil
	case "findDuplicate":
		records := RecordList(arg(0))
		idx := FindDuplicate(records, record(1), GetRegistry())
		if idx < 0 {
			return nil, nil
		}
		return map[string]any{"index": idx, "record": records[idx]}, nil
	case "generateRecordId":
		r := record(0)
		if GetRegistry().Get(Text(r["type"])) == nil {
			return nil, fmt.Errorf("Unknown record type: %s", Text(r["type"]))
		}
		return GenerateRecordID(r, GetRegistry()), nil
	case "searchRecords":
		return SearchRecords(RecordList(arg(0)), Text(arg(1)), Number(opts(2)["boostFactor"])), nil
	case "searchRecordIndexes":
		records := RecordList(arg(0))
		return recordIndexes(records, SearchRecords(records, Text(arg(1)), Number(opts(2)["boostFactor"]))), nil
	case "computeConfirmationScore":
		return ConfirmationScore(record(0)), nil
	case "getSuccessCount":
		return SuccessCount(record(0)), nil
	case "getFailureCount":
		return FailureCount(record(0)), nil
	case "getTotalApplications":
		return TotalApplications(record(0)), nil
	case "getSuccessRate":
		return SuccessRate(record(0)), nil
	case "applyConfirmationBoost":
		factor := .1
		if arg(2) != nil {
			factor = Number(arg(2))
		}
		return ApplyConfirmationBoost(Number(arg(0)), record(1), factor), nil
	case "sortByConfirmationScore":
		return SortByConfirmationScore(RecordList(arg(0))), nil
	case "sortByConfirmationScoreIndexes":
		records := RecordList(arg(0))
		return recordIndexes(records, SortByConfirmationScore(records)), nil
	}
	return nil, fmt.Errorf("Unknown Atlas library operation: %s", operation)
}

func mutationTransport(result, mutation any, err error) map[string]any {
	response := map[string]any{"result": result, "mutation": mutation}
	if err != nil {
		response["error"] = err.Error()
	}
	return response
}

// Return original argument positions so IPC facades can retain caller object identity.
func recordIndexes(records, selected []Record) []int {
	indexes := make([]int, 0, len(selected))
	for _, chosen := range selected {
		for i, record := range records {
			if reflect.ValueOf(chosen).Pointer() == reflect.ValueOf(record).Pointer() {
				indexes = append(indexes, i)
				break
			}
		}
	}
	return indexes
}
