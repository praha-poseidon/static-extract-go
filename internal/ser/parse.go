package ser

import (
	"fmt"
	"strings"
	"unicode"
)

func Parse(source string) (*Rule, error) {
	lines := splitLines(source)
	r := &Rule{Build: map[string]BuildExpr{}}
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			i++
			continue
		}
		toks := tokenize(line)
		if len(toks) == 0 {
			i++
			continue
		}
		switch toks[0] {
		case "rule":
			if len(toks) < 2 {
				return nil, fmt.Errorf("ser: rule needs a name at line %d", i+1)
			}
			r.Name = unquote(toks[1])
			i++
		case "fact":
			if len(toks) < 2 {
				return nil, fmt.Errorf("ser: fact needs a type at line %d", i+1)
			}
			r.FactType = toks[1]
			i++
		case "endpoint":
			if len(toks) < 3 {
				return nil, fmt.Errorf("ser: endpoint needs type and direction at line %d", i+1)
			}
			r.EndpointType = toks[1]
			r.EndpointDirection = toks[2]
			if r.FactType == "" {
				r.FactType = strings.ToLower(toks[1]) + "_" + strings.ToLower(toks[2])
			}
			i++
		case "find":
			if len(toks) < 2 {
				return nil, fmt.Errorf("ser: find needs free atoms at line %d", i+1)
			}
			r.Find = toks[1:]
			i++
		case "where":
			r.Where = append(r.Where, toks[1:])
			i++
		case "when":
			r.When = append(r.When, toks[1:])
			i++
		case "let":
			let, next, err := parseLet(lines, i)
			if err != nil {
				return nil, err
			}
			r.Lets = append(r.Lets, let)
			i = next
		case "build":
			build, next, err := parseBuild(lines, i)
			if err != nil {
				return nil, err
			}
			r.Build = build
			i = next
		case "trace":
			entries, next, err := parseTrace(lines, i)
			if err != nil {
				return nil, err
			}
			r.Trace = entries
			i = next
		case "dict":
			id, next, err := parseDict(lines, i)
			if err != nil {
				return nil, err
			}
			r.IdentityDict = id
			i = next
		default:
			return nil, fmt.Errorf("ser: unexpected token %q at line %d", toks[0], i+1)
		}
	}
	if r.Name == "" {
		return nil, fmt.Errorf("ser: missing rule name")
	}
	if len(r.Find) == 0 {
		return nil, fmt.Errorf("ser: missing find")
	}
	if r.FactType == "" {
		r.FactType = "fact"
	}
	return r, nil
}

func ParseAll(source string) ([]*Rule, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("ser: empty source")
	}
	parts := splitRuleDocuments(source)
	var rules []*Rule
	for _, p := range parts {
		rule, err := Parse(p)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func splitRuleDocuments(source string) []string {
	lines := splitLines(source)
	var parts []string
	var cur []string
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "rule ") && len(cur) > 0 {
			parts = append(parts, strings.Join(cur, "\n"))
			cur = []string{line}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		parts = append(parts, strings.Join(cur, "\n"))
	}
	return parts
}

func parseLet(lines []string, i int) (Let, int, error) {
	toks := tokenize(strings.TrimSpace(lines[i]))
	if len(toks) < 2 {
		return Let{}, i, fmt.Errorf("ser: bad let at line %d", i+1)
	}
	// let name = ...  or let name=
	name := toks[1]
	if strings.HasSuffix(name, "=") {
		name = strings.TrimSuffix(name, "=")
	}
	// drop "=" token if present as toks[2]
	start := 2
	if start < len(toks) && toks[start] == "=" {
		start++
	}
	let := Let{Name: name}
	// same-line: let path = from argument[0] take value
	if start < len(toks) && toks[start] == "from" {
		takeIdx := indexOf(toks[start:], "take")
		if takeIdx >= 0 {
			takeIdx += start
			let.Sources = append(let.Sources, Source{From: toks[start+1 : takeIdx], Take: toks[takeIdx+1:]})
		}
	}
	i++
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			i++
			continue
		}
		t := tokenize(line)
		if len(t) == 0 {
			i++
			continue
		}
		if t[0] == "from" {
			takeIdx := indexOf(t, "take")
			if takeIdx < 0 {
				return Let{}, i, fmt.Errorf("ser: from without take at line %d", i+1)
			}
			let.Sources = append(let.Sources, Source{From: t[1:takeIdx], Take: t[takeIdx+1:]})
			i++
			continue
		}
		if t[0] == "fallback" || t[0] == "default" {
			if len(t) >= 2 {
				let.Fallback = unquote(t[1])
			}
			i++
			continue
		}
		if t[0] == "map" {
			m, next, err := parseMapBlock(lines, i)
			if err != nil {
				return Let{}, i, err
			}
			let.Map = m
			i = next
			continue
		}
		if isTop(t[0]) {
			break
		}
		i++
	}
	return let, i, nil
}

// parseMapBlock parses map { k: v  k2: v2 } (same shape as dict / Java SER map).
func parseMapBlock(lines []string, i int) (map[string]string, int, error) {
	line := strings.TrimSpace(lines[i])
	brace := strings.Index(line, "{")
	if brace < 0 {
		return nil, i, fmt.Errorf("ser: map expects { at line %d", i+1)
	}
	out := map[string]string{}
	rest := strings.TrimSpace(line[brace+1:])
	if rest != "" {
		if strings.HasPrefix(rest, "}") {
			return out, i + 1, nil
		}
		if idx := strings.Index(rest, "}"); idx >= 0 {
			inner := strings.TrimSpace(rest[:idx])
			if inner != "" {
				if err := parseMapEntryLine(inner, out); err != nil {
					return nil, i, err
				}
			}
			return out, i + 1, nil
		}
		if err := parseMapEntryLine(rest, out); err != nil {
			return nil, i, err
		}
	}
	i++
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			i++
			continue
		}
		if strings.HasPrefix(line, "}") {
			return out, i + 1, nil
		}
		if idx := strings.Index(line, "}"); idx >= 0 {
			fieldPart := strings.TrimSpace(line[:idx])
			if fieldPart != "" {
				if err := parseMapEntryLine(fieldPart, out); err != nil {
					return nil, i, err
				}
			}
			return out, i + 1, nil
		}
		if err := parseMapEntryLine(line, out); err != nil {
			return nil, i, err
		}
		i++
	}
	return nil, i, fmt.Errorf("ser: unclosed map")
}

// parseMapEntryLine accepts "k: v" or "k: v  k2: v2" on one line.
func parseMapEntryLine(line string, out map[string]string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	// split multiple entries by whitespace between value and next key is hard;
	// support single "key: value" or "key:value" per call; multi-entry lines split on "  " then ":"
	parts := splitMapEntries(line)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		colon := strings.Index(p, ":")
		if colon <= 0 {
			return fmt.Errorf("ser: map entry must be key: value: %q", p)
		}
		k := strings.TrimSpace(p[:colon])
		v := unquote(strings.TrimSpace(p[colon+1:]))
		if k == "" {
			return fmt.Errorf("ser: empty map key: %q", p)
		}
		out[k] = v
	}
	return nil
}

func splitMapEntries(line string) []string {
	// Prefer one entry per line; if multiple "a: b c: d" use simple scan for " word:"
	var entries []string
	depth := 0
	start := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ' ', '\t':
			if depth != 0 {
				continue
			}
			// look ahead for next key:
			j := i + 1
			for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
				j++
			}
			rest := line[j:]
			if colon := strings.Index(rest, ":"); colon > 0 {
				key := strings.TrimSpace(rest[:colon])
				if key != "" && !strings.ContainsAny(key, " \t") && j > start {
					entries = append(entries, strings.TrimSpace(line[start:i]))
					start = j
				}
			}
		}
	}
	if start < len(line) {
		entries = append(entries, strings.TrimSpace(line[start:]))
	}
	if len(entries) == 0 {
		return []string{line}
	}
	return entries
}

func parseBuild(lines []string, i int) (map[string]BuildExpr, int, error) {
	line := strings.TrimSpace(lines[i])
	brace := strings.Index(line, "{")
	if brace < 0 {
		return nil, i, fmt.Errorf("ser: build expects { at line %d", i+1)
	}
	out := map[string]BuildExpr{}
	// same-line: build { path: path } or build { path: path }
	rest := strings.TrimSpace(line[brace+1:])
	if rest != "" {
		if strings.HasPrefix(rest, "}") {
			return out, i + 1, nil
		}
		// may contain fields and closing } on same line
		if idx := strings.Index(rest, "}"); idx >= 0 {
			fieldPart := strings.TrimSpace(rest[:idx])
			if fieldPart != "" {
				if err := parseBuildFieldLine(fieldPart, out); err != nil {
					return nil, i, err
				}
			}
			return out, i + 1, nil
		}
		if err := parseBuildFieldLine(rest, out); err != nil {
			return nil, i, err
		}
	}
	i++
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			i++
			continue
		}
		if strings.HasPrefix(line, "}") {
			i++
			return out, i, nil
		}
		// field line may end with }
		if idx := strings.Index(line, "}"); idx >= 0 {
			fieldPart := strings.TrimSpace(line[:idx])
			if fieldPart != "" {
				if err := parseBuildFieldLine(fieldPart, out); err != nil {
					return nil, i, err
				}
			}
			i++
			return out, i, nil
		}
		if err := parseBuildFieldLine(line, out); err != nil {
			return nil, i, err
		}
		i++
	}
	return nil, i, fmt.Errorf("ser: unclosed build")
}

func parseBuildFieldLine(line string, out map[string]BuildExpr) error {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return fmt.Errorf("ser: build field needs colon: %q", line)
	}
	key := strings.TrimSpace(line[:colon])
	raw := strings.TrimSpace(line[colon+1:])
	if raw == "" {
		out[key] = BuildExpr{}
	} else if strings.HasPrefix(raw, "\"") {
		toks := tokenize(raw)
		if len(toks) > 0 && strings.HasPrefix(toks[0], "\"") {
			out[key] = BuildExpr{Const: unquote(toks[0]), Raw: raw}
		} else {
			out[key] = BuildExpr{Raw: raw}
		}
	} else if !strings.ContainsAny(raw, "(|") {
		toks := tokenize(raw)
		if len(toks) == 1 {
			out[key] = BuildExpr{Ref: unquote(toks[0]), Raw: raw}
		} else {
			out[key] = BuildExpr{Raw: raw}
		}
	} else {
		out[key] = BuildExpr{Raw: raw}
	}
	return nil
}

func parseTrace(lines []string, i int) ([]TraceEntry, int, error) {
	line := strings.TrimSpace(lines[i])
	if !strings.Contains(line, "{") {
		return nil, i, fmt.Errorf("ser: trace expects { at line %d", i+1)
	}
	var entries []TraceEntry
	i++
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			i++
			continue
		}
		if strings.HasPrefix(line, "}") {
			i++
			return entries, i, nil
		}
		t := tokenize(line)
		if len(t) > 0 && t[0] == "from" {
			entry := TraceEntry{From: t[1:], Build: map[string]BuildExpr{}}
			i++
			for i < len(lines) {
				line := strings.TrimSpace(lines[i])
				if line == "" || strings.HasPrefix(line, "#") {
					i++
					continue
				}
				tt := tokenize(line)
				if len(tt) == 0 {
					i++
					continue
				}
				if tt[0] == "from" || tt[0] == "}" {
					break
				}
				if tt[0] == "when" {
					entry.When = append(entry.When, tt[1:])
					i++
					continue
				}
				if tt[0] == "let" {
					let, next, err := parseLet(lines, i)
					if err != nil {
						return nil, i, err
					}
					entry.Lets = append(entry.Lets, let)
					i = next
					continue
				}
				if tt[0] == "build" {
					b, next, err := parseBuild(lines, i)
					if err != nil {
						return nil, i, err
					}
					entry.Build = b
					i = next
					continue
				}
				i++
			}
			entries = append(entries, entry)
			continue
		}
		i++
	}
	return nil, i, fmt.Errorf("ser: unclosed trace")
}

// parseDict: dict { key = value ... }
func parseDict(lines []string, i int) (map[string]string, int, error) {
	line := lines[i]
	open := strings.Index(line, "{")
	if open < 0 {
		return nil, i, fmt.Errorf("ser: dict expects { at line %d", i+1)
	}
	out := map[string]string{}
	depth := 1
	quote := byte(0)
	escaped := false
	for row := i; row < len(lines); row++ {
		current := lines[row]
		start := 0
		if row == i {
			start = open + 1
		} else if trimmed := strings.TrimSpace(current); trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		var content strings.Builder
		for column := start; column < len(current); column++ {
			ch := current[column]
			if quote != 0 {
				content.WriteByte(ch)
				if escaped {
					escaped = false
				} else if ch == '\\' {
					escaped = true
				} else if ch == quote {
					quote = 0
				}
				continue
			}
			if ch == '\'' || ch == '"' {
				quote = ch
				content.WriteByte(ch)
				continue
			}
			if ch == '{' {
				depth++
				content.WriteByte(ch)
				continue
			}
			if ch == '}' {
				depth--
				if depth == 0 {
					if field := strings.TrimSpace(content.String()); field != "" {
						if err := parseDictLine(field, out); err != nil {
							return nil, row, err
						}
					}
					trailing := strings.TrimSpace(current[column+1:])
					if trailing != "" && !strings.HasPrefix(trailing, "#") {
						return nil, row, fmt.Errorf("ser: unexpected content after dict at line %d", row+1)
					}
					return out, row + 1, nil
				}
				content.WriteByte(ch)
				continue
			}
			content.WriteByte(ch)
		}
		if field := strings.TrimSpace(content.String()); field != "" && !strings.HasPrefix(field, "#") {
			if err := parseDictLine(field, out); err != nil {
				return nil, row, err
			}
		}
	}
	return nil, i, fmt.Errorf("ser: unclosed dict")
}

func parseDictLine(line string, out map[string]string) error {
	eq := strings.Index(line, " = ")
	sep := 3
	if eq < 0 {
		eq = strings.Index(line, "=")
		sep = 1
	}
	if eq <= 0 {
		return fmt.Errorf("ser: dict line must be key = value: %q", line)
	}
	key := unquote(strings.TrimSpace(line[:eq]))
	val := unquote(strings.TrimSpace(line[eq+sep:]))
	if key == "" || val == "" {
		return fmt.Errorf("ser: empty dict key/value: %q", line)
	}
	out[key] = val
	return nil
}

func isTop(s string) bool {
	switch s {
	case "rule", "fact", "endpoint", "find", "where", "when", "let", "build", "trace", "dict":
		return true
	}
	return false
}

func indexOf(ss []string, v string) int {
	for i, s := range ss {
		if s == v {
			return i
		}
	}
	return -1
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func tokenize(line string) []string {
	var out []string
	var b strings.Builder
	inQ := false
	for _, r := range line {
		switch {
		case r == '"':
			inQ = !inQ
			b.WriteRune(r)
		case !inQ && unicode.IsSpace(r):
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
