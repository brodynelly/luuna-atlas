package atlas

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	"gopkg.in/yaml.v3"
)

func yamlMappingKey(node *yaml.Node) string {
	value := yamlValue(node)
	if value == nil {
		return "null"
	}
	if number, ok := value.(float64); ok {
		if math.IsInf(number, 1) {
			return "Infinity"
		}
		if math.IsInf(number, -1) {
			return "-Infinity"
		}
	}
	return Text(value)
}

func validateYAMLKeys(root *yaml.Node, input string) error {
	visited := map[*yaml.Node]bool{}
	var walk func(*yaml.Node) error
	walk = func(node *yaml.Node) error {
		if node == nil || visited[node] {
			return nil
		}
		visited[node] = true
		if node.Kind == yaml.AliasNode {
			return walk(node.Alias)
		}
		if node.Kind == yaml.MappingNode {
			explicit := map[string]bool{}
			for i := 0; i+1 < len(node.Content); i += 2 {
				key, value := node.Content[i], node.Content[i+1]
				if err := walk(value); err != nil {
					return err
				}
				if key.Tag == "!!merge" {
					continue
				}
				name := yamlMappingKey(key)
				if explicit[name] {
					return yamlSourceError("duplicated mapping key", input, key.Line, key.Column)
				}
				explicit[name] = true
			}
			return nil
		}
		for _, child := range node.Content {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

func yamlSourceError(reason, input string, line, column int) error {
	input = strings.ReplaceAll(strings.ReplaceAll(input, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(strings.TrimSuffix(input, "\n"), "\n")
	first, last := line-4, line+1
	if first < 0 {
		first = 0
	}
	if last >= len(lines) {
		last = len(lines) - 1
	}
	maxNumber := line + 1
	if maxNumber > len(lines) {
		maxNumber = len(lines)
	}
	digits := len(strconv.Itoa(maxNumber))
	snippet := []string{}
	for i := first; i <= last; i++ {
		text, pos := yamlSnippetLine(lines[i], column-1, 79-(1+digits+3))
		snippet = append(snippet, fmt.Sprintf(" %*d | %s", digits, i+1, text))
		if i == line-1 {
			snippet = append(snippet, strings.Repeat("-", 1+digits+3+pos)+"^")
		}
	}
	return fmt.Errorf("%s (%d:%d)\n\n%s", reason, line, column, strings.Join(snippet, "\n"))
}
func yamlSnippetLine(text string, position, maxLength int) (string, int) {
	units := utf16.Encode([]rune(text))
	start, end := 0, len(units)
	head, tail := "", ""
	half := maxLength/2 - 1
	if position > half {
		head = " ... "
		start = position - half + len(head)
	}
	if end-position > half {
		tail = " ..."
		end = position + half - len(tail)
	}
	if start > len(units) {
		start = len(units)
	}
	if end > len(units) {
		end = len(units)
	}
	if end < start {
		end = start
	}
	return head + strings.ReplaceAll(string(utf16.Decode(units[start:end])), "\t", "→") + tail, position - start + len(head)
}
