package atlas

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

type validationMessage struct {
	Domain  string `json:"domain"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}
type validationReport struct {
	TotalRecords     int
	Errors, Warnings []validationMessage
}

func schemaIssueText(issues []ValidationIssue) string {
	parts := []string{}
	for _, issue := range issues {
		parts = append(parts, issue.Path+" "+issue.Message)
	}
	return strings.Join(parts, "; ")
}
func (a *App) validateCorpus(cfg Config, syncMode bool) (validationReport, error) {
	report := validationReport{Errors: []validationMessage{}, Warnings: []validationMessage{}}
	for _, domain := range DomainNames(cfg) {
		path, e := ExpertisePath(a.Cwd, domain)
		if e != nil {
			return report, e
		}
		data, e := os.ReadFile(path)
		if e != nil {
			continue
		}
		for n, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			report.TotalRecords++
			addError := func(message string) { report.Errors = append(report.Errors, validationMessage{domain, n + 1, message}) }
			value, e := ParseJSON([]byte(line))
			if e != nil {
				addError("Invalid JSON: failed to parse")
				continue
			}
			r := Record(Obj(value))
			if typeName, ok := r["type"].(string); ok {
				def := a.Registry.Types[typeName]
				if def == nil {
					if a.AllowUnknownTypes && !syncMode {
						continue
					}
					idPart := ""
					if id, ok := r["id"].(string); ok {
						idPart = " (id=" + id + ")"
					}
					addError(fmt.Sprintf("Unknown record type %q%s. Register it under custom_types in config.yaml or remove the record.", typeName, idPart))
					continue
				}
				ApplyAliases(r, def.Aliases)
			}
			_, legacy := r["outcome"]
			_, plural := r["outcomes"]
			if legacy && !plural && !syncMode {
				report.Warnings = append(report.Warnings, validationMessage{domain, n + 1, `Legacy "outcome" field (singular); run ` + "`luuna-atlas doctor --fix`" + ` to migrate to "outcomes[]"`})
			} else if issues := ValidateRecordValue(value, a.Registry); len(issues) > 0 {
				addError("Schema validation failed: " + schemaIssueText(issues))
				continue
			}
			if a.AllowDomainMismatch && !syncMode {
				continue
			}
			typeName := Text(r["type"])
			if typeName == "" {
				typeName = "<unknown>"
			}
			idPart := ""
			if id := Text(r["id"]); id != "" {
				idPart = " (id=" + id + ")"
			}
			allowed := GetAllowedTypes(cfg, domain)
			if len(allowed) > 0 && !Contains(allowed, typeName) {
				message := fmt.Sprintf("type %q%s is not allowed in domain %q. Allowed types: %s.", typeName, idPart, domain, strings.Join(allowed, ", "))
				if syncMode {
					message += " Fix the record or relax allowed_types in config.yaml."
				}
				addError(message)
				if syncMode {
					continue
				}
			}
			missing := FindMissingDomainFields(r, GetRequiredFields(cfg, domain))
			if len(missing) > 0 {
				quoted := []string{}
				for _, f := range missing {
					quoted = append(quoted, fmt.Sprintf("%q", f))
				}
				message := fmt.Sprintf("record%s missing required field(s) %s in domain %q.", idPart, strings.Join(quoted, ", "), domain)
				if syncMode {
					message += " Fix the record or relax required_fields in config.yaml."
				}
				addError(message)
			}
		}
	}
	return report, nil
}
func (a *App) runValidate(i Invocation) error {
	cfg, e := ReadConfig(a.Cwd)
	if e != nil {
		return e
	}
	report, e := a.validateCorpus(cfg, false)
	if e != nil {
		return e
	}
	valid := len(report.Errors) == 0
	if a.JSONMode {
		a.Emit(map[string]any{"success": valid, "command": "validate", "valid": valid, "totalRecords": report.TotalRecords, "totalErrors": len(report.Errors), "totalWarnings": len(report.Warnings), "errors": report.Errors, "warnings": report.Warnings})
	} else {
		type diagnostic struct {
			entry   validationMessage
			warning bool
		}
		diagnostics := []diagnostic{}
		for _, entry := range report.Errors {
			diagnostics = append(diagnostics, diagnostic{entry, false})
		}
		for _, entry := range report.Warnings {
			diagnostics = append(diagnostics, diagnostic{entry, true})
		}
		domainOrder := map[string]int{}
		for index, domain := range DomainNames(cfg) {
			domainOrder[domain] = index
		}
		sort.SliceStable(diagnostics, func(i, j int) bool {
			a, b := diagnostics[i], diagnostics[j]
			if a.entry.Domain != b.entry.Domain {
				return domainOrder[a.entry.Domain] < domainOrder[b.entry.Domain]
			}
			if a.entry.Line != b.entry.Line {
				return a.entry.Line < b.entry.Line
			}
			return a.warning && !b.warning
		})
		for _, item := range diagnostics {
			entry := item.entry
			style := "red"
			if item.warning {
				style = "yellow"
			}
			if strings.HasPrefix(entry.Message, "Schema validation failed: ") {
				fmt.Fprintln(a.Err, Style(style, fmt.Sprintf("%s:%d - Schema validation failed:", entry.Domain, entry.Line)))
				for _, issue := range strings.Split(strings.TrimPrefix(entry.Message, "Schema validation failed: "), "; ") {
					fmt.Fprintln(a.Err, Style(style, "  "+issue))
				}
			} else {
				fmt.Fprintln(a.Err, Style(style, fmt.Sprintf("%s:%d - %s", entry.Domain, entry.Line, entry.Message)))
			}
		}
		text := fmt.Sprintf("%d records validated, %d errors found", report.TotalRecords, len(report.Errors))
		if len(report.Errors) == 0 && len(report.Warnings) > 0 {
			text += fmt.Sprintf(", %d warning(s)", len(report.Warnings))
		}
		style := "green"
		if len(report.Errors) > 0 {
			style = "red"
		} else if len(report.Warnings) > 0 {
			style = "yellow"
		}
		fmt.Fprintln(a.Out, Style(style, text))
	}
	if !valid {
		a.ExitCode = 1
	}
	return nil
}
