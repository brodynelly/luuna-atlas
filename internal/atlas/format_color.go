package atlas

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// ColorLevel mirrors Chalk's process-wide stdout capability selection. Callers
// keep colors out of machine-output paths just as the reference does.
func ColorLevel() int {
	args := []string{}
	for _, arg := range os.Args[1:] {
		if arg == "--" {
			break
		}
		args = append(args, arg)
	}
	has := func(values ...string) bool {
		for _, v := range values {
			if Contains(args, v) {
				return true
			}
		}
		return false
	}
	force, defined := 0, false
	if has("--no-color", "--no-colors", "--color=false", "--color=never") {
		force, defined = 0, true
	} else if has("--color", "--colors", "--color=true", "--color=always") {
		force, defined = 1, true
	}
	invalidForce := false
	if raw, ok := os.LookupEnv("FORCE_COLOR"); ok {
		defined = true
		switch raw {
		case "true", "":
			force = 1
		case "false":
			force = 0
		default:
			prefix := regexp.MustCompile(`^[ \t]*[+-]?[0-9]+`).FindString(raw)
			if prefix == "" {
				invalidForce = true
				force = 0
			} else {
				force, _ = strconv.Atoi(strings.TrimSpace(prefix))
				if force > 3 {
					force = 3
				}
			}
		}
	}
	if defined && force == 0 && !invalidForce {
		return 0
	}
	if has("--color=16m", "--color=full", "--color=truecolor") {
		return 3
	}
	if has("--color=256") {
		return 2
	}
	_, tf := os.LookupEnv("TF_BUILD")
	_, agent := os.LookupEnv("AGENT_NAME")
	if tf && agent {
		return 1
	}
	if !stdoutIsTerminal() && !defined {
		return 0
	}
	minimum := force
	if !defined || invalidForce {
		minimum = 0
	}
	term := os.Getenv("TERM")
	if term == "dumb" {
		return minimum
	}
	if runtime.GOOS == "windows" {
		return windowsTerminalColorLevel()
	}
	present := func(key string) bool { _, ok := os.LookupEnv(key); return ok }
	if present("CI") {
		for _, key := range []string{"GITHUB_ACTIONS", "GITEA_ACTIONS", "CIRCLECI"} {
			if present(key) {
				return 3
			}
		}
		for _, key := range []string{"TRAVIS", "APPVEYOR", "GITLAB_CI", "BUILDKITE", "DRONE"} {
			if present(key) {
				return 1
			}
		}
		if os.Getenv("CI_NAME") == "codeship" {
			return 1
		}
		return minimum
	}
	if present("TEAMCITY_VERSION") {
		if regexp.MustCompile(`^(9\.(0*[1-9][0-9]*)\.|[0-9]{2,}\.)`).MatchString(os.Getenv("TEAMCITY_VERSION")) {
			return 1
		}
		return 0
	}
	if os.Getenv("COLORTERM") == "truecolor" || Contains([]string{"xterm-kitty", "xterm-ghostty", "wezterm"}, term) {
		return 3
	}
	if present("TERM_PROGRAM") {
		switch os.Getenv("TERM_PROGRAM") {
		case "iTerm.app":
			version, _ := strconv.Atoi(strings.Split(os.Getenv("TERM_PROGRAM_VERSION"), ".")[0])
			if version >= 3 {
				return 3
			}
			return 2
		case "Apple_Terminal":
			return 2
		}
	}
	if regexp.MustCompile(`(?i)-256(color)?$`).MatchString(term) {
		return 2
	}
	if regexp.MustCompile(`(?i)^screen|^xterm|^vt100|^vt220|^rxvt|color|ansi|cygwin|linux`).MatchString(term) {
		return 1
	}
	if present("COLORTERM") {
		return 1
	}
	return minimum
}
func colorRGBCode(r, g, b int, level int) string {
	if level >= 3 {
		return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
	}
	if level == 2 {
		n := 0
		if r == g && g == b {
			if r < 8 {
				n = 16
			} else if r > 248 {
				n = 231
			} else {
				n = int(math.Round((float64(r)-8)/247*24)) + 232
			}
		} else {
			n = 16 + 36*int(math.Round(float64(r)/255*5)) + 6*int(math.Round(float64(g)/255*5)) + int(math.Round(float64(b)/255*5))
		}
		return fmt.Sprintf("38;5;%d", n)
	}
	value := int(math.Round(float64(max(r, max(g, b))) / 255 * 2))
	if value == 0 {
		return "30"
	}
	code := 30 + (int(math.Round(float64(b)/255))<<2 | int(math.Round(float64(g)/255))<<1 | int(math.Round(float64(r)/255)))
	if value == 2 {
		code += 60
	}
	return strconv.Itoa(code)
}
func Style(style, text string) string {
	level := ColorLevel()
	if level == 0 || text == "" {
		return text
	}
	open, close := "", "39"
	switch style {
	case "brand":
		open = colorRGBCode(139, 90, 43, level)
	case "accent":
		open = colorRGBCode(255, 183, 77, level)
	case "muted":
		open = colorRGBCode(120, 120, 110, level)
	case "red":
		open = "31"
	case "green":
		open = "32"
	case "yellow":
		open = "33"
	case "blue":
		open = "34"
	case "magenta":
		open = "35"
	case "cyan":
		open = "36"
	case "white":
		open = "37"
	case "bold":
		open = "1"
		close = "22"
	case "dim":
		open = "2"
		close = "22"
	default:
		return text
	}
	start, end := "\x1b["+open+"m", "\x1b["+close+"m"
	text = strings.ReplaceAll(text, end, end+start)
	text = regexp.MustCompile(`\r?\n`).ReplaceAllStringFunc(text, func(line string) string { return end + line + start })
	return start + text + end
}
