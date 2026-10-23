// Package atlas exposes Luuna Atlas's native Go library. The command-line tool
// and compatibility transports use the same storage and ranking implementation.
package atlas

import core "github.com/brodynelly/luuna-atlas/internal/atlas"

type Record = core.Record
type Config = core.Config
type RecordOptions = core.RecordOptions
type RecordResult = core.RecordResult
type SearchOptions = core.SearchOptions
type SearchResult = core.SearchResult
type QueryOptions = core.QueryOptions
type EditOptions = core.EditOptions
type OutcomeOptions = core.OutcomeOptions
type AppendOutcomeResult = core.AppendOutcomeResult

func RecordExpertise(domain string, record Record, options RecordOptions) (RecordResult, error) {
	return core.RecordExpertise(domain, record, options)
}
func SearchExpertise(query string, options SearchOptions) ([]SearchResult, error) {
	return core.SearchExpertise(query, options)
}
func QueryDomain(domain string, options QueryOptions) ([]Record, error) {
	return core.QueryDomain(domain, options)
}
func EditRecord(domain, id string, updates Record, options EditOptions) (Record, error) {
	return core.EditRecord(domain, id, updates, options)
}
func AppendOutcome(domain, id string, outcome Record, options OutcomeOptions) (AppendOutcomeResult, error) {
	return core.AppendOutcome(domain, id, outcome, options)
}
func ReadConfig(cwd string) (Config, error)            { return core.ReadConfig(cwd) }
func DefaultConfig() Config                            { return core.DefaultConfig() }
func ExpertisePath(cwd, domain string) (string, error) { return core.ExpertisePath(cwd, domain) }
func ReadExpertiseFile(path string, allowUnknownTypes bool) ([]Record, error) {
	return core.ReadRecords(path, core.GetRegistry(), allowUnknownTypes)
}
func AppendRecord(path string, record Record) error {
	return core.AppendRecord(path, record, core.GetRegistry())
}
func WriteExpertiseFile(path string, records []Record) error { return core.WriteRecords(path, records) }
func GenerateRecordID(record Record) string                  { return core.GenerateRecordID(record, core.GetRegistry()) }
func FindDuplicate(records []Record, candidate Record) int {
	return core.FindDuplicate(records, candidate, core.GetRegistry())
}
func SearchRecords(records []Record, query string, boostFactor float64) []Record {
	return core.SearchRecords(records, query, boostFactor)
}
func ComputeConfirmationScore(record Record) float64 { return core.ConfirmationScore(record) }
func GetSuccessCount(record Record) int              { return core.SuccessCount(record) }
func GetFailureCount(record Record) int              { return core.FailureCount(record) }
func GetTotalApplications(record Record) int         { return core.TotalApplications(record) }
func GetSuccessRate(record Record) float64           { return core.SuccessRate(record) }
func ApplyConfirmationBoost(baseScore float64, record Record, boostFactor float64) float64 {
	return core.ApplyConfirmationBoost(baseScore, record, boostFactor)
}
func SortByConfirmationScore(records []Record) []Record { return core.SortByConfirmationScore(records) }
