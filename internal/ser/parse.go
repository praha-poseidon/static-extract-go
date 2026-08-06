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
	name := toks[1]
	if strings.HasSuffix(name, "=") {
		name = strings.TrimSuffix(name, "=")
	}
	let := Let{Name: name}
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
		if isTop(t[0]) {
			break
		}
		i++
	}
	return let, i, nil
}

func parseBuild(lines []string, i int) (map[string]BuildExpr, int, error) {
	line := strings.TrimSpace(lines[i])
	if !strings.Contains(line, "{") {
		return nil, i, fmt.Errorf("ser: build expects { at line %d", i+1)
	}
	out := map[string]BuildExpr{}
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
		colon := strings.Index(line, ":")
		if colon < 0 {
			return nil, i, fmt.Errorf("ser: build field needs colon at line %d", i+1)
		}
		key := strings.TrimSpace(line[:colon])
		raw := strings.TrimSpace(line[colon+1:])
		if pipe := strings.Index(raw, "|"); pipe >= 0 {
			raw = strings.TrimSpace(raw[:pipe])
		}
		toks := tokenize(raw)
		if len(toks) == 0 {
			out[key] = BuildExpr{}
		} else if strings.HasPrefix(toks[0], "\"") {
			out[key] = BuildExpr{Const: unquote(toks[0])}
		} else {
			out[key] = BuildExpr{Ref: unquote(toks[0])}
		}
		i++
	}
	return nil, i, fmt.Errorf("ser: unclosed build")
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

func isTop(s string) bool {
	switch s {
	case "rule", "fact", "endpoint", "find", "where", "when", "let", "build", "trace":
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
