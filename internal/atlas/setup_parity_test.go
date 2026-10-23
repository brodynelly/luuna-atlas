package atlas

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParityForcedColorBytes(t *testing.T) {
	reference := parityReference(t)
	palette := filepath.ToSlash(filepath.Join(filepath.Dir(reference), "utils", "palette.ts"))
	root := filepath.Clean(filepath.Join(filepath.Dir(reference), "..", "..", ".."))
	chalk := filepath.ToSlash(filepath.Join(root, "node_modules", "chalk", "source", "index.js"))
	paletteURL, chalkURL := "file:///"+palette, "file:///"+chalk
	p, _ := json.Marshal(paletteURL)
	c, _ := json.Marshal(chalkURL)
	script := "import {brand,accent,muted} from " + string(p) + ";import chalk from " + string(c) + ";console.log(JSON.stringify([brand('soil'),accent('id'),muted('meta'),chalk.green('ok'),chalk.bold('one\\ntwo'),brand(chalk.red('inner'))]));"
	for _, force := range []string{"0", "1", "2", "3"} {
		t.Run(force, func(t *testing.T) {
			t.Setenv("FORCE_COLOR", force)
			cmd := exec.Command("bun", "-e", script)
			data, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			if err = json.Unmarshal(data, &want); err != nil {
				t.Fatalf("%s %v", data, err)
			}
			got := []string{Style("brand", "soil"), Style("accent", "id"), Style("muted", "meta"), Style("green", "ok"), Style("bold", "one\ntwo"), Style("brand", Style("red", "inner"))}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("FORCE_COLOR=%s\nreference=%q\ngo=%q", force, want, got)
			}
		})
	}
}

func TestParityConfiguredHooksAndInjection(t *testing.T) {
	integrationShell(t)
	steps := []parityStep{
		{Args: []string{"record", "code", "--type", "failure", "--description", "before worker race", "--resolution", "use isolated state", "--classification", "foundational", "--dir-anchor", "src", "--json"}},
		{Args: []string{"query", "code", "--json"}},
		{Args: []string{"prime", "--files", "src/a.go", "--records-only"}},
		{Args: []string{"prune", "--json"}},
		{Args: []string{"hook"}, Input: `{"tool_name":"Read","tool_input":{"file_path":"src/a.go"},"session_id":"test-session"}`},
		{Args: []string{"hook"}, Input: `{"tool_name":"Read","tool_input":{"file_path":"src/a.go"},"session_id":"test-session"}`},
		{Args: []string{"hook"}, Input: `{"tool_name":"Write","tool_input":{"file_path":".luuna/atlas/expertise/code.jsonl"}}`},
		{Args: []string{"hook"}, Input: `not JSON`},
	}
	parityExecute(t, steps, func(dir string) {
		if _, _, e := InitStore(dir); e != nil {
			t.Fatal(e)
		}
		if e := AddDomain(dir, "code"); e != nil {
			t.Fatal(e)
		}
		if e := os.MkdirAll(filepath.Join(dir, "src"), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("package a\n"), 0644); e != nil {
			t.Fatal(e)
		}
		cfg, e := ReadConfig(dir)
		if e != nil {
			t.Fatal(e)
		}
		cfg["hooks"] = map[string]any{"pre-record": []string{"sed 's/before/after/g'"}, "post-record": []string{"cat > post.json"}, "pre-prime": []string{"cat > prime.json; cat prime.json"}, "pre-prune": []string{"cat > prune.json"}}
		if e = WriteConfig(dir, cfg); e != nil {
			t.Fatal(e)
		}
	})
}
func TestParityCustomRecipeExtensions(t *testing.T) {
	integrationShell(t)
	steps := []parityStep{
		{Args: []string{"setup", "custom", "--json"}}, {Args: []string{"setup", "custom", "--check", "--json"}}, {Args: []string{"setup", "custom", "--remove", "--json"}},
		{Args: []string{"setup", "shell", "--json"}}, {Args: []string{"setup", "shell", "--check", "--json"}}, {Args: []string{"setup", "shell", "--remove", "--json"}},
		{Args: []string{"setup", "invalid", "--json"}}, {Args: []string{"setup", "invalid-shape", "--json"}}, {Args: []string{"setup", "throws", "--json"}},
		{Args: []string{"setup", "packaged", "--json"}}, {Args: []string{"setup", "packaged", "--check", "--json"}},
		{Args: []string{"setup", "conditional", "--json"}},
		{Args: []string{"setup", "--list", "--json"}},
	}
	parityExecute(t, steps, func(dir string) {
		if _, _, e := InitStore(dir); e != nil {
			t.Fatal(e)
		}
		recipes := filepath.Join(dir, ".luuna", "atlas", "recipes")
		if e := os.MkdirAll(recipes, 0755); e != nil {
			t.Fatal(e)
		}
		files := map[string]string{
			"custom.ts":        `export default { async install(){console.log('custom extension notice');return {success:true,message:'installed'}}, async check(){return {success:true,message:'checked'}}, async remove(){return {success:true,message:'removed'}} }`,
			"shell.sh":         "#!/bin/sh\necho \"$LUUNA_ATLAS_RECIPE_NAME $LUUNA_ATLAS_RECIPE_ACTION\"\n",
			"invalid.ts":       `export async function install(){return {success:true,message:'no default'}}`,
			"invalid-shape.ts": `export default { install: 'not a function' }`,
			"throws.ts":        `export default { async install(){throw new Error('extension boom')},async check(){return {success:true,message:'checked'}},async remove(){return {success:true,message:'removed'}} }`,
		}
		for name, content := range files {
			if e := os.WriteFile(filepath.Join(recipes, name), []byte(content), 0755); e != nil {
				t.Fatal(e)
			}
		}
		pkg := filepath.Join(dir, "node_modules", "luuna-atlas-recipe-packaged")
		if e := os.MkdirAll(pkg, 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"luuna-atlas-recipe-packaged","type":"module","main":"index.js"}`), 0644); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(pkg, "index.js"), []byte(`export default {async install(){return {success:true,message:'package installed'}},async check(){return {success:true,message:'package checked'}},async remove(){return {success:true,message:'package removed'}}}`), 0644); e != nil {
			t.Fatal(e)
		}
		conditional := filepath.Join(dir, "node_modules", "luuna-atlas-recipe-conditional")
		if e := os.MkdirAll(conditional, 0755); e != nil {
			t.Fatal(e)
		}
		for file, content := range map[string]string{
			"package.json": `{"name":"luuna-atlas-recipe-conditional","type":"module","exports":{"import":"./import.js","require":"./require.js"}}`,
			"import.js":    `export default {async install(){return {success:true,message:'import entry'}},async check(){return {success:true,message:'checked'}},async remove(){return {success:true,message:'removed'}}}`,
			"require.js":   `export default {async install(){return {success:true,message:'require entry'}},async check(){return {success:true,message:'checked'}},async remove(){return {success:true,message:'removed'}}}`,
		} {
			if e := os.WriteFile(filepath.Join(conditional, file), []byte(content), 0644); e != nil {
				t.Fatal(e)
			}
		}
	})
}
func TestParityExplicitGitHookSetup(t *testing.T) {
	steps := []parityStep{{Args: []string{"init", "--json"}}, {Args: []string{"setup", "--hooks", "--check", "--json"}}, {Args: []string{"setup", "--hooks", "--json"}}, {Args: []string{"setup", "--hooks", "--json"}}, {Args: []string{"setup", "--hooks", "--check", "--json"}}, {Args: []string{"setup", "--hooks", "--remove", "--json"}}, {Args: []string{"setup", "--hooks", "--remove", "--json"}}}
	parityExecute(t, steps, func(dir string) {
		cmd := exec.Command("git", "init", "-q")
		cmd.Dir = dir
		if b, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("git init: %s %v", b, e)
		}
	})
}
func TestParityCompletionBytes(t *testing.T) {
	reference := parityReference(t)
	left, right := parityProjects(t)
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			step := parityStep{Args: []string{"completions", shell}}
			ref := parityRun(t, reference, left, step, false)
			got := CompletionScript(shell)
			if ref.Code != 0 || ref.Err != "" || ref.Out != got {
				t.Fatalf("%s byte mismatch\nreference: %q\ngo: %q\nstderr: %s", shell, ref.Out, got, ref.Err)
			}
		})
	}
	_ = fmt.Sprint(right)
}

func TestParityManifestCustomTypeOrder(t *testing.T) {
	steps := []parityStep{{Args: []string{"prime", "--manifest", "--records-only"}}, {Args: []string{"prime", "--manifest", "--format", "plain", "--records-only"}}, {Args: []string{"prime", "--manifest", "--format", "xml", "--records-only"}}, {Args: []string{"prime", "--manifest", "--json"}}}
	parityExecute(t, steps, func(dir string) {
		if _, _, err := InitStore(dir); err != nil {
			t.Fatal(err)
		}
		if err := AddDomain(dir, "code"); err != nil {
			t.Fatal(err)
		}
		cfg, err := ReadConfig(dir)
		if err != nil {
			t.Fatal(err)
		}
		cfg["custom_types"] = map[string]any{"zulu": map[string]any{"extends": "convention"}, "alpha": map[string]any{"extends": "convention"}}
		if err := WriteConfig(dir, cfg); err != nil {
			t.Fatal(err)
		}
		path, _ := ExpertisePath(dir, "code")
		records := []Record{{"id": "mx-abc001", "type": "zulu", "content": "One", "classification": "foundational", "recorded_at": "2026-01-01T00:00:00.000Z"}, {"id": "mx-abc002", "type": "alpha", "content": "Two", "classification": "foundational", "recorded_at": "2026-01-01T00:00:00.000Z"}, {"id": "mx-abc003", "type": "zulu", "content": "Three", "classification": "foundational", "recorded_at": "2026-01-01T00:00:00.000Z"}}
		if err := WriteRecords(path, records); err != nil {
			t.Fatal(err)
		}
	})
}

func TestParityHookAllowUnknownRecords(t *testing.T) {
	input := `{"tool_name":"Read","tool_input":{"file_path":"src/a.go"},"session_id":"unknown-types"}`
	parityExecute(t, []parityStep{{Args: []string{"hook"}, Input: input}, {Args: []string{"--allow-unknown-types", "hook"}, Input: input}}, func(dir string) {
		if _, _, err := InitStore(dir); err != nil {
			t.Fatal(err)
		}
		if err := AddDomain(dir, "code"); err != nil {
			t.Fatal(err)
		}
		path, _ := ExpertisePath(dir, "code")
		records := []Record{{"id": "mx-abc001", "type": "pattern", "name": "Known anchor", "description": "Use isolated records", "files": []string{"src/a.go"}, "classification": "foundational", "recorded_at": "2026-01-01T00:00:00.000Z"}, {"id": "mx-abc002", "type": "future-type", "description": "Unknown type alongside known data", "classification": "foundational", "recorded_at": "2026-01-01T00:00:00.000Z"}}
		if err := WriteRecords(path, records); err != nil {
			t.Fatal(err)
		}
	})
}
