package atlas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func configSegments(path string) []string {
	out := []string{}
	for _, s := range strings.Split(path, ".") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
func schemaDescend(segments []string) (map[string]any, bool) {
	node := contract.ConfigSchema
	for _, seg := range segments {
		props := Obj(node["properties"])
		if value, ok := props[seg]; ok {
			node = Obj(value)
			continue
		}
		if value, ok := node["additionalProperties"].(map[string]any); ok {
			node = value
			continue
		}
		return nil, false
	}
	return node, true
}
func schemaDefaults(node map[string]any) (any, bool) {
	if value, ok := node["default"]; ok {
		return value, true
	}
	values := map[string]any{}
	for _, key := range ObjectKeys(Obj(node["properties"])) {
		if v, ok := schemaDefaults(Obj(Obj(node["properties"])[key])); ok {
			values[key] = v
		}
	}
	if len(values) == 0 {
		return nil, false
	}
	return values, true
}
func configWalk(cfg any, segments []string) (any, bool) {
	cur := cfg
	for _, s := range segments {
		next, ok := Obj(cur)[s]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}
func validateConfigPath(segments []string) error {
	node := contract.ConfigSchema
	for n, seg := range segments {
		if value, ok := Obj(node["properties"])[seg]; ok {
			node = Obj(value)
			continue
		}
		if value, ok := node["additionalProperties"].(map[string]any); ok {
			node = value
			continue
		}
		known := strings.Join(ObjectKeys(Obj(node["properties"])), ", ")
		if known == "" {
			known = "(none)"
		}
		parent := strings.Join(segments[:n], ".")
		if parent == "" {
			parent = "<root>"
		}
		return fmt.Errorf("Unknown config path: '%s' is not a known knob. Known keys at '%s': %s.", strings.Join(segments[:n+1], "."), parent, known)
	}
	return nil
}
func (a *App) configPlainError(err error) error {
	fmt.Fprintln(a.Err, err.Error())
	a.ExitCode = 1
	return nil
}
func (a *App) runConfigShow(i Invocation) error {
	cfg, err := ReadConfig(a.Cwd)
	if err != nil {
		return a.configPlainError(err)
	}
	if !i.Has("path") {
		a.Emit(cfg)
		return nil
	}
	segments := configSegments(i.String("path"))
	if len(segments) == 0 {
		return a.configPlainError(fmt.Errorf("--path must not be empty."))
	}
	if value, ok := configWalk(cfg, segments); ok {
		a.Emit(value)
		return nil
	}
	if node, ok := schemaDescend(segments); ok {
		if value, has := schemaDefaults(node); has {
			a.Emit(value)
			return nil
		}
	}
	return a.configPlainError(fmt.Errorf("Path '%s' not found in config and has no schema default.", i.String("path")))
}
func (a *App) runConfigSet(i Invocation) error   { return a.configMutate(i, false) }
func (a *App) runConfigUnset(i Invocation) error { return a.configMutate(i, true) }
func (a *App) configMutate(i Invocation, unset bool) error {
	path := i.Positionals[0]
	segments := configSegments(path)
	if len(segments) == 0 {
		return a.configPlainError(fmt.Errorf("<path> must not be empty."))
	}
	file, err := ConfigPath(a.Cwd)
	if err != nil {
		return a.configPlainError(err)
	}
	action := "set"
	if unset {
		action = "unset"
	}
	if !exists(file) {
		return a.configPlainError(fmt.Errorf("No .luuna/atlas/ directory found. Run `luuna-atlas init` to set up this project before `luuna-atlas config %s`.", action))
	}
	if err = validateConfigPath(segments); err != nil {
		return a.configPlainError(err)
	}
	var value any
	if !unset {
		value, err = YAMLValue(i.Positionals[1])
		if err != nil {
			return a.configPlainError(fmt.Errorf("Invalid YAML for <value>: %s", err))
		}
	}
	err = WithFileLock(file, func() error {
		cfg, e := ReadConfig(a.Cwd)
		if e != nil {
			return e
		}
		cur := map[string]any(cfg)
		ancestors := []map[string]any{cur}
		for _, seg := range segments[:len(segments)-1] {
			next, ok := cur[seg].(map[string]any)
			if !ok {
				if unset {
					return nil
				}
				next = map[string]any{}
				cur[seg] = next
			}
			cur = next
			ancestors = append(ancestors, cur)
		}
		leaf := segments[len(segments)-1]
		if unset {
			if _, ok := cur[leaf]; !ok {
				return nil
			}
			delete(cur, leaf)
			for n := len(ancestors) - 1; n >= 1; n-- {
				if len(ancestors[n]) > 0 {
					break
				}
				parentNode, _ := schemaDescend(segments[:n-1])
				if Contains(List(parentNode["required"]), segments[n-1]) {
					break
				}
				delete(ancestors[n-1], segments[n-1])
			}
		} else {
			cur[leaf] = value
		}
		issues := ValidateJSONSchema(cfg, contract.ConfigSchema)
		if len(issues) > 0 {
			lines := []string{}
			for _, issue := range issues {
				p := issue.Path
				if p == "" {
					p = "<root>"
				}
				tail := ""
				parts := strings.Split(strings.TrimPrefix(issue.Path, "/"), "/")
				if node, ok := schemaDescend(parts); ok && Text(node["title"]) != "" {
					tail = " (" + Text(node["title"]) + ")"
				}
				lines = append(lines, "  - "+p+": "+issue.Message+tail)
			}
			return fmt.Errorf("Invalid config after %s:\n%s", action, strings.Join(lines, "\n"))
		}
		data, e := MarshalYAML(cfg)
		if e != nil {
			return e
		}
		return parentAtomicWrite(file, data)
	})
	if err != nil {
		return a.configPlainError(err)
	}
	return nil
}
func parentAtomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return RenameReplacing(name, path)
}
