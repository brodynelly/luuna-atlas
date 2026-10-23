package atlas

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const Version = "0.11.0"

type Invocation struct {
	Positionals []string
	Flags       map[string][]string
}

func (i Invocation) String(name string) string {
	values := i.Flags[name]
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}
func (i Invocation) Strings(name string) []string { return i.Flags[name] }
func (i Invocation) Bool(name string) bool        { return i.Has(name) && i.String(name) != "false" }
func (i Invocation) Has(name string) bool         { _, ok := i.Flags[name]; return ok }

type App struct {
	Cwd                                                              string
	In                                                               io.Reader
	Out, Err                                                         io.Writer
	JSONMode, Quiet, Verbose, AllowUnknownTypes, AllowDomainMismatch bool
	Format                                                           string
	Registry                                                         *Registry
	ExitCode                                                         int
	Command                                                          string
}

func NewApp() *App {
	cwd, _ := os.Getwd()
	return &App{Cwd: cwd, In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
}
func (a *App) Emit(value any) {
	orderCommandOutput(Obj(value))
	fmt.Fprintln(a.Out, JSONPretty(value))
}
func (a *App) OK(command string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["success"] = true
	fields["command"] = command
	a.Emit(fields)
}
func (a *App) Error(command, message string) {
	a.ExitCode = 1
	if a.JSONMode {
		result := map[string]any{"success": false, "command": command, "error": message}
		orderCommandOutput(result)
		fmt.Fprintln(a.Err, JSONPretty(result))
	} else {
		fmt.Fprintln(a.Err, Style("red", "Error: "+message))
	}
}
func (a *App) Human(text string) {
	if !a.Quiet {
		fmt.Fprintln(a.Out, text)
	}
}
func (a *App) Warning(text string)   { fmt.Fprintln(a.Err, Style("yellow", "Warning: "+text)) }
func optionName(value string) string { return strings.TrimLeft(value, "-") }

// Property order is visible in human-readable JSON and retained for callers
// that snapshot the existing CLI responses. Missing optional fields are skipped.
func orderCommandOutput(result map[string]any) {
	command := Text(result["command"])
	if command == "" {
		return
	}
	orders := map[string][]string{
		"init":          {"created", "path"},
		"add":           {"domain"},
		"record":        {"action", "wouldDo", "domain", "type", "index", "record", "created", "updated", "skipped", "errors", "warnings"},
		"edit":          {"domain", "id", "type", "record"},
		"query":         {"domains"},
		"show":          {"records"},
		"search":        {"query", "total", "domains"},
		"rank":          {"count", "records"},
		"ready":         {"count", "entries"},
		"status":        {"domains", "governance", "shelf_life"},
		"stale":         {"records"},
		"validate":      {"valid", "totalRecords", "totalErrors", "totalWarnings", "errors", "warnings"},
		"doctor":        {"checks", "summary", "fixed"},
		"audit":         {"report", "suggestions"},
		"archive":       {"domain", "dryRun", "archived", "kept"},
		"restore":       {"domain", "id", "type", "summary", "archive_reason"},
		"delete":        {"domain", "dryRun", "deleted", "kept", "id", "type", "summary"},
		"delete-domain": {"domain", "dryRun", "deletedFile", "recordCount"},
		"move":          {"dryRun", "sourceDomain", "targetDomain", "record", "incomingReferences", "warnings"},
		"outcome":       {"action", "domain", "id", "outcomes", "outcome", "total_outcomes"},
		"prune":         {"dryRun", "hard", "aggressive", "checkAnchors", "totalPruned", "totalDemoted", "totalAnchorDemoted", "totalSupersessionDemoted", "results", "warnings"},
		"compact":       {"action", "candidates", "compacted", "results", "wouldCompact", "groups", "domain", "removed", "replacement", "warnings"},
		"onboard":       {"file", "action"},
		"setup":         {"target", "provider", "source", "path", "recipe", "action", "providers", "message"},
		"sync":          {"validated", "committed", "errors", "message"},
		"learn":         {"changedFiles", "suggestedDomains", "unmatchedFiles", "message"},
		"diff":          {"since", "domains", "message"},
		"upgrade":       {"error", "current", "latest", "up_to_date", "updated"},
	}
	keys := []string{"success", "command", "error"}
	keys = append(keys, orders[command]...)
	rememberOrder(result, keys)
}
