package atlas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

type parityResult struct {
	Code     int
	Out, Err string
}
type parityStep struct {
	Args  []string
	Input string
}

func parityReference(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "compat", "reference", "src", "cli.ts")
	if _, e := os.Stat(path); e != nil {
		t.Fatal(e)
	}
	if _, e := exec.LookPath("bun"); e != nil {
		t.Skip("Bun is required only for the development compatibility oracle")
	}
	return path
}
func parityRun(t *testing.T, reference, dir string, step parityStep, native bool) parityResult {
	t.Helper()
	if native {
		var out, stderr bytes.Buffer
		app := &App{Cwd: dir, In: strings.NewReader(step.Input), Out: &out, Err: &stderr}
		code := app.Run(step.Args)
		return parityResult{code, out.String(), stderr.String()}
	}
	cmd := exec.Command("bun", append([]string{reference}, step.Args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "FORCE_COLOR=0", "LUUNA_ATLAS_LOG_JSON=1")
	cmd.Stdin = strings.NewReader(step.Input)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	e := cmd.Run()
	code := 0
	if exit, ok := e.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if e != nil {
		t.Fatal(e)
	}
	return parityResult{code, out.String(), stderr.String()}
}
func parityNormalize(value any, dir string, started time.Time) any {
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range v {
			out[k] = parityNormalize(x, dir, started)
		}
		return out
	case []any:
		out := []any{}
		for _, x := range v {
			out = append(out, parityNormalize(x, dir, started))
		}
		return out
	case string:
		v = strings.ReplaceAll(v, strings.ReplaceAll(dir, `\`, `\\`), "<PROJECT>")
		v = strings.ReplaceAll(v, dir, "<PROJECT>")
		v = strings.ReplaceAll(v, filepath.ToSlash(dir), "<PROJECT>")
		if date, e := time.Parse(time.RFC3339Nano, v); e == nil && date.After(started.Add(-time.Minute)) {
			return "<GENERATED-TIMESTAMP>"
		}
		v = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?Z`).ReplaceAllStringFunc(v, func(text string) string {
			if date, err := time.Parse(time.RFC3339Nano, text); err == nil && date.After(started.Add(-time.Minute)) {
				return "<GENERATED-TIMESTAMP>"
			}
			return text
		})
		return strings.ReplaceAll(v, "\r\n", "\n")
	default:
		return v
	}
}
func parityOutput(text, dir string, started time.Time) any {
	dec := json.NewDecoder(strings.NewReader(text))
	values := []any{}
	for {
		var v any
		e := dec.Decode(&v)
		if e == io.EOF {
			break
		}
		if e != nil {
			return parityNormalize(text, dir, started)
		}
		values = append(values, parityNormalize(v, dir, started))
	}
	if len(values) == 0 {
		return ""
	}
	return values
}
func parityCompare(t *testing.T, label string, ref, got parityResult, refDir, goDir string, started time.Time) {
	t.Helper()
	if parityCompareUncaught(t, label, ref, got, refDir, goDir, started) {
		return
	}
	if ref.Code != got.Code {
		t.Errorf("%s: exit reference=%d Go=%d\nref stdout=%s\nref stderr=%s\nGo stdout=%s\nGo stderr=%s", label, ref.Code, got.Code, ref.Out, ref.Err, got.Out, got.Err)
	}
	for _, stream := range []struct{ name, ref, got string }{{"stdout", ref.Out, got.Out}, {"stderr", ref.Err, got.Err}} {
		want := parityOutput(stream.ref, refDir, started)
		actual := parityOutput(stream.got, goDir, started)
		if !reflect.DeepEqual(want, actual) {
			t.Errorf("%s: %s mismatch\nREFERENCE:\n%s\nGO:\n%s", label, stream.name, stream.ref, stream.got)
		}
	}
}
func parityTree(t *testing.T, dir string, started time.Time) map[string]any {
	t.Helper()
	out := map[string]any{}
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		relative, _ := filepath.Rel(dir, path)
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(relative, ".lock") || strings.Contains(relative, ".tmp.") {
			t.Errorf("leaked transient file: %s", relative)
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		switch {
		case strings.HasSuffix(path, ".jsonl"):
			rows := []any{}
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				var value any
				if json.Unmarshal([]byte(line), &value) == nil {
					rows = append(rows, parityNormalize(value, dir, started))
				} else {
					rows = append(rows, line)
				}
			}
			out[relative] = rows
		case strings.HasSuffix(path, ".yaml"):
			var value any
			if e := yaml.Unmarshal(data, &value); e != nil {
				out[relative] = string(data)
			} else {
				out[relative] = parityNormalize(value, dir, started)
			}
		case strings.HasSuffix(path, ".json"):
			var value any
			if json.Unmarshal(data, &value) == nil {
				out[relative] = parityNormalize(value, dir, started)
			} else {
				out[relative] = string(data)
			}
		default:
			out[relative] = parityNormalize(string(data), dir, started)
		}
		return nil
	})
	return out
}
func parityProjects(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	ref, goDir := filepath.Join(root, "reference", "project"), filepath.Join(root, "go", "project")
	for _, dir := range []string{ref, goDir} {
		if e := os.MkdirAll(dir, 0755); e != nil {
			t.Fatal(e)
		}
	}
	return ref, goDir
}
func parityExecute(t *testing.T, steps []parityStep, prepare func(string)) {
	t.Helper()
	reference := parityReference(t)
	refDir, goDir := parityProjects(t)
	if prepare != nil {
		prepare(refDir)
		prepare(goDir)
	}
	started := time.Now()
	for n, step := range steps {
		label := fmt.Sprintf("step %d: %s", n, strings.Join(step.Args, " "))
		t.Run(label, func(t *testing.T) {
			ref := parityRun(t, reference, refDir, step, false)
			got := parityRun(t, reference, goDir, step, true)
			parityCompare(t, label, ref, got, refDir, goDir, started)
			want, actual := parityTree(t, refDir, started), parityTree(t, goDir, started)
			if !reflect.DeepEqual(want, actual) {
				w, _ := json.MarshalIndent(want, "", "  ")
				g, _ := json.MarshalIndent(actual, "", "  ")
				t.Errorf("persisted state mismatch\nREFERENCE:%s\nGO:%s", w, g)
			}
		})
	}
}
func TestParityHelpAndParser(t *testing.T) {
	reference := parityReference(t)
	refDir, goDir := parityProjects(t)
	steps := []parityStep{{Args: []string{"--help"}}, {Args: []string{"--version"}}, {Args: []string{"--version", "--json"}}, {Args: []string{"serch", "--json"}}, {Args: []string{"record"}}, {Args: []string{"record", "test", "--classification", "wrong"}}, {Args: []string{"archive", "test", "mx-abc123"}}, {Args: []string{"query", "--bad"}}}
	for _, spec := range contract.CLI.Commands {
		steps = append(steps, parityStep{Args: []string{spec.Name, "--help"}})
		for _, child := range spec.Commands {
			steps = append(steps, parityStep{Args: []string{spec.Name, child.Name, "--help"}})
		}
	}
	for _, step := range steps {
		t.Run(strings.Join(step.Args, " "), func(t *testing.T) {
			ref := parityRun(t, reference, refDir, step, false)
			got := parityRun(t, reference, goDir, step, true)
			parityCompare(t, strings.Join(step.Args, " "), ref, got, refDir, goDir, time.Now())
		})
	}
}
func TestParityLifecycle(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	steps := []parityStep{{Args: []string{"status", "--json"}}, {Args: []string{"init", "--json"}}, {Args: []string{"init"}}, {Args: []string{"add", "architecture", "--json"}}, {Args: []string{"add", "testing"}}, {Args: []string{"add", "architecture", "--json"}}, {Args: []string{"record", "architecture", "--type", "decision", "--title", "Atomic replacement", "--rationale", "Concurrent readers must see a complete file", "--classification", "foundational", "--json"}}, {Args: []string{"record", "testing", "--type", "failure", "--description", "Shared temp filename", "--resolution", "Use a unique temp directory", "--classification", "foundational", "--json"}}, {Args: []string{"record", "architecture", "--type", "pattern", "--name", "Scoped locking", "--description", "Use a lock per domain file", "--files", "src/a.ts", "--json"}}, {Args: []string{"record", "architecture", "--type", "reference", "--name", "Schema docs", "--description", "See the schema source", "--files", "src/schema.ts", "--json"}}, {Args: []string{"record", "architecture", "--type", "guide", "--name", "Review records", "--description", "Query then validate", "--json"}}, {Args: []string{"record", "testing", "--type", "convention", "--content", "Avoid shared state because tests run concurrently", "--dir-anchor", "test", "--classification", "foundational", "--json"}}, {Args: []string{"query", "--all", "--json"}}, {Args: []string{"query", "architecture", "--format", "compact"}}, {Args: []string{"query", "architecture", "--format", "xml"}}, {Args: []string{"query", "architecture", "--format", "plain"}}, {Args: []string{"query", "architecture", "--format", "ids"}}, {Args: []string{"search", "lock", "--json"}}, {Args: []string{"search", "absent"}}, {Args: []string{"search", "--type", "failure", "--json"}}, {Args: []string{"ready", "--json"}}, {Args: []string{"rank", "--json"}}, {Args: []string{"status", "--json"}}, {Args: []string{"validate", "--json"}}, {Args: []string{"stale", "--json"}}, {Args: []string{"prime", "--json"}}, {Args: []string{"prime", "--all", "--records-only"}}, {Args: []string{"prime", "--all", "--full", "--records-only"}}, {Args: []string{"prime", "--manifest", "--json"}}, {Args: []string{"prime", "--dry-run", "--budget", "20"}}, {Args: []string{"compact", "architecture", "--analyze", "--json"}}, {Args: []string{"prune", "--dry-run", "--json"}}, {Args: []string{"doctor", "--json"}}, {Args: []string{"audit", "--json", "--suggest"}}, {Args: []string{"config", "show"}}, {Args: []string{"config", "show", "--path", "search"}}, {Args: []string{"config", "set", "search.boost_factor", "0.2"}}, {Args: []string{"config", "unset", "search.boost_factor"}}, {Args: []string{"config", "schema"}}, {Args: []string{"upgrade", "--check", "--json"}}, {Args: []string{"update", "--json"}}}
	parityExecute(t, steps, func(dir string) {
		_ = os.MkdirAll(filepath.Join(dir, "src"), 0755)
		_ = os.MkdirAll(filepath.Join(dir, "test"), 0755)
		_ = os.WriteFile(filepath.Join(dir, "src", "a.ts"), []byte("export {}\n"), 0644)
		_ = os.WriteFile(filepath.Join(dir, "src", "schema.ts"), []byte("export {}\n"), 0644)
	})
}
func TestParityProviderRoundTrips(t *testing.T) {
	steps := []parityStep{{Args: []string{"init", "--json"}}, {Args: []string{"setup", "--list", "--json"}}}
	for _, provider := range []string{"claude", "cursor", "codex"} {
		steps = append(steps, parityStep{Args: []string{"setup", provider, "--json"}}, parityStep{Args: []string{"setup", provider, "--check", "--json"}}, parityStep{Args: []string{"setup", provider, "--remove", "--json"}})
	}
	steps = append(steps, parityStep{Args: []string{"onboard", "--stdout"}}, parityStep{Args: []string{"onboard", "--json"}}, parityStep{Args: []string{"onboard", "--check", "--json"}}, parityStep{Args: []string{"completions", "bash"}}, parityStep{Args: []string{"completions", "zsh"}}, parityStep{Args: []string{"completions", "fish"}})
	parityExecute(t, steps, nil)
}
