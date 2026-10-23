package atlas

import (
	"math"
	"reflect"
	"strconv"
	"strings"
)

// Numeric sidecars are private IPC metadata. Public JSON continues to follow
// JSON.stringify: non-finite numbers become null and negative zero becomes zero.
type TransportNumber struct {
	Path  []any  `json:"path"`
	Value string `json:"value"`
}

func RestoreTransportNumbers(request any) any {
	frame := Obj(request)
	for _, entry := range AnyList(frame["numbers"]) {
		item := Obj(entry)
		path := AnyList(item["path"])
		value := 0.0
		switch Text(item["value"]) {
		case "NaN":
			value = math.NaN()
		case "Infinity":
			value = math.Inf(1)
		case "-Infinity":
			value = math.Inf(-1)
		case "-0":
			value = math.Copysign(0, -1)
		default:
			continue
		}
		node := request
		for i, part := range path {
			last := i == len(path)-1
			if object, ok := node.(map[string]any); ok {
				key := Text(part)
				if last {
					object[key] = value
				} else {
					node = object[key]
				}
			} else {
				index := int(Number(part))
				if key, ok := part.(string); ok {
					parsed, err := strconv.Atoi(key)
					if err != nil {
						break
					}
					index = parsed
				}
				array, ok := node.([]any)
				if !ok || index < 0 || index >= len(array) || float64(index) != Number(part) {
					break
				}
				if last {
					array[index] = value
				} else {
					node = array[index]
				}
			}
		}
	}
	return request
}

func CollectTransportNumbers(result any) []TransportNumber {
	out := []TransportNumber{}
	var walk func(reflect.Value, []any)
	walk = func(value reflect.Value, path []any) {
		if !value.IsValid() {
			return
		}
		for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return
			}
			value = value.Elem()
		}
		if value.CanInterface() {
			if record, ok := value.Interface().(Record); ok {
				walk(reflect.ValueOf(RecordValue(record)), path)
				return
			}
		}
		switch value.Kind() {
		case reflect.Float32, reflect.Float64:
			number := value.Float()
			special := ""
			switch {
			case math.IsNaN(number):
				special = "NaN"
			case math.IsInf(number, 1):
				special = "Infinity"
			case math.IsInf(number, -1):
				special = "-Infinity"
			case number == 0 && math.Signbit(number):
				special = "-0"
			}
			if special != "" {
				out = append(out, TransportNumber{append([]any{}, path...), special})
			}
		case reflect.Map:
			if value.IsNil() || value.Type().Key().Kind() != reflect.String {
				return
			}
			object := Obj(value.Interface())
			for _, key := range ObjectKeys(object) {
				walk(reflect.ValueOf(object[key]), append(append([]any{}, path...), key))
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < value.Len(); i++ {
				walk(value.Index(i), append(append([]any{}, path...), i))
			}
		case reflect.Struct:
			typ := value.Type()
			for i := 0; i < value.NumField(); i++ {
				field := typ.Field(i)
				if field.PkgPath != "" {
					continue
				}
				tag := field.Tag.Get("json")
				parts := strings.Split(tag, ",")
				key := parts[0]
				if key == "-" {
					continue
				}
				if key == "" {
					key = field.Name
				}
				omit := false
				for _, option := range parts[1:] {
					if option == "omitempty" && transportEmpty(value.Field(i)) {
						omit = true
					}
				}
				if !omit {
					walk(value.Field(i), append(append([]any{}, path...), key))
				}
			}
		}
	}
	walk(reflect.ValueOf(result), []any{})
	return out
}
func transportEmpty(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.Interface, reflect.Pointer:
		return value.IsZero()
	}
	return false
}

// Defaults must retain non-finite numbers restored by the transport. A JSON
// round trip would replace them with null before configuration merging.
func cloneTransportValue(value any) any {
	switch v := value.(type) {
	case Config:
		return Config(Obj(cloneTransportValue(map[string]any(v))))
	case Record:
		return RecordFromValue(cloneTransportValue(RecordValue(v)))
	case map[string]any:
		copied := map[string]any{}
		for _, key := range ObjectKeys(v) {
			copied[key] = cloneTransportValue(v[key])
		}
		rememberOrder(copied, ObjectKeys(v))
		return copied
	case []any:
		copied := make([]any, len(v))
		for i, item := range v {
			copied[i] = cloneTransportValue(item)
		}
		return copied
	case []string:
		return append([]string{}, v...)
	}
	return value
}
