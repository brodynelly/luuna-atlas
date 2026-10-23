package atlas

import (
	"fmt"
	"strings"
)

var completionCommands = []string{"init", "add", "record", "edit", "query", "setup", "prime", "hook", "onboard", "status", "validate", "prune", "archive", "restore", "search", "show", "stale", "rank", "outcome", "doctor", "ready", "sync", "delete", "delete-domain", "move", "learn", "compact", "config", "diff", "upgrade", "completions", "audit"}
var completionGlobals = []string{"--version", "--json", "--quiet", "--verbose", "--timing", "--allow-unknown-types", "--allow-domain-mismatch", "--format"}

func CompletionScript(shell string) string {
	allWords := strings.Join(append(append([]string{}, completionCommands...), completionGlobals...), " ")
	switch shell {
	case "bash":
		return "# luuna-atlas bash completions\n# Add to ~/.bashrc: eval \"$(luuna-atlas completions bash)\"\n_atlas() {\n  local cur=\"${COMP_WORDS[COMP_CWORD]}\"\n  COMPREPLY=( $(compgen -W \"" + allWords + "\" -- \"$cur\") )\n}\ncomplete -F _atlas luuna-atlas\ncomplete -F _atlas la\n"
	case "zsh":
		return "# luuna-atlas zsh completions\n# Add to ~/.zshrc: eval \"$(luuna-atlas completions zsh)\"\n_atlas() {\n  local -a words\n  words=(" + allWords + ")\n  _describe 'luuna-atlas commands' words\n}\ncompdef _atlas luuna-atlas\ncompdef _atlas la\n"
	case "fish":
		lines := []string{"# luuna-atlas fish completions", "# Save to ~/.config/fish/completions/luuna-atlas.fish", ""}
		for _, command := range completionCommands {
			lines = append(lines, "complete -c luuna-atlas -n '__fish_use_subcommand' -a '"+command+"' -d '"+command+" command'", "complete -c la -n '__fish_use_subcommand' -a '"+command+"' -d '"+command+" command'")
		}
		return strings.Join(lines, "\n") + "\n"
	}
	return ""
}
func (a *App) runCompletions(i Invocation) error {
	if len(i.Positionals) == 0 {
		return nil
	}
	shell := strings.ToLower(i.Positionals[0])
	if !Contains([]string{"bash", "zsh", "fish"}, shell) {
		a.ExitCode = 1
		fmt.Fprintln(a.Err, Style("red", "✗")+" "+Style("red", "Unsupported shell: \""+i.Positionals[0]+"\". Supported: bash, zsh, fish"))
		return nil
	}
	fmt.Fprint(a.Out, CompletionScript(shell))
	return nil
}
