package atlas

import (
	"math"
	"reflect"
	"testing"
)

func TestNativeNumericTransportSidecars(t *testing.T) {
	request := map[string]any{"args": []any{nil, nil, nil, float64(0)}, "numbers": []any{
		map[string]any{"path": []any{"args", float64(0)}, "value": "NaN"},
		map[string]any{"path": []any{"args", float64(1)}, "value": "Infinity"},
		map[string]any{"path": []any{"args", float64(2)}, "value": "-Infinity"},
		map[string]any{"path": []any{"args", float64(3)}, "value": "-0"},
	}}
	restored := Obj(RestoreTransportNumbers(request))
	values := AnyList(restored["args"])
	if !math.IsNaN(Number(values[0])) || !math.IsInf(Number(values[1]), 1) || !math.IsInf(Number(values[2]), -1) || !math.Signbit(Number(values[3])) {
		t.Fatalf("numeric restoration failed: %v", values)
	}
	type tagged struct {
		Values  []any   `json:"values"`
		Ignored float64 `json:"-"`
		Omitted float64 `json:"omitted,omitempty"`
	}
	result := tagged{Values: values, Ignored: math.Inf(1), Omitted: math.Copysign(0, -1)}
	sidecar := CollectTransportNumbers(result)
	if len(sidecar) != 4 {
		t.Fatalf("unexpected sidecar: %v", sidecar)
	}
	if !reflect.DeepEqual(sidecar[3].Path, []any{"values", 3}) || sidecar[3].Value != "-0" {
		t.Fatalf("wrong JSON path: %v", sidecar[3])
	}
	root := CollectTransportNumbers(math.NaN())
	encoded, err := MarshalJSON(root)
	if err != nil || string(encoded) != `[{"path":[],"value":"NaN"}]` {
		t.Fatalf("root numeric path must be empty array: %s %v", encoded, err)
	}
}
