package atlas

import (
	"path/filepath"
	"regexp"
	"strings"
)

const GuardReason = "luuna-atlas: do not edit .luuna/atlas/expertise/*.jsonl by hand. Use `luuna-atlas record <domain> --type <t> --description ...` to add, `luuna-atlas edit <id>` to change, `luuna-atlas delete <id>` to remove (they lock, validate, and dedup the store)."

var fileToolFields = map[string]string{"Write": "file_path", "Edit": "file_path", "MultiEdit": "file_path", "NotebookEdit": "notebook_path"}
var expertiseFileRE = regexp.MustCompile(`(^|[\\/])\.luuna[\\/]+atlas[\\/]+expertise[\\/]+[^\\/]+\.jsonl$`)
var expertiseMentionRE = regexp.MustCompile(`\.luuna[\\/]+atlas[\\/]+expertise`)
var shellSegmentRE = regexp.MustCompile(`\n|;|&&|\|\||\|`)
var shellRedirectRE = regexp.MustCompile(`(?:^|[^0-9<])(?:&?>>?|[0-9]>>?)\s*("[^"]*"|'[^']*'|[^\s;&|<>]+)`)
var shellAssignRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
var shellInplaceRE = regexp.MustCompile(`^-[a-zA-Z]*i`)

func IsExpertiseFile(path, cwd string) bool {
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	path, _ = filepath.Abs(path)
	return expertiseFileRE.MatchString(path)
}
func unquoteShell(s string) string {
	if strings.HasPrefix(s, "'") || strings.HasPrefix(s, "\"") {
		s = s[1:]
	}
	if strings.HasSuffix(s, "'") || strings.HasSuffix(s, "\"") {
		s = s[:len(s)-1]
	}
	return s
}
func shellSegmentWrites(segment string) bool {
	words := strings.Fields(strings.TrimSpace(segment))
	for j := range words {
		words[j] = unquoteShell(words[j])
	}
	for len(words) > 0 && (shellAssignRE.MatchString(words[0]) || Contains([]string{"sudo", "env", "command", "nohup", "time", "xargs"}, words[0])) {
		words = words[1:]
	}
	name := ""
	args := []string{}
	if len(words) > 0 {
		name = words[0]
		args = words[1:]
	}
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = name[index+1:]
	}
	if name == "luuna-atlas" || Contains(LegacyCommandNames, name) {
		return false
	}
	for _, m := range shellRedirectRE.FindAllStringSubmatch(segment, -1) {
		if expertiseMentionRE.MatchString(unquoteShell(m[1])) {
			return true
		}
	}
	paths := []string{}
	anyExpertise := false
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			paths = append(paths, arg)
			if expertiseMentionRE.MatchString(arg) {
				anyExpertise = true
			}
		}
	}
	if Contains([]string{"tee", "rm", "mv", "truncate", "touch", "unlink", "shred", "sponge"}, name) {
		return anyExpertise
	}
	if Contains([]string{"cp", "ln", "install", "rsync"}, name) {
		return len(paths) > 0 && expertiseMentionRE.MatchString(paths[len(paths)-1])
	}
	if name == "sed" || name == "perl" {
		if !anyExpertise {
			return false
		}
		for _, arg := range args {
			if shellInplaceRE.MatchString(arg) || strings.HasPrefix(arg, "--in-place") {
				return true
			}
		}
	}
	if name == "dd" {
		for _, arg := range args {
			if strings.HasPrefix(arg, "of=") && expertiseMentionRE.MatchString(arg) {
				return true
			}
		}
	}
	return false
}
func BashWritesExpertise(command string) bool {
	if !expertiseMentionRE.MatchString(command) {
		return false
	}
	for _, segment := range shellSegmentRE.Split(command, -1) {
		if shellSegmentWrites(segment) {
			return true
		}
	}
	return false
}
func GuardDecision(input map[string]any, cwd string) string {
	tool := Text(input["tool_name"])
	toolInput := Obj(input["tool_input"])
	if tool == "Bash" {
		if command, ok := toolInput["command"].(string); ok && BashWritesExpertise(command) {
			return GuardReason
		}
		return ""
	}
	key := fileToolFields[tool]
	if key != "" {
		if target, ok := toolInput[key].(string); ok && target != "" && IsExpertiseFile(target, cwd) {
			return GuardReason
		}
	}
	return ""
}
