package atlas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

type RecipeResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
type providerRecipe struct{ Name, Source, Path, ShadowedBy string }

var builtinProviderNames = []string{"claude", "codex", "cursor"}

const codexTOMLStart = "# luuna-atlas:start — managed by `luuna-atlas setup codex`"
const codexTOMLEnd = "# luuna-atlas:end"
const codexTOMLBlock = codexTOMLStart + "\n[features]\ncodex_hooks = true\n\n[[hooks.SessionStart]]\n\n[[hooks.SessionStart.hooks]]\ntype = \"command\"\ncommand = \"luuna-atlas prime\"\nstatusMessage = \"Loading luuna-atlas expertise\"\n" + codexTOMLEnd

var atlasHandlerRE = regexp.MustCompile(`^\s*luuna-atlas\s+(prime|hook)(\s|$)|` + LegacyHookCommandPattern)

func setupRead(path string) string { b, _ := os.ReadFile(path); return string(b) }
func setupExists(path string) bool { _, e := os.Stat(path); return e == nil }
func setupWrite(path, content string) error {
	if e := os.MkdirAll(filepath.Dir(path), 0777); e != nil {
		return e
	}
	return os.WriteFile(path, []byte(content), 0666)
}
func setupFooterAsset(key string, cfg Config) string {
	return strings.ReplaceAll(formatAssets[key], formatAssets["close_conditional_embedded"], SessionEndReminder("embedded", cfg))
}
func recipeOK(message string) RecipeResult   { return RecipeResult{true, message} }
func recipeFail(message string) RecipeResult { return RecipeResult{false, message} }
func installGitHook(cwd, action string) (RecipeResult, error) {
	gitDir := filepath.Join(cwd, ".git")
	path := filepath.Join(gitDir, "hooks", "pre-commit")
	content := setupRead(path)
	switch action {
	case "check":
		if !setupExists(path) {
			return recipeFail("Git pre-commit hook not found."), nil
		}
		if !strings.Contains(content, "# luuna-atlas:start") {
			return recipeFail("Git pre-commit hook exists but has no luuna-atlas section."), nil
		}
		return recipeOK("Git pre-commit hook is installed."), nil
	case "remove":
		if !setupExists(path) {
			return recipeOK("Git pre-commit hook not found; nothing to remove."), nil
		}
		if !strings.Contains(content, "# luuna-atlas:start") {
			return recipeOK("No luuna-atlas section in pre-commit hook; nothing to remove."), nil
		}
		start, end := strings.Index(content, "# luuna-atlas:start"), strings.Index(content, "# luuna-atlas:end")
		if end < 0 {
			end = -1
		}
		cut := end + len("# luuna-atlas:end")
		if cut > len(content) {
			cut = len(content)
		}
		cleaned := strings.TrimSpace(extraNewlines.ReplaceAllString(content[:start]+content[cut:], "\n\n"))
		if cleaned == "" || cleaned == "#!/bin/sh" {
			e := os.Remove(path)
			return recipeOK("Removed luuna-atlas pre-commit hook (file deleted)."), e
		}
		e := setupWrite(path, cleaned+"\n")
		return recipeOK("Removed luuna-atlas section from pre-commit hook."), e
	default:
		if !setupExists(gitDir) {
			return recipeFail("Not a git repository — .git directory not found."), nil
		}
		if strings.Contains(content, "# luuna-atlas:start") {
			return recipeOK("Git pre-commit hook already installed."), nil
		}
		if content != "" {
			content = strings.TrimRight(content, " \r\n\t") + "\n\n" + formatAssets["git_hook"] + "\n"
		} else {
			content = "#!/bin/sh\n\n" + formatAssets["git_hook"] + "\n"
		}
		if e := setupWrite(path, content); e != nil {
			return RecipeResult{}, e
		}
		if e := os.Chmod(path, 0755); e != nil {
			return RecipeResult{}, e
		}
		return recipeOK("Installed luuna-atlas pre-commit git hook."), nil
	}
}

type claudeHookSpec struct{ Event, Matcher, Command string }

var claudeHookSpecs = []claudeHookSpec{{"SessionStart", "", "luuna-atlas prime"}, {"PreToolUse", "Read|Edit|Write|MultiEdit|NotebookEdit|Bash", "luuna-atlas hook"}}

func claudeGroup(spec claudeHookSpec) map[string]any {
	handler := map[string]any{"type": "command", "command": spec.Command}
	rememberOrder(handler, []string{"type", "command"})
	group := map[string]any{"matcher": spec.Matcher, "hooks": []any{handler}}
	rememberOrder(group, []string{"matcher", "hooks"})
	return group
}
func isAtlasHandler(v any) bool { return atlasHandlerRE.MatchString(Text(Obj(v)["command"])) }
func claudeHooksObject(settings map[string]any) (map[string]any, error) {
	v, ok := settings["hooks"]
	if !ok {
		return map[string]any{}, nil
	}
	switch v.(type) {
	case map[string]any, Record, Config:
		return Obj(v), nil
	}
	return nil, fmt.Errorf("`hooks` in Claude settings is not an object; nothing was written")
}
func claudeEventGroups(hooks map[string]any, event string) ([]any, error) {
	v, ok := hooks[event]
	if !ok {
		return []any{}, nil
	}
	if _, ok := v.([]any); !ok {
		return nil, fmt.Errorf("`hooks.%s` in Claude settings is not an array; nothing was written", event)
	}
	return AnyList(v), nil
}
func claudeInstalled(groups []any, spec claudeHookSpec) bool {
	ours := []any{}
	for _, g := range groups {
		for _, h := range AnyList(Obj(g)["hooks"]) {
			if isAtlasHandler(h) {
				ours = append(ours, g)
				break
			}
		}
	}
	return len(ours) == 1 && reflect.DeepEqual(Obj(ours[0]), claudeGroup(spec))
}
func readClaudeSettings(path string) (map[string]any, error) {
	if !setupExists(path) {
		return nil, nil
	}
	raw := setupRead(path)
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	v, e := ParseJSON([]byte(raw))
	if e != nil {
		return nil, fmt.Errorf("Failed to parse Claude settings at %s: %s", path, e)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Claude settings at %s is not a JSON object", path)
	}
	return m, nil
}
func syncClaudeSettings(cwd string, remove bool) (*bool, error) {
	path := filepath.Join(cwd, ".claude", "settings.json")
	settings, e := readClaudeSettings(path)
	if e != nil {
		return nil, e
	}
	if settings == nil {
		if remove {
			return nil, nil
		}
		settings = map[string]any{}
	}
	oldHooks, e := claudeHooksObject(settings)
	if e != nil {
		return nil, e
	}
	hooks := map[string]any{}
	for k, v := range oldHooks {
		hooks[k] = v
	}
	hookOrder := ObjectKeys(oldHooks)
	events := []string{}
	if remove {
		events = ObjectKeys(hooks)
	} else {
		for _, s := range claudeHookSpecs {
			events = append(events, s.Event)
		}
	}
	changed := false
	for _, event := range events {
		groups, e := claudeEventGroups(hooks, event)
		if e != nil {
			return nil, e
		}
		var spec *claudeHookSpec
		for j := range claudeHookSpecs {
			if claudeHookSpecs[j].Event == event {
				spec = &claudeHookSpecs[j]
				break
			}
		}
		if !remove && spec != nil && claudeInstalled(groups, *spec) {
			continue
		}
		updated := []any{}
		for _, g := range groups {
			handlers := AnyList(Obj(g)["hooks"])
			kept := []any{}
			for _, h := range handlers {
				if !isAtlasHandler(h) {
					kept = append(kept, h)
				}
			}
			if len(kept) == len(handlers) {
				updated = append(updated, g)
			} else if len(kept) > 0 {
				copy := map[string]any{}
				for k, v := range Obj(g) {
					copy[k] = v
				}
				copy["hooks"] = kept
				rememberOrder(copy, ObjectKeys(g))
				updated = append(updated, copy)
			}
		}
		if !remove && spec != nil {
			updated = append(updated, claudeGroup(*spec))
		}
		if reflect.DeepEqual(updated, groups) {
			continue
		}
		changed = true
		if len(updated) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = updated
			if !Contains(hookOrder, event) {
				hookOrder = append(hookOrder, event)
			}
		}
	}
	rememberOrder(hooks, hookOrder)
	if changed {
		if len(hooks) == 0 {
			delete(settings, "hooks")
		} else {
			settings["hooks"] = hooks
		}
		if e := setupWrite(path, JSONPretty(settings)+"\n"); e != nil {
			return nil, e
		}
	}
	return &changed, nil
}
func runClaudeRecipe(cwd, action string) (RecipeResult, error) {
	if action == "check" {
		settings, e := readClaudeSettings(filepath.Join(cwd, ".claude", "settings.json"))
		if e != nil {
			return RecipeResult{}, e
		}
		if settings == nil {
			return recipeFail("Claude settings.json not found."), nil
		}
		hooks, e := claudeHooksObject(settings)
		if e != nil {
			return RecipeResult{}, e
		}
		missing := []string{}
		for _, spec := range claudeHookSpecs {
			groups, e := claudeEventGroups(hooks, spec.Event)
			if e != nil {
				return RecipeResult{}, e
			}
			if !claudeInstalled(groups, spec) {
				missing = append(missing, spec.Event)
			}
		}
		if len(missing) > 0 {
			return recipeFail("Missing hooks for: " + strings.Join(missing, ", ") + "."), nil
		}
		return recipeOK("Claude hooks are installed and correct."), nil
	}
	changed, e := syncClaudeSettings(cwd, action == "remove")
	if e != nil {
		return RecipeResult{}, e
	}
	if action == "remove" {
		if changed == nil {
			return recipeOK("Claude settings.json not found; nothing to remove."), nil
		}
		if *changed {
			return recipeOK("Removed luuna-atlas hooks from Claude settings."), nil
		}
		return recipeOK("No luuna-atlas hooks found in Claude settings."), nil
	}
	if changed != nil && *changed {
		return recipeOK("Installed Claude hooks: SessionStart `luuna-atlas prime`, PreToolUse `luuna-atlas hook` (file-anchored records + .luuna/atlas/expertise guard)."), nil
	}
	return recipeOK("Claude hooks already installed."), nil
}
func runCursorRecipe(cwd, action string) (RecipeResult, error) {
	path := filepath.Join(cwd, ".cursor", "rules", "luuna-atlas.mdc")
	cfg, _ := ReadConfig(cwd)
	expected := setupFooterAsset("cursor", cfg)
	switch action {
	case "check":
		if !setupExists(path) {
			return recipeFail("Cursor rule file not found."), nil
		}
		if setupRead(path) != expected {
			return recipeFail("Cursor rule file exists but has been modified."), nil
		}
		return recipeOK("Cursor rule is installed and correct."), nil
	case "remove":
		if !setupExists(path) {
			return recipeOK("Cursor rule not found; nothing to remove."), nil
		}
		e := os.Remove(path)
		return recipeOK("Removed Cursor rule file."), e
	default:
		if setupExists(path) && setupRead(path) == expected {
			return recipeOK("Cursor rule already installed."), nil
		}
		e := setupWrite(path, expected)
		return recipeOK("Installed Cursor rule at .cursor/rules/luuna-atlas.mdc."), e
	}
}
func removeCodexTOML(content string) string {
	start := strings.Index(content, codexTOMLStart)
	if start < 0 {
		return content
	}
	end := strings.Index(content[start:], codexTOMLEnd)
	if end < 0 {
		return content
	}
	end += start
	cut := len(content)
	if newline := strings.Index(content[end:], "\n"); newline >= 0 {
		cut = end + newline + 1
	}
	clean := strings.TrimRight(extraNewlines.ReplaceAllString(content[:start]+content[cut:], "\n\n"), " \r\n\t")
	if clean != "" {
		clean += "\n"
	}
	return clean
}
func runCodexRecipe(cwd, action string) (RecipeResult, error) {
	agents, toml := filepath.Join(cwd, "AGENTS.md"), filepath.Join(cwd, ".codex", "config.toml")
	ac, tc := setupRead(agents), setupRead(toml)
	aa, ta := HasMarkerSection(ac), strings.Contains(tc, codexTOMLStart)
	switch action {
	case "check":
		switch {
		case !setupExists(agents):
			return recipeFail("AGENTS.md not found."), nil
		case !aa:
			return recipeFail("AGENTS.md exists but has no luuna-atlas section."), nil
		case !setupExists(toml):
			return recipeFail(".codex/config.toml not found."), nil
		case !ta:
			return recipeFail(".codex/config.toml exists but has no luuna-atlas SessionStart hook."), nil
		}
		return recipeOK("Codex integration installed (AGENTS.md + .codex/config.toml)."), nil
	case "remove":
		removed := []string{}
		if aa {
			if e := setupWrite(agents, RemoveMarkerSection(ac)); e != nil {
				return RecipeResult{}, e
			}
			removed = append(removed, "AGENTS.md luuna-atlas section")
		}
		if ta {
			if e := setupWrite(toml, removeCodexTOML(tc)); e != nil {
				return RecipeResult{}, e
			}
			removed = append(removed, ".codex/config.toml SessionStart hook")
		}
		if len(removed) == 0 {
			return recipeOK("No Codex integration found; nothing to remove."), nil
		}
		return recipeOK("Removed Codex integration: " + strings.Join(removed, ", ") + "."), nil
	default:
		installed := []string{}
		if !aa {
			cfg, _ := ReadConfig(cwd)
			section := setupFooterAsset("codex", cfg)
			next := section + "\n"
			if ac != "" {
				next = strings.TrimRight(ac, " \r\n\t") + "\n\n" + section + "\n"
			}
			if e := setupWrite(agents, next); e != nil {
				return RecipeResult{}, e
			}
			installed = append(installed, "AGENTS.md luuna-atlas section")
		}
		if !ta {
			next := codexTOMLBlock + "\n"
			if strings.TrimSpace(tc) != "" {
				next = strings.TrimRight(tc, " \r\n\t") + "\n\n" + codexTOMLBlock + "\n"
			}
			if e := setupWrite(toml, next); e != nil {
				return RecipeResult{}, e
			}
			installed = append(installed, ".codex/config.toml SessionStart hook")
		}
		if len(installed) == 0 {
			return recipeOK("Codex integration already installed (AGENTS.md + .codex/config.toml)."), nil
		}
		return recipeOK("Installed Codex integration: " + strings.Join(installed, ", ") + "."), nil
	}
}

func findNpmRecipe(name, cwd string) string {
	// npm recipes are JavaScript extensions. Use their runtime's module resolver
	// so package exports, conditions and symlinks retain their existing meaning.
	// No Atlas implementation is imported or executed by this extension probe.
	const resolver = `import {createRequire} from 'node:module';try{process.stdout.write(createRequire(import.meta.url).resolve(process.env.LUUNA_ATLAS_RECIPE_PACKAGE,{paths:[process.cwd()]}))}catch{process.exit(1)}`
	cmd := exec.Command("bun", "-e", resolver)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "LUUNA_ATLAS_RECIPE_PACKAGE=luuna-atlas-recipe-"+name)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
func resolveProvider(name, cwd string) *providerRecipe {
	for _, extension := range []string{"ts", "sh"} {
		path := filepath.Join(cwd, ".luuna", "atlas", "recipes", name+"."+extension)
		if setupExists(path) {
			return &providerRecipe{Name: name, Source: "filesystem-" + extension, Path: path}
		}
	}
	if path := findNpmRecipe(name, cwd); path != "" {
		return &providerRecipe{Name: name, Source: "npm", Path: path}
	}
	if Contains(builtinProviderNames, name) {
		return &providerRecipe{Name: name, Source: "builtin"}
	}
	return nil
}
func providerListings(cwd string) []providerRecipe {
	fs := []providerRecipe{}
	entries, _ := os.ReadDir(filepath.Join(cwd, ".luuna", "atlas", "recipes"))
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if ext == ".ts" || ext == ".sh" {
			fs = append(fs, providerRecipe{Name: strings.TrimSuffix(e.Name(), ext), Source: "filesystem-" + ext[1:], Path: filepath.Join(cwd, ".luuna", "atlas", "recipes", e.Name())})
		}
	}
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].Name < fs[j].Name })
	out := []providerRecipe{}
	for _, name := range builtinProviderNames {
		shadow := ""
		for _, f := range fs {
			if f.Name == name {
				shadow = f.Source
				break
			}
		}
		if shadow == "" && findNpmRecipe(name, cwd) != "" {
			shadow = "npm"
		}
		out = append(out, providerRecipe{Name: name, Source: "builtin", ShadowedBy: shadow})
	}
	out = append(out, fs...)
	order := map[string]int{"filesystem-ts": 0, "filesystem-sh": 1, "npm": 2, "builtin": 3}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return order[out[i].Source] < order[out[j].Source]
	})
	return out
}

const recipeSentinel = "\x1eLUUNA_ATLAS_RECIPE_RESULT:"
const recipeRunner = `import {pathToFileURL} from 'node:url';let result;try{const mod=await import(pathToFileURL(process.env.LUUNA_ATLAS_RECIPE_PATH).href);const name=process.env.LUUNA_ATLAS_RECIPE_NAME;const source=process.env.LUUNA_ATLAS_RECIPE_SOURCE;const label=source==='npm'?'Recipe package "luuna-atlas-recipe-'+name+'"':'Recipe "'+name+'" at '+process.env.LUUNA_ATLAS_RECIPE_PATH;if(mod.default===undefined){result={resolveError:label+' has no default export. ProviderRecipes must be exposed via \u0060export default { install, check, remove }\u0060; named exports alone are not accepted.'+(source==='npm'?'':' See examples/recipes/internal-ide.ts for a template.')}}else if(!mod.default||typeof mod.default!=='object'||!['install','check','remove'].every(k=>typeof mod.default[k]==='function')){result={resolveError:label+' default export is not a valid ProviderRecipe (must have async install/check/remove functions).'+(source==='npm'?'':' See examples/recipes/internal-ide.ts for a template.')}}else{try{result={result:await mod.default[process.env.LUUNA_ATLAS_RECIPE_ACTION](process.cwd())}}catch(e){result={threw:e instanceof Error?e.message:String(e)}}}}catch(e){result={resolveError:e instanceof Error?e.message:String(e)}}process.stdout.write('\x1eLUUNA_ATLAS_RECIPE_RESULT:'+JSON.stringify(result));`

func (a *App) runExternalRecipe(r providerRecipe, action string) (RecipeResult, error) {
	if r.Source == "filesystem-sh" {
		argv := []string{action}
		exe := r.Path
		if runtime.GOOS == "windows" {
			exe = "sh"
			argv = []string{r.Path, action}
		}
		cmd := exec.Command(exe, argv...)
		cmd.Dir = a.Cwd
		cmd.Env = append(os.Environ(), "LUUNA_ATLAS_RECIPE_NAME="+r.Name, "LUUNA_ATLAS_RECIPE_ACTION="+action)
		var out, errOut bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errOut
		e := cmd.Run()
		ok := e == nil
		message := strings.TrimSpace(out.String())
		if message == "" {
			if ok {
				message = r.Name + " " + action + " succeeded"
			} else {
				code := 1
				if cmd.ProcessState != nil {
					code = cmd.ProcessState.ExitCode()
				}
				message = fmt.Sprintf("%s %s exited with code %d", r.Name, action, code)
				if s := strings.TrimSpace(errOut.String()); s != "" {
					message += ": " + s
				}
			}
		}
		return RecipeResult{ok, message}, nil
	}
	cmd := exec.Command("bun", "-e", recipeRunner)
	cmd.Dir = a.Cwd
	cmd.Env = append(os.Environ(), "LUUNA_ATLAS_RECIPE_PATH="+r.Path, "LUUNA_ATLAS_RECIPE_NAME="+r.Name, "LUUNA_ATLAS_RECIPE_ACTION="+action, "LUUNA_ATLAS_RECIPE_SOURCE="+r.Source)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = a.Err
	if e := cmd.Run(); e != nil {
		return RecipeResult{}, e
	}
	raw := out.String()
	index := strings.LastIndex(raw, recipeSentinel)
	if index < 0 {
		return RecipeResult{}, fmt.Errorf("recipe \"%s\" returned no result", r.Name)
	}
	if index > 0 {
		fmt.Fprint(a.Out, raw[:index])
	}
	var payload map[string]any
	if e := json.Unmarshal([]byte(raw[index+len(recipeSentinel):]), &payload); e != nil {
		return RecipeResult{}, e
	}
	if s := Text(payload["resolveError"]); s != "" {
		return RecipeResult{}, fmt.Errorf("%s", s)
	}
	if s := Text(payload["threw"]); s != "" {
		return recipeFail(fmt.Sprintf("recipe \"%s\" %s threw (%s): %s", r.Name, action, r.Path, s)), nil
	}
	result := Obj(payload["result"])
	return RecipeResult{Bool(result["success"]), Text(result["message"])}, nil
}
func (a *App) runSetup(i Invocation) error {
	dir, e := AtlasDir(a.Cwd)
	if e != nil {
		return e
	}
	if !setupExists(dir) {
		a.Error("setup", "No .luuna/atlas/ directory found. Run `luuna-atlas init` first.")
		return nil
	}
	if i.Bool("list") {
		providers := providerListings(a.Cwd)
		if a.JSONMode {
			items := []any{}
			for _, r := range providers {
				m := map[string]any{"name": r.Name, "source": r.Source}
				if r.Path != "" {
					m["path"], _ = filepath.Rel(a.Cwd, r.Path)
				}
				if r.ShadowedBy != "" {
					m["shadowed_by"] = r.ShadowedBy
				}
				rememberOrder(m, []string{"name", "source", "path", "shadowed_by"})
				items = append(items, m)
			}
			fields := map[string]any{"action": "list", "providers": items}
			rememberOrder(fields, []string{"success", "command", "action", "providers"})
			a.OK("setup", fields)
			return nil
		}
		fmt.Fprintln(a.Out, Style("bold", "Available providers:"))
		width := 6
		for _, r := range providers {
			if len(r.Name) > width {
				width = len(r.Name)
			}
		}
		for _, r := range providers {
			label := "built-in"
			marker := "✓"
			if r.ShadowedBy != "" {
				label = Style("dim", label+" (shadowed by "+r.ShadowedBy+")")
				marker = Style("dim", "·")
			} else {
				marker = Style("green", marker)
			}
			if r.Source != "builtin" {
				rel, _ := filepath.Rel(a.Cwd, r.Path)
				label = r.Source + ": " + rel
			}
			fmt.Fprintf(a.Out, "  %s %-*s  %s\n", marker, width, r.Name, label)
		}
		fmt.Fprintln(a.Out)
		fmt.Fprintln(a.Out, Style("dim", "Resolution order: filesystem (.luuna/atlas/recipes/<name>.{ts,sh}) → npm (luuna-atlas-recipe-<name>) → built-in."))
		return nil
	}
	provider := ""
	if len(i.Positionals) > 0 {
		provider = i.Positionals[0]
	}
	if provider == "" && !i.Bool("hooks") {
		message := "Specify a provider, use --hooks, or use --list."
		if !a.JSONMode {
			message = "specify a provider, use --hooks, or use --list."
		}
		a.Error("setup", message)
		return nil
	}
	action := "install"
	if i.Bool("check") {
		action = "check"
	} else if i.Bool("remove") {
		action = "remove"
	}
	if i.Bool("hooks") {
		result, e := installGitHook(a.Cwd, action)
		if e != nil {
			return e
		}
		if a.JSONMode {
			fields := map[string]any{"success": result.Success, "command": "setup", "target": "hooks", "action": action, "message": result.Message}
			rememberOrder(fields, []string{"success", "command", "target", "action", "message"})
			a.Emit(fields)
		} else if result.Success {
			fmt.Fprintln(a.Out, Style("green", "✔ "+result.Message))
		} else {
			fmt.Fprintln(a.Err, Style("red", "✖ "+result.Message))
		}
		if !result.Success {
			a.ExitCode = 1
		}
		if provider == "" {
			return nil
		}
	}
	r := resolveProvider(provider, a.Cwd)
	if r == nil {
		a.Error("setup", fmt.Sprintf("Unknown provider \"%s\". Run `luuna-atlas setup --list` to see discovered providers, or add a recipe at .luuna/atlas/recipes/%s.{ts,sh} or install luuna-atlas-recipe-%s.", provider, provider, provider))
		return nil
	}
	var result RecipeResult
	if r.Source != "builtin" {
		result, e = a.runExternalRecipe(*r, action)
		if e != nil {
			a.Error("setup", e.Error())
			return nil
		}
	} else {
		switch provider {
		case "claude":
			result, e = runClaudeRecipe(a.Cwd, action)
		case "cursor":
			result, e = runCursorRecipe(a.Cwd, action)
		case "codex":
			result, e = runCodexRecipe(a.Cwd, action)
		}
		if e != nil {
			result = recipeFail(fmt.Sprintf("recipe \"%s\" %s threw (built-in): %s", provider, action, e.Error()))
		}
	}
	if a.JSONMode {
		fields := map[string]any{"success": result.Success, "command": "setup", "provider": provider, "source": r.Source, "action": action, "message": result.Message}
		if r.Path != "" {
			fields["path"] = r.Path
		}
		rememberOrder(fields, []string{"success", "command", "provider", "source", "path", "action", "message"})
		a.Emit(fields)
	} else if result.Success {
		fmt.Fprintln(a.Out, Style("green", "✔ "+result.Message))
	} else if action == "check" {
		fmt.Fprintln(a.Out, Style("yellow", "✖ "+result.Message))
	} else {
		fmt.Fprintln(a.Err, Style("red", "Error: "+result.Message))
	}
	if !result.Success {
		a.ExitCode = 1
	}
	return nil
}
