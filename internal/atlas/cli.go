package atlas

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"
)

//go:embed contract.json
var contractBytes []byte

type argumentSpec struct {
	Name               string
	Required, Variadic bool
}
type optionSpec struct {
	Flags, Short, Long, Name, Attribute, Description string
	Required, Optional, Variadic, Negate, Mandatory  bool
	Default                                          any
	Choices                                          []string
	global                                           bool
}
type commandSpec struct {
	Name, Description, Help string
	Hidden                  bool
	Arguments               []argumentSpec
	Options                 []optionSpec
	Commands                []commandSpec
}
type compatibilityContract struct {
	Version, Baseline          string
	CLI                        commandSpec
	ConfigSchema, RecordSchema map[string]any
}

var contract compatibilityContract

func init() {
	if err := json.Unmarshal(contractBytes, &contract); err != nil {
		panic(err)
	}
	if parsed, err := ParseJSON(contractBytes); err == nil {
		contract.ConfigSchema = Obj(Obj(parsed)["configSchema"])
		contract.RecordSchema = Obj(Obj(parsed)["recordSchema"])
	}
}

func (a *App) Run(args []string) int {
	if os.Getenv("LUUNA_ATLAS_API_BRIDGE") == "1" {
		data, err := io.ReadAll(a.In)
		var result any
		if err == nil {
			var request any
			request, err = ParseJSON(data)
			if err == nil {
				result, err = RunAPIBridge(request)
			}
		}
		if err != nil {
			a.Emit(map[string]any{"success": false, "error": err.Error()})
			return 1
		}
		a.Emit(map[string]any{"success": true, "result": result, "numbers": CollectTransportNumbers(result)})
		return 0
	}
	for _, arg := range args {
		switch arg {
		case "--json":
			a.JSONMode = true
		case "--quiet", "-q":
			a.Quiet = true
		case "--verbose":
			a.Verbose = true
		case "--allow-unknown-types":
			a.AllowUnknownTypes = true
		case "--allow-domain-mismatch":
			a.AllowDomainMismatch = true
		}
	}
	cfg, _ := ReadConfig(a.Cwd)
	reg, err := NewRegistry(cfg)
	if err != nil {
		if a.JSONMode {
			a.Error("init", "Config error: "+err.Error())
		} else {
			fmt.Fprintln(a.Err, "Config error: "+err.Error())
			fmt.Fprintln(a.Err, "Edit .luuna/atlas/config.yaml to resolve, or run `luuna-atlas doctor` for details.")
			a.ExitCode = 1
		}
		return a.ExitCode
	}
	a.Registry = reg
	start := time.Now()
	wantsVersion := false
	timing := false
	for _, arg := range args {
		if arg == "--version" || arg == "-v" {
			wantsVersion = true
		}
		if arg == "--timing" {
			timing = true
		}
	}
	if wantsVersion && a.JSONMode {
		platform := runtime.GOOS
		if platform == "windows" {
			platform = "win32"
		}
		arch := runtime.GOARCH
		if arch == "amd64" {
			arch = "x64"
		}
		version := map[string]any{"name": "@luuna/atlas-cli", "version": Version, "runtime": "bun", "platform": platform + "-" + arch}
		rememberOrder(version, []string{"name", "version", "runtime", "platform"})
		a.Emit(version)
		return 0
	}
	root := contract.CLI
	index := 0
	for index < len(args) && strings.HasPrefix(args[index], "-") {
		token := args[index]
		if token == "--" {
			index++
			continue
		}
		if expanded, ok := expandShortOption(root.Options, token); ok {
			args = replaceArgument(args, index, expanded)
			continue
		}
		if token == "--help" || token == "-h" {
			fmt.Fprint(a.Out, renderHelp(root))
			return 0
		}
		if token == "--version" || token == "-v" {
			fmt.Fprintln(a.Out, Version)
			return 0
		}
		opt, inline, hasInline := findOption(root.Options, token)
		if opt == nil {
			_, help, parseErr := a.parseInvocation(root, root.Options, args[index:])
			if help {
				fmt.Fprint(a.Out, renderHelp(root))
				return 0
			}
			if errors.Is(parseErr, errVersionHandled) {
				return 0
			}
			a.parserError(parseErr.Error())
			return 1
		}
		vals, next, e := consumeOption(*opt, args, index, inline, hasInline)
		if e != nil {
			a.parserError(e.Error())
			return 1
		}
		a.setGlobal(opt.Name, vals)
		index = next
	}
	if index >= len(args) {
		fmt.Fprint(a.Err, renderHelp(root))
		return 1
	}
	name := args[index]
	index++
	var selected *commandSpec
	for i := range root.Commands {
		if root.Commands[i].Name == name {
			copy := root.Commands[i]
			selected = &copy
			break
		}
	}
	if selected == nil {
		suggestion := suggestCommand(name, root.Commands)
		message := "Unknown command: " + name
		if a.JSONMode {
			if suggestion != "" {
				message += ". Did you mean '" + suggestion + "'?"
			}
			a.Error(name, message)
		} else {
			fmt.Fprintln(a.Err, message)
			if suggestion != "" {
				fmt.Fprintf(a.Err, "Did you mean '%s'?\n", suggestion)
			}
			fmt.Fprintln(a.Err, "Run 'luuna-atlas --help' for usage.")
			a.ExitCode = 1
		}
		return 1
	}
	a.Command = name
	if len(selected.Commands) > 0 {
		for index < len(args) && strings.HasPrefix(args[index], "-") {
			if args[index] == "--help" || args[index] == "-h" {
				fmt.Fprint(a.Out, renderHelp(*selected))
				return 0
			}
			opt, inline, hasInline := findOption(root.Options, args[index])
			if opt == nil {
				break
			}
			if opt.Name == "version" {
				fmt.Fprintln(a.Out, Version)
				return 0
			}
			vals, next, e := consumeOption(*opt, args, index, inline, hasInline)
			if e != nil {
				a.parserError(e.Error())
				return 1
			}
			a.setGlobal(opt.Name, vals)
			index = next
		}
		if index >= len(args) {
			fmt.Fprint(a.Err, renderHelp(*selected))
			return 1
		}
		if args[index] == "--help" || args[index] == "-h" {
			fmt.Fprint(a.Out, renderHelp(*selected))
			return 0
		}
		childName := args[index]
		index++
		if childName == "help" {
			if index == len(args) {
				fmt.Fprint(a.Out, renderHelp(*selected))
				return 0
			}
			childName = args[index]
			for _, child := range selected.Commands {
				if child.Name == childName {
					fmt.Fprint(a.Out, renderHelp(child))
					return 0
				}
			}
			fmt.Fprint(a.Err, renderHelp(*selected))
			return 1
		}
		var child *commandSpec
		for i := range selected.Commands {
			if selected.Commands[i].Name == childName {
				c := selected.Commands[i]
				child = &c
				break
			}
		}
		if child == nil {
			if containsString(args[index:], "--help") || containsString(args[index:], "-h") {
				fmt.Fprint(a.Out, renderHelp(*selected))
				return 0
			}
			a.parserError("unknown command '" + childName + "'")
			return 1
		}
		selected = child
		a.Command = name + " " + childName
	}
	spec := *selected
	if name == "record" || name == "edit" || name == "search" || name == "rank" {
		a.customizeSpec(&spec)
	}
	invocation, help, e := a.parseInvocation(spec, root.Options, args[index:])
	if help {
		fmt.Fprint(a.Out, renderHelp(spec))
		return 0
	}
	if e != nil {
		if errors.Is(e, errVersionHandled) {
			return 0
		}
		a.parserError(e.Error())
		return 1
	}
	if e = a.dispatch(a.Command, invocation); e != nil {
		a.Error(a.Command, e.Error())
	}
	if timing {
		fmt.Fprintf(a.Err, "Done in %dms\n", time.Since(start).Milliseconds())
	}
	return a.ExitCode
}

func (a *App) parserError(message string) { fmt.Fprintln(a.Err, "error: "+message); a.ExitCode = 1 }
func (a *App) setGlobal(name string, values []string) {
	v := "true"
	if len(values) > 0 {
		v = values[len(values)-1]
	}
	switch name {
	case "json":
		a.JSONMode = v != "false"
	case "quiet":
		a.Quiet = v != "false"
	case "verbose":
		a.Verbose = v != "false"
	case "allow-unknown-types":
		a.AllowUnknownTypes = v != "false"
	case "allow-domain-mismatch":
		a.AllowDomainMismatch = v != "false"
	case "format":
		a.Format = v
	}
}

func findOption(options []optionSpec, token string) (*optionSpec, string, bool) {
	name, inline, hasInline := strings.Cut(token, "=")
	for i := range options {
		if name == options[i].Long || name == options[i].Short {
			if hasInline && !options[i].Required && !options[i].Optional {
				return nil, "", false
			}
			return &options[i], inline, hasInline
		}
	}
	return nil, "", false
}
func consumeOption(opt optionSpec, args []string, index int, inline string, hasInline bool) ([]string, int, error) {
	if !opt.Required && !opt.Optional {
		if hasInline {
			return nil, index, fmt.Errorf("option '%s' does not take an argument", opt.Long)
		}
		if opt.Negate {
			return []string{"false"}, index + 1, nil
		}
		return []string{"true"}, index + 1, nil
	}
	vals := []string{}
	next := index + 1
	if hasInline {
		vals = append(vals, inline)
	} else if next < len(args) && (opt.Required || !strings.HasPrefix(args[next], "-") || isNegativeNumber(args[next])) {
		vals = append(vals, args[next])
		next++
	} else if opt.Required {
		return nil, index, fmt.Errorf("option '%s' argument missing", opt.Flags)
	}
	if opt.Variadic {
		for next < len(args) && (!strings.HasPrefix(args[next], "-") || isNegativeNumber(args[next])) {
			vals = append(vals, args[next])
			next++
		}
	}
	for _, val := range vals {
		if len(opt.Choices) > 0 && !containsString(opt.Choices, val) {
			return nil, index, fmt.Errorf("option '%s' argument '%s' is invalid. Allowed choices are %s.", opt.Flags, val, strings.Join(opt.Choices, ", "))
		}
	}
	return vals, next, nil
}
func isNegativeNumber(s string) bool {
	return negativeNumberArg.MatchString(s)
}

var negativeNumberArg = regexp.MustCompile(`^-(\d+|\d*\.\d+)(e[+-]?\d+)?$`)

func (a *App) parseInvocation(spec commandSpec, globals []optionSpec, args []string) (Invocation, bool, error) {
	i := Invocation{Positionals: []string{}, Flags: map[string][]string{}}
	options := append([]optionSpec{}, spec.Options...)
	for _, g := range globals {
		g.global = true
		found := false
		for n, local := range options {
			if local.Long == g.Long {
				options[n] = g
				found = true
				break
			}
		}
		if !found {
			options = append(options, g)
		}
	}
	for _, opt := range spec.Options {
		key := strings.TrimPrefix(opt.Name, "no-")
		if opt.Default != nil {
			switch v := opt.Default.(type) {
			case []any:
				for _, value := range v {
					i.Flags[key] = append(i.Flags[key], fmt.Sprint(value))
				}
				if len(v) == 0 {
					i.Flags[key] = []string{}
				}
			default:
				i.Flags[key] = []string{fmt.Sprint(v)}
			}
		} else if opt.Negate {
			i.Flags[key] = []string{"true"}
		}
	}
	positionalOnly := false
	unknown := ""
	for pos := 0; pos < len(args); {
		token := args[pos]
		if token == "--" && !positionalOnly {
			positionalOnly = true
			pos++
			continue
		}
		if !positionalOnly && (token == "--help" || token == "-h") {
			return i, true, nil
		}
		if !positionalOnly && (token == "--version" || token == "-v") {
			fmt.Fprintln(a.Out, Version)
			return i, false, errVersionHandled
		}
		if positionalOnly || !strings.HasPrefix(token, "-") || token == "-" || isNegativeNumber(token) {
			i.Positionals = append(i.Positionals, token)
			pos++
			continue
		}
		opt, inline, hasInline := findOption(options, token)
		if opt == nil {
			if expanded, ok := expandShortOption(options, token); ok {
				args = replaceArgument(args, pos, expanded)
				continue
			}
		}
		if opt == nil {
			if unknown == "" {
				unknown = token
			}
			pos++
			continue
		}
		vals, next, err := consumeOption(*opt, args, pos, inline, hasInline)
		if err != nil {
			return i, false, err
		}
		key := strings.TrimPrefix(opt.Name, "no-")
		if opt.global {
			a.setGlobal(opt.Name, vals)
		} else if opt.Variadic || opt.Name == "dir-anchor" {
			if !seenFlagBefore(args[:pos], opt.Long) {
				i.Flags[key] = []string{}
			}
			i.Flags[key] = append(i.Flags[key], vals...)
		} else {
			i.Flags[key] = vals
		}
		pos = next
	}
	for _, opt := range spec.Options {
		if opt.Mandatory && !i.Has(strings.TrimPrefix(opt.Name, "no-")) {
			return i, false, fmt.Errorf("required option '%s' not specified", opt.Flags)
		}
	}
	if unknown != "" {
		return i, false, fmt.Errorf("unknown option '%s'", unknown)
	}
	required := 0
	variadic := false
	for n, arg := range spec.Arguments {
		if arg.Required {
			required++
		}
		if arg.Variadic {
			variadic = true
		}
		if arg.Required && len(i.Positionals) <= n {
			return i, false, fmt.Errorf("missing required argument '%s'", arg.Name)
		}
	}
	_ = required
	if !variadic && len(i.Positionals) > len(spec.Arguments) {
		return i, false, fmt.Errorf("too many arguments for '%s'. Expected %d arguments but got %d.", spec.Name, len(spec.Arguments), len(i.Positionals))
	}
	return i, false, nil
}

var errVersionHandled = errors.New("version handled")

// Commander consumes the first recognized short option and reprocesses the
// remaining group. Required values may be attached directly to the short flag.
func expandShortOption(options []optionSpec, token string) ([]string, bool) {
	if len(token) <= 2 || !strings.HasPrefix(token, "-") || strings.HasPrefix(token, "--") {
		return nil, false
	}
	short := token[:2]
	opt, _, _ := findOption(options, short)
	if opt == nil {
		return nil, false
	}
	if opt.Required || opt.Optional {
		return []string{short, token[2:]}, true
	}
	return []string{short, "-" + token[2:]}, true
}

func replaceArgument(args []string, index int, values []string) []string {
	out := append([]string{}, args[:index]...)
	out = append(out, values...)
	return append(out, args[index+1:]...)
}

func seenFlagBefore(args []string, flag string) bool {
	for _, v := range args {
		if v == flag || strings.HasPrefix(v, flag+"=") {
			return true
		}
	}
	return false
}
func containsString(items []string, value string) bool {
	for _, v := range items {
		if v == value {
			return true
		}
	}
	return false
}

func (a *App) customizeSpec(spec *commandSpec) {
	originalOptions := append([]optionSpec{}, spec.Options...)
	defer customizeHelp(spec, originalOptions)
	for n := range spec.Options {
		if spec.Options[n].Name == "type" {
			spec.Options[n].Choices = append([]string{}, a.Registry.Order...)
		}
	}
	if spec.Name != "record" && spec.Name != "edit" {
		return
	}
	for _, name := range a.Registry.Order {
		def := a.Registry.Types[name]
		if def.Builtin {
			continue
		}
		fields := append(append([]string{}, def.Required...), def.Optional...)
		for _, field := range fields {
			flag := strings.ReplaceAll(field, "_", "-")
			found := false
			for _, o := range spec.Options {
				if o.Name == flag {
					found = true
				}
			}
			if !found {
				description := name + " field: " + field
				if spec.Name == "edit" {
					description = "update " + description
				}
				spec.Options = append(spec.Options, optionSpec{Name: flag, Long: "--" + flag, Flags: "--" + flag + " <" + field + ">", Required: true, Description: description})
			}
		}
	}
}

func suggestCommand(input string, commands []commandSpec) string {
	best := ""
	distance := 3
	for _, command := range commands {
		d := editDistance(input, command.Name)
		if d < distance {
			best = command.Name
			distance = d
		}
	}
	return best
}
func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, ca := range ar {
		cur := make([]int, len(br)+1)
		cur[0] = i + 1
		for j, cb := range br {
			cost := 0
			if ca != cb {
				cost = 1
			}
			cur[j+1] = min(cur[j]+1, prev[j+1]+1, prev[j]+cost)
		}
		prev = cur
	}
	return prev[len(br)]
}

func (a *App) dispatch(command string, i Invocation) error {
	switch command {
	case "init":
		created, path, err := InitStore(a.Cwd)
		if err != nil {
			return err
		}
		if a.JSONMode {
			a.OK(command, map[string]any{"created": created, "path": path})
		} else if created {
			a.Human(Style("brand", "Initialized .luuna/atlas/ in "+a.Cwd))
		} else {
			a.Human(Style("brand", "Updated .luuna/atlas/ — filled in any missing artifacts."))
		}
		return nil
	case "add":
		return a.runAdd(i)
	case "record":
		return a.runRecord(i)
	case "edit":
		return a.runEdit(i)
	case "query":
		return a.runQuery(i)
	case "setup":
		return a.runSetup(i)
	case "prime":
		return a.runPrime(i)
	case "hook":
		return a.runHook(i)
	case "onboard":
		return a.runOnboard(i)
	case "status":
		return a.runStatus(i)
	case "validate":
		return a.runValidate(i)
	case "prune":
		return a.runPrune(i)
	case "archive":
		return a.runArchive(i)
	case "restore":
		return a.runRestore(i)
	case "search":
		return a.runSearch(i)
	case "show":
		return a.runShow(i)
	case "stale":
		return a.runStale(i)
	case "rank":
		return a.runRank(i)
	case "outcome":
		return a.runOutcome(i)
	case "doctor":
		return a.runDoctor(i)
	case "ready":
		return a.runReady(i)
	case "sync":
		return a.runSync(i)
	case "delete":
		return a.runDelete(i)
	case "delete-domain":
		return a.runDeleteDomain(i)
	case "move":
		return a.runMove(i)
	case "learn":
		return a.runLearn(i)
	case "compact":
		return a.runCompact(i)
	case "config schema":
		a.Emit(contract.ConfigSchema)
		return nil
	case "config show":
		return a.runConfigShow(i)
	case "config set":
		return a.runConfigSet(i)
	case "config unset":
		return a.runConfigUnset(i)
	case "diff":
		return a.runDiff(i)
	case "upgrade":
		message := "Luuna Atlas is not published; update through a reviewed Luuna checkout."
		if a.JSONMode {
			a.Emit(map[string]any{"success": false, "command": command, "error": message, "updated": false})
			a.ExitCode = 1
			return nil
		}
		return errors.New(message)
	case "update":
		message := "'luuna-atlas update' is deprecated. Use 'luuna-atlas upgrade' instead."
		if a.JSONMode {
			a.Error(command, message)
		} else {
			fmt.Fprintln(a.Out, "⚠ "+message)
			a.ExitCode = 1
		}
		return nil
	case "completions":
		return a.runCompletions(i)
	case "audit":
		return a.runAudit(i)
	}
	return fmt.Errorf("Unknown command: %s", command)
}

func (a *App) readDomain(domain string) ([]Record, error) {
	path, err := ExpertisePath(a.Cwd, domain)
	if err != nil {
		return nil, err
	}
	return ReadRecords(path, a.Registry, a.AllowUnknownTypes)
}
func fileModTime(path string) *time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	t := info.ModTime()
	return &t
}
