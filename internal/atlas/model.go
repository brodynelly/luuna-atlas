package atlas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Record map[string]any
type Config map[string]any

// JavaScript's locale-independent lowercase uses full/contextual Unicode
// mappings (not Unicode's simple one-rune lowercase table).
func JSLower(text string) string { return cases.Lower(language.Und).String(text) }

// The historical reader accepts JSON arrays as rows even though the schema
// rejects them. Keep their JSON value outside the map so no private field leaks
// into library results, query output, or a subsequent serialization.
var rawRecordValues sync.Map

type rawRecordValue struct {
	record Record
	value  any
}

func RecordFromValue(value any) Record {
	if record, ok := value.(Record); ok {
		return record
	}
	record := Record(Obj(value))
	if _, ok := value.([]any); ok {
		rawRecordValues.Store(reflect.ValueOf(record).Pointer(), rawRecordValue{record, value})
	}
	return record
}

func RecordValue(record Record) any {
	if raw, ok := rawRecordValues.Load(reflect.ValueOf(record).Pointer()); ok {
		return raw.(rawRecordValue).value
	}
	return map[string]any(record)
}

func (record Record) MarshalJSON() ([]byte, error) {
	if record == nil {
		return []byte("null"), nil
	}
	return MarshalJSON(RecordValue(record))
}

type ValidationIssue struct {
	Path    string         `json:"instancePath"`
	Message string         `json:"message"`
	Keyword string         `json:"keyword,omitempty"`
	Params  map[string]any `json:"params,omitempty"`
}
type DomainViolation struct{ Kind, Field, Message string }
type TypeDefinition struct {
	Name                                                        string
	Required, Optional                                          []string
	DedupKey, IDKey, Summary, SectionTitle, Compact, FilesField string
	ExtractsFiles, Disabled, Builtin, SummaryInherited          bool
	Aliases                                                     map[string][]string
	Parent                                                      string
}
type Registry struct {
	Types    map[string]*TypeDefinition
	Order    []string
	Disabled []string
}
type DomainRecords struct {
	Domain  string   `json:"domain"`
	Records []Record `json:"records"`
}
type Anchor struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}
type AnchorValidityResult struct {
	Valid         int      `json:"valid"`
	Total         int      `json:"total"`
	ValidFraction *float64 `json:"validFraction"`
	Broken        []Anchor `json:"broken"`
}
type NearDuplicate struct {
	Record        Record
	Similarity    float64
	SharedAnchors []string
}
type BM25Result struct {
	Record        Record   `json:"record"`
	Score         float64  `json:"score"`
	MatchedFields []string `json:"matchedFields"`
}
type StaleReason struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}
type StaleEntry struct {
	Record  Record
	Reasons []StaleReason
}
type ActiveContext struct {
	ChangedFiles []string
	Trackers     map[string]string
}
type ActiveWorkResult struct {
	Missions, GH, Linear, Bead string
	Warnings                   []string
}

var objectOrder sync.Map

func rememberOrder(m map[string]any, keys []string) {
	if m != nil {
		objectOrder.Store(reflect.ValueOf(m).Pointer(), append([]string{}, keys...))
	}
}
func ObjectKeys(v any) []string {
	m := Obj(v)
	if remembered, ok := objectOrder.Load(reflect.ValueOf(m).Pointer()); ok {
		out := []string{}
		seen := map[string]bool{}
		for _, key := range remembered.([]string) {
			if _, exists := m[key]; exists && !seen[key] {
				out = append(out, key)
				seen[key] = true
			}
		}
		rest := []string{}
		for key := range m {
			if !seen[key] {
				rest = append(rest, key)
			}
		}
		sort.Strings(rest)
		return jsObjectKeyOrder(append(out, rest...))
	}
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return jsObjectKeyOrder(out)
}

// ECMAScript enumerates canonical uint32 array indexes before string keys,
// regardless of the order those properties appeared in YAML or JSON.
func jsObjectKeyOrder(keys []string) []string {
	type indexedKey struct {
		key   string
		index uint64
	}
	indexes := []indexedKey{}
	strings := []string{}
	for _, key := range keys {
		index, err := strconv.ParseUint(key, 10, 32)
		if err == nil && index != 1<<32-1 && strconv.FormatUint(index, 10) == key {
			indexes = append(indexes, indexedKey{key, index})
		} else {
			strings = append(strings, key)
		}
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].index < indexes[j].index })
	out := make([]string, 0, len(keys))
	for _, item := range indexes {
		out = append(out, item.key)
	}
	return append(out, strings...)
}
func Text(v any) string {
	if v == nil {
		return ""
	}
	switch n := v.(type) {
	case string:
		return n
	case bool:
		if n {
			return "true"
		}
		return "false"
	case json.Number:
		return string(n)
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(n), 'f', -1, 32)
	case int:
		return strconv.Itoa(n)
	case []string:
		return strings.Join(n, ",")
	case []any:
		parts := []string{}
		for _, x := range n {
			parts = append(parts, Text(x))
		}
		return strings.Join(parts, ",")
	case map[string]any, Record, Config:
		return "[object Object]"
	}
	return fmt.Sprint(v)
}
func Obj(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case Record:
		return map[string]any(m)
	case Config:
		return map[string]any(m)
	}
	if v != nil {
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
			out := map[string]any{}
			it := rv.MapRange()
			for it.Next() {
				out[it.Key().String()] = it.Value().Interface()
			}
			return out
		}
	}
	return map[string]any{}
}
func List(v any) []string {
	switch a := v.(type) {
	case []string:
		return a
	case []any:
		out := make([]string, 0, len(a))
		for _, x := range a {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return []string{}
}
func AnyList(v any) []any {
	switch a := v.(type) {
	case []any:
		return a
	case []string:
		out := make([]any, len(a))
		for i, x := range a {
			out[i] = x
		}
		return out
	case []Record:
		out := make([]any, len(a))
		for i, x := range a {
			out[i] = x
		}
		return out
	}
	if v != nil {
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			out := make([]any, rv.Len())
			for i := range out {
				out[i] = rv.Index(i).Interface()
			}
			return out
		}
	}
	return []any{}
}
func RecordList(v any) []Record {
	out := []Record{}
	for _, x := range AnyList(v) {
		switch y := x.(type) {
		case Record:
			out = append(out, y)
		case map[string]any:
			out = append(out, Record(y))
		case []any:
			out = append(out, RecordFromValue(y))
		}
	}
	return out
}
func Number(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}
func Bool(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0 && !math.IsNaN(x)
	case int:
		return x != 0
	}
	return true
}
func NowISO() string         { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }
func ISO(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }
func ParseDate(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02", "2006-01-02T15:04:05", "2006-01-02 15:04:05", time.RFC1123} {
		if t, e := time.Parse(layout, s); e == nil {
			return t
		}
	}
	return time.Time{}
}
func ParseTime(s string) time.Time { return ParseDate(s) }
func CloneRecord(r Record) Record {
	b, _ := MarshalJSON(r)
	clone, _ := ParseJSON(b)
	return RecordFromValue(clone)
}
func SplitCSV(value string) []string {
	out := []string{}
	for _, s := range strings.Split(value, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
func Contains(values []string, s string) bool {
	for _, v := range values {
		if v == s {
			return true
		}
	}
	return false
}
func Unique(values []string) []string {
	out := []string{}
	for _, v := range values {
		if !Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
func MarshalJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := marshalOrdered(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func marshalOrdered(b *bytes.Buffer, v any) error {
	switch value := v.(type) {
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			b.WriteString("null")
			return nil
		}
		if value == 0 {
			b.WriteByte('0')
			return nil
		}
	case float32:
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			b.WriteString("null")
			return nil
		}
		if value == 0 {
			b.WriteByte('0')
			return nil
		}
	}
	var object map[string]any
	switch m := v.(type) {
	case Record:
		if raw, ok := rawRecordValues.Load(reflect.ValueOf(m).Pointer()); ok {
			return marshalOrdered(b, raw.(rawRecordValue).value)
		}
		object = map[string]any(m)
	case Config:
		object = map[string]any(m)
	case map[string]any:
		object = m
	}
	if object != nil {
		b.WriteByte('{')
		for i, key := range ObjectKeys(object) {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := marshalOrdered(b, key); err != nil {
				return err
			}
			b.WriteByte(':')
			if err := marshalOrdered(b, object[key]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	}
	if v != nil {
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			if rv.Kind() == reflect.Slice && rv.IsNil() {
				b.WriteString("null")
				return nil
			}
			b.WriteByte('[')
			for i := 0; i < rv.Len(); i++ {
				if i > 0 {
					b.WriteByte(',')
				}
				if err := marshalOrdered(b, rv.Index(i).Interface()); err != nil {
					return err
				}
			}
			b.WriteByte(']')
			return nil
		}
	}
	e := json.NewEncoder(b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return err
	}
	data := b.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		b.Truncate(len(data) - 1)
	}
	return nil
}
func JSONPretty(v any) string {
	b, e := MarshalJSON(v)
	if e != nil {
		return ""
	}
	var dst bytes.Buffer
	_ = json.Indent(&dst, b, "", "  ")
	return dst.String()
}
func ParseJSON(data []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	value, err := decodeJSON(d)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected non-whitespace character after JSON")
		}
		return nil, err
	}
	return value, nil
}
func decodeJSON(d *json.Decoder) (any, error) {
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	if delim == '{' {
		m := map[string]any{}
		keys := []string{}
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return nil, e
			}
			k := key.(string)
			v, e := decodeJSON(d)
			if e != nil {
				return nil, e
			}
			m[k] = v
			keys = append(keys, k)
		}
		_, err = d.Token()
		rememberOrder(m, keys)
		return m, err
	}
	if delim == '[' {
		a := []any{}
		for d.More() {
			v, e := decodeJSON(d)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		_, err = d.Token()
		return a, err
	}
	return nil, fmt.Errorf("unexpected JSON delimiter")
}
func ConfigValue(cfg Config, path ...string) any {
	var v any = cfg
	for _, p := range path {
		v = Obj(v)[p]
	}
	return v
}
func NumberDefault(v any, fallback float64) float64 {
	if v == nil {
		return fallback
	}
	return Number(v)
}
func StringDefault(v any, fallback string) string {
	if v == nil {
		return fallback
	}
	return Text(v)
}
func DomainNames(cfg Config) []string { return ObjectKeys(Obj(cfg["domains"])) }
