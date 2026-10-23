package atlas

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func integrationShell(t *testing.T) {
	t.Helper()
	if _, e := exec.LookPath("sh"); e == nil {
		return
	}
	if runtime.GOOS == "windows" {
		out, e := exec.Command("git", "--exec-path").Output()
		if e == nil {
			usr := filepath.Clean(filepath.Join(strings.TrimSpace(string(out)), "..", "..", "..", "usr", "bin"))
			if setupExists(filepath.Join(usr, "sh.exe")) {
				t.Setenv("PATH", usr+string(os.PathListSeparator)+os.Getenv("PATH"))
				return
			}
		}
	}
	t.Fatal("real sh required for hook compatibility tests")
}
func TestLifecycleHookMutationBlockAndObserve(t *testing.T) {
	integrationShell(t)
	cwd := t.TempDir()
	cfg := Config{"hooks": map[string]any{"pre-record": []string{"printf '{\"payload\":{\"domain\":\"ops\",\"record\":{\"content\":\"updated\"}}}'"}}}
	result, e := RunHooks(cwd, cfg, "pre-record", map[string]any{"domain": "ops", "record": map[string]any{"content": "before"}}, nil)
	if e != nil || result.Blocked || !result.RanAny {
		t.Fatalf("%+v %v", result, e)
	}
	if Text(Obj(Obj(result.Payload)["record"])["content"]) != "updated" {
		t.Fatal(result.Payload)
	}
	marker := filepath.ToSlash(filepath.Join(cwd, "unexpected"))
	cfg["hooks"] = map[string]any{"pre-record": []string{"exit 9", "touch '" + marker + "'"}}
	result, e = RunHooks(cwd, cfg, "pre-record", map[string]any{}, nil)
	if e != nil || !result.Blocked || len(result.Executions) != 1 || setupExists(marker) {
		t.Fatalf("block failed %+v %v", result, e)
	}
	cfg["hooks"] = map[string]any{"post-record": []string{"exit 7", "printf observed"}}
	result, e = RunHooks(cwd, cfg, "post-record", map[string]any{}, nil)
	if e != nil || result.Blocked || len(result.Executions) != 2 || len(result.Warnings) != 1 {
		t.Fatalf("post semantics %+v %v", result, e)
	}
}
func TestLifecycleTimeoutReturnsPromptly(t *testing.T) {
	integrationShell(t)
	cfg := Config{"hooks": map[string]any{"pre-prune": []string{"sleep 5"}}, "hook_settings": map[string]any{"timeout_ms": 50}}
	start := time.Now()
	result, e := RunHooks(t.TempDir(), cfg, "pre-prune", map[string]any{}, nil)
	if e != nil || !result.Blocked || len(result.Executions) != 1 || !result.Executions[0].TimedOut {
		t.Fatalf("%+v %v", result, e)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout did not bound process wait")
	}
}
func TestGuardAndHookFailOpen(t *testing.T) {
	cwd := t.TempDir()
	for _, command := range []string{"echo x > .luuna/atlas/expertise/a.jsonl", "cat x | tee .luuna/atlas/expertise/a.jsonl", "rm .luuna/atlas/expertise/a.jsonl", "sed -i x .luuna/atlas/expertise/a.jsonl", "cp x .luuna/atlas/expertise/a.jsonl"} {
		if !BashWritesExpertise(command) {
			t.Fatalf("did not deny %s", command)
		}
	}
	for _, command := range []string{"cat .luuna/atlas/expertise/a.jsonl", "luuna-atlas record a --type convention x", "git add .luuna/atlas/expertise/a.jsonl"} {
		if BashWritesExpertise(command) {
			t.Fatalf("wrongly denied %s", command)
		}
	}
	if HookOutput("not-json", cwd, BuiltinRegistry()) != "" {
		t.Fatal("invalid hook should fail open")
	}
	out := HookOutput(`{"tool_name":"Write","tool_input":{"file_path":".luuna/atlas/expertise/a.jsonl"}}`, cwd, BuiltinRegistry())
	if !strings.Contains(out, `"permissionDecision":"deny"`) {
		t.Fatal(out)
	}
}
func TestHookInjectionPersistsOnlyIgnoredUsageAndDeduplicates(t *testing.T) {
	cwd := t.TempDir()
	if _, _, e := InitStore(cwd); e != nil {
		t.Fatal(e)
	}
	if e := AddDomain(cwd, "code"); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(cwd, "src"), 0777); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(cwd, "src", "worker.go"), []byte("package worker"), 0666); e != nil {
		t.Fatal(e)
	}
	reg := BuiltinRegistry()
	path, e := ExpertisePath(cwd, "code")
	if e != nil {
		t.Fatal(e)
	}
	r := Record{"id": "mx-abc123", "type": "failure", "description": "avoid concurrent fixture sharing", "resolution": "use isolated temp paths", "classification": "foundational", "recorded_at": NowISO(), "dir_anchors": []string{"src"}}
	if e = WriteRecords(path, []Record{r}); e != nil {
		t.Fatal(e)
	}
	req := InjectRequest{File: "src/worker.go", Cwd: cwd, Session: "session/one", Tool: "Read", Registry: reg}
	out, e := InjectForFile(req)
	if e != nil || !strings.Contains(out, "mx-abc123") {
		t.Fatalf("%q %v", out, e)
	}
	second, e := InjectForFile(req)
	if e != nil || second != "" {
		t.Fatalf("duplicate %q %v", second, e)
	}
	dir, _ := AtlasDir(cwd)
	if setupRead(filepath.Join(dir, "state", ".gitignore")) != "*\n" {
		t.Fatal("state ignore")
	}
	usage := setupRead(filepath.Join(dir, "state", "usage.jsonl"))
	if strings.Count(usage, "\n") != 1 {
		t.Fatal(usage)
	}
}
func TestOnboardAndBuiltInSetupRoundTrips(t *testing.T) {
	cwd := t.TempDir()
	if _, _, e := InitStore(cwd); e != nil {
		t.Fatal(e)
	}
	var out, errOut bytes.Buffer
	app := &App{Cwd: cwd, Out: &out, Err: &errOut, JSONMode: true, Registry: BuiltinRegistry()}
	if e := app.runOnboard(Invocation{}); e != nil {
		t.Fatal(e)
	}
	before := setupRead(filepath.Join(cwd, "CLAUDE.md"))
	if !HasMarkerSection(before) || !strings.Contains(before, "atlas-onboard:v0.11.0") {
		t.Fatal(before)
	}
	if e := app.runOnboard(Invocation{}); e != nil {
		t.Fatal(e)
	}
	if setupRead(filepath.Join(cwd, "CLAUDE.md")) != before {
		t.Fatal("onboarding not idempotent")
	}
	for _, name := range builtinProviderNames {
		for _, action := range []string{"install", "check", "remove"} {
			var result RecipeResult
			var e error
			switch name {
			case "claude":
				result, e = runClaudeRecipe(cwd, action)
			case "cursor":
				result, e = runCursorRecipe(cwd, action)
			case "codex":
				result, e = runCodexRecipe(cwd, action)
			}
			if e != nil || !result.Success {
				t.Fatalf("%s %s %+v %v", name, action, result, e)
			}
		}
	}
}
func TestTypeScriptRecipeIsExtensionOnly(t *testing.T) {
	if _, e := exec.LookPath("bun"); e != nil {
		t.Skip("Bun needed only for an explicitly configured TS extension")
	}
	cwd := t.TempDir()
	path := filepath.Join(cwd, ".luuna", "atlas", "recipes", "custom.ts")
	if e := setupWrite(path, `export default {async install(cwd:string){return {success:true,message:'installed in '+cwd}},async check(){return {success:true,message:'checked'}},async remove(){return {success:true,message:'removed'}}}`); e != nil {
		t.Fatal(e)
	}
	var out, errOut bytes.Buffer
	app := &App{Cwd: cwd, Out: &out, Err: &errOut}
	r := resolveProvider("custom", cwd)
	if r == nil {
		t.Fatal("recipe missing")
	}
	result, e := app.runExternalRecipe(*r, "check")
	if e != nil || !result.Success || result.Message != "checked" {
		t.Fatalf("%+v %v output %s %s", result, e, out.String(), errOut.String())
	}
}
