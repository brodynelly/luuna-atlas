package atlas

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode"
)

func ReadRecords(path string, registry *Registry, allowUnknown bool) ([]Record, error) {
	data, e := os.ReadFile(path)
	if e != nil {
		return []Record{}, nil
	}
	if registry == nil {
		registry = GetRegistry()
	}
	out := []Record{}
	for idx, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		value, e := ParseJSON([]byte(line))
		if e != nil {
			preview := trimmed
			if formatLen(preview) > 80 {
				preview = formatSlice(preview, 77) + "..."
			}
			return nil, fmt.Errorf("Malformed JSONL at %s:%d: %s. Line: %s", path, idx+1, writeJSONParseError(line), preview)
		}
		switch value.(type) {
		case nil, string, bool, float64, json.Number:
			return nil, fmt.Errorf("raw is not an Object. (evaluating '\"outcome\" in raw')")
		}
		raw := RecordFromValue(value)
		if legacy, ok := raw["outcome"]; ok && legacy != nil {
			if _, exists := raw["outcomes"]; !exists {
				o := Obj(legacy)
				normalized := map[string]any{}
				if v, exists := o["status"]; exists {
					normalized["status"] = v
				}
				for _, k := range []string{"duration", "test_results", "agent"} {
					if v, exists := o[k]; exists {
						normalized[k] = v
					}
				}
				raw["outcomes"] = []any{normalized}
				delete(raw, "outcome")
			}
		}
		typeName, _ := raw["type"].(string)
		def := registry.Get(typeName)
		if typeName != "" && def == nil && !allowUnknown {
			idPart := ""
			if id, ok := raw["id"].(string); ok {
				idPart = " (id=" + id + ")"
			}
			return nil, fmt.Errorf("Unknown record type \"%s\" at %s:%d%s. Register it under custom_types in config.yaml, remove the record, or pass --allow-unknown-types to bypass.", typeName, path, idx+1, idPart)
		}
		if def != nil {
			ApplyAliases(raw, def.Aliases)
		}
		out = append(out, raw)
	}
	return out, nil
}
func GenerateRecordID(record Record, registry *Registry) string {
	if registry == nil {
		registry = GetRegistry()
	}
	def := registry.Get(Text(record["type"]))
	if def == nil {
		return ""
	}
	key := Text(record["type"]) + ":" + Text(record[def.IDKey])
	sum := sha256.Sum256([]byte(key))
	return "mx-" + hex.EncodeToString(sum[:])[:6]
}
func AppendRecord(path string, record Record, registry *Registry) error {
	if !Bool(record["id"]) {
		id := GenerateRecordID(record, registry)
		if id == "" {
			return fmt.Errorf("Unknown record type: %s", Text(record["type"]))
		}
		record["id"] = id
	}
	b, e := MarshalJSON(record)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = f.Write(append(b, '\n'))
	return e
}
func WriteRecords(path string, records []Record) error {
	lines := []string{}
	for _, r := range records {
		if !Bool(r["id"]) {
			id := GenerateRecordID(r, nil)
			if id == "" {
				return fmt.Errorf("Unknown record type: %s", Text(r["type"]))
			}
			r["id"] = id
		}
		b, e := MarshalJSON(r)
		if e != nil {
			return e
		}
		lines = append(lines, string(b))
	}
	body := strings.Join(lines, "\n")
	if len(records) > 0 {
		body += "\n"
	}
	return WriteFileAtomic(path, []byte(body), 0666)
}
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	random := make([]byte, 8)
	if _, e := rand.Read(random); e != nil {
		return e
	}
	tmp := path + ".tmp." + hex.EncodeToString(random)
	if e := os.WriteFile(tmp, data, mode); e != nil {
		return e
	}
	if e := RenameReplacing(tmp, path); e != nil {
		_ = os.Remove(tmp)
		return e
	}
	return nil
}
func RenameReplacing(from, to string) error {
	for attempt := 1; ; attempt++ {
		e := os.Rename(from, to)
		if e == nil {
			return nil
		}
		if runtime.GOOS != "windows" || attempt >= 20 {
			return e
		}
		if !os.IsPermission(e) && !strings.Contains(JSLower(e.Error()), "used by another process") && !strings.Contains(JSLower(e.Error()), "access is denied") {
			return e
		}
		delay := 5 * time.Millisecond * time.Duration(1<<attempt)
		if delay > 100*time.Millisecond {
			delay = 100 * time.Millisecond
		}
		time.Sleep(delay)
	}
}
func WithFileLock(path string, fn func() error) error {
	lock := path + ".lock"
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, e := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
		if e == nil {
			_ = f.Close()
			break
		}
		if !os.IsExist(e) {
			return e
		}
		if s, e := os.Lstat(lock); e == nil && time.Since(s.ModTime()) > 30*time.Second {
			_ = os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Timed out waiting for lock on %s. If no other luuna-atlas process is running, delete the lock file manually.", lock)
		}
		time.Sleep(50 * time.Millisecond)
	}
	defer os.Remove(lock)
	return fn()
}
func NormalizeText(text string) string {
	var b strings.Builder
	separator := false
	for _, r := range JSLower(text) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			if separator && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
	}
	return b.String()
}
func RecordBodyText(record Record, registry *Registry) string {
	_, fields, order := ExtractRecordText(record)
	parts := []string{}
	for _, f := range order {
		if f != "tags" && f != "files" {
			parts = append(parts, fields[f])
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if registry == nil {
		registry = GetRegistry()
	}
	def := registry.Get(Text(record["type"]))
	if def == nil {
		return ""
	}
	for _, f := range append(append([]string{}, def.Required...), def.Optional...) {
		if s, ok := record[f].(string); ok {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}
func FindDuplicate(records []Record, candidate Record, registry *Registry) int {
	if registry == nil {
		registry = GetRegistry()
	}
	def := registry.Get(Text(candidate["type"]))
	if def == nil {
		return -1
	}
	value := func(r Record) string {
		if def.DedupKey == "content_hash" {
			return NormalizeText(RecordBodyText(r, registry))
		}
		s, ok := r[def.DedupKey].(string)
		if !ok {
			return ""
		}
		return NormalizeText(s)
	}
	candidateValue := value(candidate)
	if candidateValue == "" {
		return -1
	}
	for i, r := range records {
		if r["type"] == candidate["type"] && value(r) == candidateValue {
			return i
		}
	}
	return -1
}
func ResolveRecordID(records []Record, id string) (int, error) {
	hash := strings.TrimPrefix(id, "mx-")
	for idx, r := range records {
		if Text(r["id"]) == "mx-"+hash {
			return idx, nil
		}
	}
	matches := []int{}
	ids := []string{}
	for idx, r := range records {
		if strings.HasPrefix(Text(r["id"]), "mx-"+hash) {
			matches = append(matches, idx)
			ids = append(ids, Text(r["id"]))
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return -1, fmt.Errorf("Ambiguous identifier \"%s\" matches %d records: %s. Use more characters to disambiguate.", id, len(matches), strings.Join(ids, ", "))
	}
	return -1, fmt.Errorf("Record \"%s\" not found. Run `luuna-atlas query` to see record IDs.", id)
}
func FilterByFile(records []Record, file string) []Record {
	out := []Record{}
	for _, r := range records {
		for _, f := range List(r["files"]) {
			if strings.Contains(JSLower(f), JSLower(file)) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}
func FilterByType(records []Record, typeName string) []Record {
	out := []Record{}
	for _, r := range records {
		if Text(r["type"]) == typeName {
			out = append(out, r)
		}
	}
	return out
}
func FilterByClassification(records []Record, class string) []Record {
	out := []Record{}
	for _, r := range records {
		if Text(r["classification"]) == class {
			out = append(out, r)
		}
	}
	return out
}
func FileModTime(path string) *time.Time {
	st, e := os.Stat(path)
	if e != nil {
		return nil
	}
	t := st.ModTime()
	return &t
}
