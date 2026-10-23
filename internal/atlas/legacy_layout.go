// Legacy pre-Luuna names and one-way migration live only in this module.
package atlas

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const (
	LegacyStoreDir           = ".mulch"
	LegacyConfigFile         = "mulch.config.yaml"
	LegacyMissionsDir        = ".seeds"
	LegacyEvidenceKey        = "seeds"
	LegacyMarkerStart        = "<!-- mulch:start -->"
	LegacyMarkerEnd          = "<!-- mulch:end -->"
	LegacyHookCommandPattern = `^\s*(ml|mulch)\s+(prime|hook)(\s|$)`
)

var LegacyCommandNames = []string{"ml", "mulch"}

func HasLegacyStore(root string) bool {
	st, err := os.Lstat(filepath.Join(root, LegacyStoreDir))
	return err == nil && (st.IsDir() || st.Mode()&os.ModeSymlink != 0)
}

func MigrateLegacyStore(root string) error {
	old := filepath.Join(root, LegacyStoreDir)
	oldStat, e := os.Lstat(old)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	luuna := filepath.Join(root, ".luuna")
	target := filepath.Join(luuna, "atlas")
	if oldStat.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Refusing to migrate .mulch: it is a symbolic link. Move its contents to .luuna/atlas manually.")
	}
	if !oldStat.IsDir() {
		return nil
	}
	if s, e := os.Lstat(luuna); e == nil && s.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Refusing to migrate .mulch: .luuna is a symbolic link. Replace it with a real directory first.")
	}
	if lexists(target) {
		return fmt.Errorf("Both .mulch/ and .luuna/atlas/ exist. Resolve this by hand: keep .luuna/atlas/ and remove or archive .mulch/ (or the reverse), then retry.")
	}
	if e = os.MkdirAll(luuna, 0777); e != nil {
		return e
	}
	if e = os.Rename(old, target); e != nil {
		return e
	}
	oldCfg := filepath.Join(target, LegacyConfigFile)
	if lexists(oldCfg) && !lexists(filepath.Join(target, "config.yaml")) {
		if e = os.Rename(oldCfg, filepath.Join(target, "config.yaml")); e != nil {
			return e
		}
	}
	for _, name := range []string{".gitattributes", ".gitignore"} {
		p := filepath.Join(root, name)
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		re := regexp.MustCompile(`(?m)^(\s*/?)\.mulch/`)
		next := re.ReplaceAllString(string(b), "${1}.luuna/atlas/")
		if next != string(b) {
			if e = os.WriteFile(p, []byte(next), 0666); e != nil {
				return e
			}
		}
	}
	fmt.Fprintln(os.Stderr, "luuna-atlas: moved legacy .mulch/ to .luuna/atlas/ (mulch.config.yaml is now config.yaml)")
	return nil
}
func ResolveMissionsDir(cwd string) string {
	current := filepath.Join(cwd, ".luuna", "missions")
	if lexists(current) {
		return current
	}
	legacy := filepath.Join(cwd, LegacyMissionsDir)
	if lexists(legacy) {
		return legacy
	}
	return current
}
func EvidenceMissions(evidence any) string {
	ev := Obj(evidence)
	value, ok := ev["missions"]
	if !ok || value == nil {
		value = ev[LegacyEvidenceKey]
	}
	if s, ok := value.(string); ok && s != "" {
		return s
	}
	return ""
}
