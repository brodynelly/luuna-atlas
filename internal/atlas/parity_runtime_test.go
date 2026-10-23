package atlas

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

var parityUncaughtMessage = regexp.MustCompile(`(?m)^(?:error|TypeError|RangeError|SyntaxError|ReferenceError): ([^\r\n]+)`)
var parityBunStack = regexp.MustCompile(`(?m)^\s+at .+\.ts:\d+:\d+\)?\s*$`)
var parityStackStart = regexp.MustCompile(`(?m)^[ \t]+at `)

// Uncaught failures formerly included Bun/JSC source frames and a runtime
// version. Preserve their diagnostic, exit status, stdout and persisted data;
// source frames are specific to the replaced runtime. Handled CLI failures
// continue through the ordinary, exact output comparison.
func parityCompareUncaught(t *testing.T, label string, ref, got parityResult, refDir, goDir string, started time.Time) bool {
	t.Helper()
	match := parityUncaughtMessage.FindStringSubmatchIndex(ref.Err)
	if ref.Code != 1 || len(match) != 4 || !strings.Contains(ref.Err, "Bun v") || !parityBunStack.MatchString(ref.Err) {
		return false
	}
	message := ref.Err[match[2]:]
	if stack := parityStackStart.FindStringIndex(message); stack != nil {
		message = strings.TrimRight(message[:stack[0]], "\r\n")
	}
	if got.Code != 1 {
		t.Errorf("%s: uncaught reference failure exited 1, Go exited %d", label, got.Code)
	}
	if !reflect.DeepEqual(parityOutput(ref.Out, refDir, started), parityOutput(got.Out, goDir, started)) {
		t.Errorf("%s: stdout changed on uncaught failure\nREFERENCE:\n%s\nGO:\n%s", label, ref.Out, got.Out)
	}
	var result map[string]any
	diagnostic := ""
	if e := json.Unmarshal([]byte(got.Err), &result); e == nil && result["success"] == false {
		diagnostic = Text(result["error"])
	} else {
		line := strings.TrimSpace(got.Err)
		if strings.HasPrefix(line, "Error: ") {
			diagnostic = strings.TrimPrefix(line, "Error: ")
		}
	}
	want := parityNormalize(message, refDir, started)
	actual := parityNormalize(diagnostic, goDir, started)
	if !reflect.DeepEqual(want, actual) {
		t.Errorf("%s: uncaught failure diagnostic mismatch\nREFERENCE:\n%s\nGO:\n%s", label, message, got.Err)
	}
	return true
}
