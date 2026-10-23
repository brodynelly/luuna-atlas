package atlas

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The released executable must operate independently of the development oracle,
// JavaScript runtimes, Go itself, and the source checkout.
func TestNativeExecutableRunsWithEmptyPath(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate module root")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	goTool := filepath.Join(runtime.GOROOT(), "bin", "go"+ext)
	if _, e := os.Stat(goTool); e != nil {
		t.Fatal(e)
	}
	buildDir := t.TempDir()
	built := filepath.Join(buildDir, "luuna-atlas"+ext)
	build := exec.Command(goTool, "build", "-o", built, "./cmd/luuna-atlas")
	build.Dir = root
	if output, e := build.CombinedOutput(); e != nil {
		t.Fatalf("build native executable: %v\n%s", e, output)
	}
	isolated := t.TempDir()
	executable := filepath.Join(isolated, "luuna-atlas"+ext)
	data, e := os.ReadFile(built)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(executable, data, 0755); e != nil {
		t.Fatal(e)
	}
	project := filepath.Join(isolated, "project")
	if e = os.Mkdir(project, 0755); e != nil {
		t.Fatal(e)
	}
	env := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "PATH") && key != "FORCE_COLOR" && key != "NO_COLOR" {
			env = append(env, entry)
		}
	}
	env = append(env, "PATH=", "FORCE_COLOR=0", "NO_COLOR=1")
	var last map[string]any
	for _, args := range [][]string{
		{"init", "--json"},
		{"record", "knowledge", "--type", "decision", "--title", "Independent native executable", "--rationale", "No language runtime is present on PATH", "--classification", "foundational", "--json", "--quiet"},
		{"query", "knowledge", "--json"},
	} {
		cmd := exec.Command(executable, args...)
		cmd.Dir = project
		cmd.Env = env
		output, e := cmd.Output()
		if e != nil {
			t.Fatalf("isolated %v: %v\n%s", args, e, output)
		}
		last = map[string]any{}
		if e = json.Unmarshal(output, &last); e != nil {
			t.Fatalf("isolated %v returned invalid JSON: %v\n%s", args, e, output)
		}
		if success, ok := last["success"].(bool); !ok || !success {
			t.Fatalf("isolated %v failed: %s", args, output)
		}
	}
	domains := AnyList(last["domains"])
	if len(domains) != 1 || Text(Obj(domains[0])["domain"]) != "knowledge" {
		t.Fatalf("isolated query returned wrong domain: %#v", last)
	}
	records := AnyList(Obj(domains[0])["records"])
	if len(records) != 1 || Text(Obj(records[0])["title"]) != "Independent native executable" {
		t.Fatalf("isolated query lost persisted record: %#v", last)
	}
	path := filepath.Join(project, ".luuna", "atlas", "expertise", "knowledge.jsonl")
	if _, e = os.Stat(path); e != nil {
		t.Fatalf("isolated executable did not persist project data: %v", e)
	}
}
