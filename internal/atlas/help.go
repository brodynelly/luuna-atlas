package atlas

import (
	"fmt"
	"os"
	"strings"
)

func renderHelp(spec commandSpec) string {
	text := spec.Help
	// Commander strips help colors independently of Chalk's color selection.
	// In particular NO_COLOR disables help even when FORCE_COLOR is enabled.
	force := os.Getenv("FORCE_COLOR")
	_, cliColor := os.LookupEnv("CLICOLOR_FORCE")
	if os.Getenv("NO_COLOR") != "" || force == "0" || force == "false" || (force == "" && !cliColor && !stdoutIsTerminal()) {
		return text
	}
	lines := strings.Split(text, "\n")
	section := ""
	name := spec.Name
	header := name
	if name == "luuna-atlas" {
		header = "Luuna Atlas"
	}
	for n, line := range lines {
		if n == 0 {
			lines[n] = Style("brand", Style("bold", header)) + " " + Style("muted", "v"+Version) + " — Structured expertise management"
			continue
		}
		if strings.HasPrefix(line, "Usage: ") {
			lines[n] = "Usage: " + Style("dim", name) + " <command> [options]"
			continue
		}
		if line == "Commands:" {
			section = "commands"
			continue
		}
		if line == "Options:" {
			section = "options"
			continue
		}
		if strings.HasPrefix(line, "Run '") {
			lines[n] = "Run '" + Style("dim", name) + " <command> --help' for command-specific help."
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			continue
		}
		if section == "commands" {
			commands := append(append([]commandSpec{}, spec.Commands...), commandSpec{Name: "help", Description: "display help for command"})
			for _, child := range commands {
				if child.Hidden {
					continue
				}
				prefix := "  " + child.Name
				if strings.HasPrefix(line, prefix+" ") {
					rest := strings.TrimPrefix(line, prefix)
					descriptionIndex := strings.LastIndex(rest, child.Description)
					if descriptionIndex < 0 {
						break
					}
					termAndPadding := rest[:descriptionIndex]
					trimmed := strings.TrimRight(termAndPadding, " ")
					padding := termAndPadding[len(trimmed):]
					args := trimmed
					lines[n] = "  " + Style("green", child.Name) + Style("dim", args) + padding + child.Description
					break
				}
			}
		}
		if section == "options" {
			opts := append(append([]optionSpec{}, spec.Options...), optionSpec{Flags: "-h, --help", Description: "display help for command"})
			for _, opt := range opts {
				if strings.HasPrefix(line, "  "+opt.Flags) {
					lines[n] = "  " + Style("dim", opt.Flags) + strings.TrimPrefix(line, "  "+opt.Flags)
					break
				}
			}
		}
	}
	return strings.Join(lines, "\n")
}

func customizeHelp(spec *commandSpec, originalOptions []optionSpec) {
	for _, opt := range spec.Options {
		if opt.Name == "type" && len(opt.Choices) > 0 {
			for _, old := range originalOptions {
				if old.Name == "type" && len(old.Choices) > 0 {
					oldChoices, choices := []string{}, []string{}
					for _, v := range old.Choices {
						oldChoices = append(oldChoices, fmt.Sprintf("%q", v))
					}
					for _, v := range opt.Choices {
						choices = append(choices, fmt.Sprintf("%q", v))
					}
					spec.Help = strings.ReplaceAll(spec.Help, "choices: "+strings.Join(oldChoices, ", "), "choices: "+strings.Join(choices, ", "))
				}
			}
		}
	}
	added := []string{}
	for _, opt := range spec.Options {
		found := false
		for _, old := range originalOptions {
			if old.Name == opt.Name {
				found = true
				break
			}
		}
		if !found {
			padding := strings.Repeat(" ", max(2, 20-len(opt.Flags)))
			added = append(added, "  "+opt.Flags+padding+opt.Description)
		}
	}
	if len(added) > 0 {
		spec.Help = strings.Replace(spec.Help, "  -h, --help", strings.Join(added, "\n")+"\n  -h, --help", 1)
	}
}
