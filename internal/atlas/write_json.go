package atlas

import (
	"fmt"
	"strings"
)

// JSON is decoded by encoding/json. This diagnostic walk preserves the former
// JavaScriptCore JSON.parse messages without executing a JavaScript runtime.
func writeJSONParseError(input string) string {
	p := writeJSONDiagnostic{s: input}
	message := p.value()
	p.space()
	if message == "" && p.pos != len(p.s) {
		message = "Unable to parse JSON string"
	}
	if message == "" {
		message = "Unable to parse JSON string"
	}
	return "JSON Parse error: " + message
}

type writeJSONDiagnostic struct {
	s   string
	pos int
}

func (p *writeJSONDiagnostic) space() {
	for p.pos < len(p.s) && strings.ContainsRune(" \t\r\n", rune(p.s[p.pos])) {
		p.pos++
	}
}
func (p *writeJSONDiagnostic) value() string {
	p.space()
	if p.pos == len(p.s) {
		return "Unexpected EOF"
	}
	switch ch := p.s[p.pos]; ch {
	case '{':
		p.pos++
		p.space()
		if p.pos < len(p.s) && p.s[p.pos] == '}' {
			p.pos++
			return ""
		}
		if p.pos == len(p.s) || p.s[p.pos] != '"' {
			return "Expected '}'"
		}
		for {
			if message := p.stringValue(); message != "" {
				return message
			}
			p.space()
			if p.pos == len(p.s) || p.s[p.pos] != ':' {
				return "Expected ':' before value in object property definition"
			}
			p.pos++
			if message := p.value(); message != "" {
				return message
			}
			p.space()
			if p.pos < len(p.s) && p.s[p.pos] == '}' {
				p.pos++
				return ""
			}
			if p.pos == len(p.s) || p.s[p.pos] != ',' {
				return "Expected '}'"
			}
			p.pos++
			p.space()
			if p.pos == len(p.s) || p.s[p.pos] != '"' {
				return "Property name must be a string literal"
			}
		}
	case '[':
		p.pos++
		p.space()
		if p.pos < len(p.s) && p.s[p.pos] == ']' {
			p.pos++
			return ""
		}
		for {
			if message := p.value(); message != "" {
				return message
			}
			p.space()
			if p.pos < len(p.s) && p.s[p.pos] == ']' {
				p.pos++
				return ""
			}
			if p.pos == len(p.s) || p.s[p.pos] != ',' {
				return "Expected ']'"
			}
			p.pos++
			p.space()
			if p.pos < len(p.s) && p.s[p.pos] == ']' {
				return "Unexpected comma at the end of array expression"
			}
		}
	case '"':
		return p.stringValue()
	case '\'':
		return "Single quotes (') are not allowed in JSON"
	case '}', ']', ':', ',', '.':
		return fmt.Sprintf("Unexpected token '%c'", ch)
	}
	ch := p.s[p.pos]
	if ch == '-' || ch >= '0' && ch <= '9' {
		return p.number()
	}
	if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || ch == '$' {
		start := p.pos
		for p.pos < len(p.s) {
			ch = p.s[p.pos]
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '$') {
				break
			}
			p.pos++
		}
		word := p.s[start:p.pos]
		for _, literal := range []string{"true", "false", "null"} {
			if strings.HasPrefix(word, literal) {
				p.pos = start + len(literal)
				return ""
			}
		}
		return "Unexpected identifier " + writeJSONQuote(word)
	}
	return fmt.Sprintf("Unrecognized token '%c'", ch)
}
func (p *writeJSONDiagnostic) number() string {
	if p.s[p.pos] == '-' {
		p.pos++
	}
	if p.pos == len(p.s) || p.s[p.pos] < '0' || p.s[p.pos] > '9' {
		return "Invalid number"
	}
	if p.s[p.pos] == '0' {
		p.pos++
	} else {
		for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.pos < len(p.s) && p.s[p.pos] == '.' {
		p.pos++
		if p.pos == len(p.s) || p.s[p.pos] < '0' || p.s[p.pos] > '9' {
			return "Invalid digits after decimal point"
		}
		for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.pos < len(p.s) && (p.s[p.pos] == 'e' || p.s[p.pos] == 'E') {
		start := p.pos
		p.pos++
		if p.pos < len(p.s) && (p.s[p.pos] == '+' || p.s[p.pos] == '-') {
			p.pos++
		}
		if p.pos == len(p.s) || p.s[p.pos] < '0' || p.s[p.pos] > '9' {
			p.pos = start
			return ""
		}
		for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
			p.pos++
		}
	}
	return ""
}
func (p *writeJSONDiagnostic) stringValue() string {
	p.pos++
	for p.pos < len(p.s) {
		ch := p.s[p.pos]
		p.pos++
		if ch == '"' {
			return ""
		}
		if ch < 0x20 {
			return "Unterminated string"
		}
		if ch != '\\' {
			continue
		}
		if p.pos == len(p.s) {
			return "Unterminated string"
		}
		ch = p.s[p.pos]
		p.pos++
		if strings.ContainsRune(`"\/bfnrt`, rune(ch)) {
			continue
		}
		if ch != 'u' {
			return fmt.Sprintf("Invalid escape character %c", ch)
		}
		start := p.pos - 2
		end := p.pos + 4
		if end > len(p.s) {
			end = len(p.s)
		}
		valid := end-p.pos == 4
		for n := p.pos; n < end; n++ {
			if !strings.ContainsRune("0123456789abcdefABCDEF", rune(p.s[n])) {
				valid = false
			}
		}
		if !valid {
			return `"` + p.s[start:end] + `" is not a valid unicode escape`
		}
		p.pos = end
	}
	return "Unterminated string"
}
