package atlas

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const MarkerStart = "<!-- luuna-atlas:start -->"
const MarkerEnd = "<!-- luuna-atlas:end -->"

var markerPairs = [][2]string{{MarkerStart, MarkerEnd}, {LegacyMarkerStart, LegacyMarkerEnd}}
var extraNewlines = regexp.MustCompile(`\n{3,}`)

func HasMarkerSection(content string) bool {
	for _, p := range markerPairs {
		if strings.Contains(content, p[0]) {
			return true
		}
	}
	return false
}
func ReplaceMarkerSection(content, section string) (string, bool) {
	for _, p := range markerPairs {
		start, end := strings.Index(content, p[0]), strings.Index(content, p[1])
		if start >= 0 && end >= 0 {
			return content[:start] + section + content[end+len(p[1]):], true
		}
	}
	return content, false
}
func RemoveMarkerSection(content string) string {
	if next, ok := ReplaceMarkerSection(content, ""); ok {
		return strings.TrimSpace(extraNewlines.ReplaceAllString(next, "\n\n")) + "\n"
	}
	return content
}
func WrapInMarkers(snippet string) string { return MarkerStart + "\n" + snippet + MarkerEnd }
func onboardSnippet(cfg Config) string {
	return strings.ReplaceAll(formatAssets["onboard"], "__ATLAS_SESSION_CLOSE__", SessionEndReminder("embedded", cfg))
}

type onboardTarget struct {
	Name, Path string
	Exists     bool
}

func resolveOnboardTarget(cwd string) (onboardTarget, []onboardTarget) {
	candidates := []onboardTarget{{"CLAUDE.md", filepath.Join(cwd, "CLAUDE.md"), false}, {".claude/CLAUDE.md", filepath.Join(cwd, ".claude", "CLAUDE.md"), false}, {"AGENTS.md", filepath.Join(cwd, "AGENTS.md"), false}}
	found := []onboardTarget{}
	for _, c := range candidates {
		b, e := os.ReadFile(c.Path)
		if e != nil {
			continue
		}
		c.Exists = true
		if HasMarkerSection(string(b)) || strings.Contains(string(b), "## Project Expertise (Atlas)") {
			found = append(found, c)
		}
	}
	if len(found) > 0 {
		return found[0], found[1:]
	}
	for _, index := range []int{0, 2} {
		c := candidates[index]
		if _, e := os.Stat(c.Path); e == nil {
			c.Exists = true
			return c, nil
		}
	}
	return candidates[0], nil
}
func replaceLegacyOnboard(content, newSection string) string {
	const header = "## Project Expertise (Atlas)"
	const tail = "luuna-atlas validate && git add .luuna/atlas/ && git commit -m \"luuna-atlas: record learnings\""
	start := strings.Index(content, header)
	if start < 0 {
		return content
	}
	end := len(content)
	if rel := strings.Index(content[start:], tail); rel >= 0 {
		tailEnd := start + rel + len(tail)
		if fence := strings.Index(content[tailEnd:], "```"); fence >= 0 {
			end = tailEnd + fence + 3
			for end < len(content) && content[end] == '\n' {
				end++
			}
		}
	}
	return content[:start] + newSection + content[end:]
}
func (a *App) runOnboard(i Invocation) error {
	cfg, _ := ReadConfig(a.Cwd)
	snippet := WrapInMarkers(onboardSnippet(cfg))
	if i.Bool("stdout") {
		_, e := fmt.Fprint(a.Out, snippet)
		return e
	}
	target, duplicates := resolveOnboardTarget(a.Cwd)
	b, _ := os.ReadFile(target.Path)
	content := string(b)
	action := ""
	current := HasMarkerSection(content) && strings.Contains(content, "<!-- atlas-onboard:v"+Version+" -->")
	if i.Bool("check") {
		action = "not_installed"
		if target.Exists {
			if HasMarkerSection(content) {
				action = "outdated"
				if current {
					action = "up_to_date"
				}
			} else if strings.Contains(content, "## Project Expertise (Atlas)") {
				action = "legacy"
			}
		}
	} else {
		next := content
		switch {
		case !target.Exists:
			action = "created"
			next = snippet + "\n"
		case HasMarkerSection(content):
			if current {
				action = "up_to_date"
			} else {
				action = "updated"
				next, _ = ReplaceMarkerSection(content, snippet)
			}
		case strings.Contains(content, "## Project Expertise (Atlas)"):
			action = "migrated"
			next = replaceLegacyOnboard(content, snippet+"\n")
		default:
			action = "appended"
			next = strings.TrimRight(content, " \r\n\t") + "\n\n" + snippet + "\n"
		}
		if action != "up_to_date" {
			if e := os.MkdirAll(filepath.Dir(target.Path), 0777); e != nil {
				return e
			}
			if e := os.WriteFile(target.Path, []byte(next), 0666); e != nil {
				return e
			}
		}
	}
	if a.JSONMode {
		fields := map[string]any{"file": target.Name, "action": action}
		rememberOrder(fields, []string{"success", "command", "file", "action"})
		a.OK("onboard", fields)
	} else {
		message := ""
		if i.Bool("check") {
			switch action {
			case "not_installed":
				message = "Atlas snippet is not installed in " + target.Name + "."
			case "up_to_date":
				message = "Atlas snippet in " + target.Name + " is up to date."
			case "outdated":
				message = "Atlas snippet in " + target.Name + " is outdated. Run `luuna-atlas onboard` to update."
			case "legacy":
				message = "Atlas snippet in " + target.Name + " uses legacy format (no markers). Run `luuna-atlas onboard` to migrate."
			}
		} else {
			switch action {
			case "created":
				message = "Atlas onboarding snippet written to " + target.Name + "."
			case "appended":
				message = "Atlas onboarding snippet appended to " + target.Name + "."
			case "updated":
				message = "Atlas onboarding snippet updated in " + target.Name + "."
			case "migrated":
				message = "Atlas onboarding snippet migrated to marker format in " + target.Name + "."
			case "up_to_date":
				message = "Atlas snippet in " + target.Name + " is already up to date. No changes made."
			}
		}
		color := "green"
		if (i.Bool("check") && action != "up_to_date") || (!i.Bool("check") && action == "up_to_date") {
			color = "yellow"
		}
		a.Human(Style(color, message))
		if len(duplicates) > 0 {
			names := []string{}
			for _, d := range duplicates {
				names = append(names, d.Name)
			}
			a.Human(Style("yellow", "Warning: luuna-atlas snippet also found in: "+strings.Join(names, ", ")))
		}
	}
	return nil
}
