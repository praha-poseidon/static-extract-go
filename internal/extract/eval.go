package extract

import (
	"go/ast"
	"strconv"
	"strings"
	"unicode"

	"github.com/praha-poseidon/static-extract-go/internal/find"
	"github.com/praha-poseidon/static-extract-go/internal/resolve"
	"github.com/praha-poseidon/static-extract-go/internal/ser"
)

func evalBuild(rule *ser.Rule, a find.Anchor) map[string]string {
	lets := map[string]string{}
	for _, let := range rule.Lets {
		val := ""
		for _, src := range let.Sources {
			val = evalSource(a, src)
			if val != "" {
				break
			}
		}
		if val == "" {
			val = let.Fallback
		}
		// SER map { } — aligned with Java ValueSupport: empty table no-op; miss → ""
		val = applyMapping(val, let.Map)
		lets[let.Name] = val
	}
	out := map[string]string{}
	for k, expr := range rule.Build {
		out[k] = evalBuildExpr(expr, lets)
	}
	return out
}

// applyMapping matches Java: empty/nil map leaves value; non-empty miss → "".
func applyMapping(val string, entries map[string]string) string {
	if len(entries) == 0 {
		return val
	}
	if v, ok := entries[val]; ok {
		return v
	}
	return ""
}

func evalBuildExpr(expr ser.BuildExpr, lets map[string]string) string {
	raw := strings.TrimSpace(expr.Raw)
	if raw == "" {
		if expr.Const != "" {
			return expr.Const
		}
		if v, ok := lets[expr.Ref]; ok {
			return v
		}
		return expr.Ref
	}
	// split pipeline: base | op args | op args
	parts := splitPipeline(raw)
	val := evalValueExpr(parts[0], lets)
	for _, step := range parts[1:] {
		val = applyPipelineStep(val, step)
	}
	return val
}

func splitPipeline(s string) []string {
	var parts []string
	var b strings.Builder
	depthParen := 0
	depthBrace := 0
	for _, r := range s {
		switch r {
		case '(':
			depthParen++
			b.WriteRune(r)
		case ')':
			depthParen--
			b.WriteRune(r)
		case '{':
			depthBrace++
			b.WriteRune(r)
		case '}':
			depthBrace--
			b.WriteRune(r)
		case '|':
			if depthParen == 0 && depthBrace == 0 {
				parts = append(parts, strings.TrimSpace(b.String()))
				b.Reset()
				continue
			}
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		parts = append(parts, strings.TrimSpace(b.String()))
	}
	return parts
}

func evalValueExpr(expr string, lets map[string]string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	if strings.HasPrefix(expr, "\"") {
		return unquote(expr)
	}
	if strings.HasPrefix(expr, "concat(") && strings.HasSuffix(expr, ")") {
		inner := expr[len("concat(") : len(expr)-1]
		args := splitArgs(inner)
		var b strings.Builder
		for _, a := range args {
			b.WriteString(evalValueExpr(strings.TrimSpace(a), lets))
		}
		return b.String()
	}
	// bare let ref or literal word
	if v, ok := lets[expr]; ok {
		return v
	}
	return expr
}

func splitArgs(s string) []string {
	var args []string
	var b strings.Builder
	depth := 0
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
			b.WriteRune(r)
		case !inQ && r == '(':
			depth++
			b.WriteRune(r)
		case !inQ && r == ')':
			depth--
			b.WriteRune(r)
		case !inQ && depth == 0 && r == ',':
			args = append(args, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		args = append(args, b.String())
	}
	return args
}

func applyPipelineStep(val, step string) string {
	step = strings.TrimSpace(step)
	toks := strings.Fields(step)
	if len(toks) == 0 {
		return val
	}
	switch toks[0] {
	case "normalize":
		if len(toks) < 2 {
			return val
		}
		switch toks[1] {
		case "slash":
			return normalizeSlash(val)
		case "pathVariable":
			return normalizePathVariable(val)
		case "extractPath":
			return extractPath(val)
		case "upper":
			return strings.ToUpper(val)
		case "lower":
			return strings.ToLower(val)
		case "trim":
			return strings.TrimSpace(val)
		default:
			return val
		}
	case "map":
		// | map { k: v ... }  (aligned with Java pipeline map)
		entries := parseInlineMap(step)
		return applyMapping(val, entries)
	case "replace":
		// | replace old new  (two tokens after replace) or replace "old" "new"
		if len(toks) >= 3 {
			return strings.ReplaceAll(val, unquote(toks[1]), unquote(toks[2]))
		}
		return val
	default:
		return val
	}
}

// parseInlineMap extracts key:value pairs from a pipeline step like `map { get: GET set: SET }`.
func parseInlineMap(step string) map[string]string {
	out := map[string]string{}
	brace := strings.Index(step, "{")
	if brace < 0 {
		return out
	}
	end := strings.LastIndex(step, "}")
	if end <= brace {
		return out
	}
	inner := strings.TrimSpace(step[brace+1 : end])
	_ = parseMapEntriesInto(inner, out)
	return out
}

func parseMapEntriesInto(inner string, out map[string]string) error {
	// reuse ser-style key: value pairs (whitespace separated entries)
	// single-line: get: GET  set: SET
	parts := splitTopLevelMapPairs(inner)
	for _, p := range parts {
		colon := strings.Index(p, ":")
		if colon <= 0 {
			continue
		}
		k := strings.TrimSpace(p[:colon])
		v := unquote(strings.TrimSpace(p[colon+1:]))
		if k != "" {
			out[k] = v
		}
	}
	return nil
}

func splitTopLevelMapPairs(s string) []string {
	var entries []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			continue
		}
		j := i + 1
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
		rest := s[j:]
		if c := strings.Index(rest, ":"); c > 0 {
			key := strings.TrimSpace(rest[:c])
			if key != "" && !strings.ContainsAny(key, " \t") {
				entries = append(entries, strings.TrimSpace(s[start:i]))
				start = j
			}
		}
	}
	if start < len(s) {
		entries = append(entries, strings.TrimSpace(s[start:]))
	}
	if len(entries) == 0 && strings.TrimSpace(s) != "" {
		return []string{strings.TrimSpace(s)}
	}
	return entries
}

func normalizeSlash(s string) string {
	if s == "" {
		return s
	}
	// collapse // ; ensure single leading slash if path-like
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	// join semantics: if we concatenated base+path without slash, fix common case
	return s
}

func normalizePathVariable(s string) string {
	// :id -> {id} lightly
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == ':' && i+1 < len(s) && (unicode.IsLetter(rune(s[i+1])) || s[i+1] == '_') {
			j := i + 1
			for j < len(s) && (unicode.IsLetter(rune(s[j])) || unicode.IsDigit(rune(s[j])) || s[j] == '_') {
				j++
			}
			b.WriteByte('{')
			b.WriteString(s[i+1 : j])
			b.WriteByte('}')
			i = j - 1
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func extractPath(s string) string {
	// strip scheme://host
	if i := strings.Index(s, "://"); i >= 0 {
		rest := s[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			return rest[j:]
		}
		return "/"
	}
	if strings.HasPrefix(s, "lb://") {
		rest := s[len("lb://"):]
		if j := strings.Index(rest, "/"); j >= 0 {
			return rest[j:]
		}
	}
	return s
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	return s
}

// evalSource interprets from … take … with chain / receiver resolve def / argument.
func evalSource(a find.Anchor, src ser.Source) string {
	from := src.From
	take := src.Take
	if len(from) == 0 {
		return ""
	}

	// Declaration anchors (class/interface methods, package funcs) — parity with Java find method
	if a.Kind == "method" || a.Kind == "func" {
		// from method take name  |  from func take name  |  from name take name
		if from[0] == "method" || from[0] == "func" || from[0] == "name" {
			if len(take) == 0 || take[0] == "name" {
				return a.Name
			}
		}
		if len(take) > 0 && take[0] == "name" && from[0] != "argument" && !strings.HasPrefix(from[0], "argument") {
			// from method ... take name already handled; keep fallback for "from name"
			if from[0] == "method" || from[0] == "func" {
				return a.Name
			}
		}
	}

	f, ok := resolve.FocusFromAnchor(a)
	if !ok {
		if from[0] == "literal" && len(from) >= 2 {
			return strings.Trim(from[1], `"`)
		}
		return ""
	}

	i := 0
	// F2: chain prev|next|previous [name|N]
	if from[i] == "chain" {
		i++
		if i >= len(from) {
			return ""
		}
		dir := from[i]
		i++
		steps := 1
		nameFilter := ""
		// optional steps count or call-name filter; not site tokens like argument[0]
		if i < len(from) && !isSiteToken(from[i]) {
			if n, err := strconv.Atoi(from[i]); err == nil {
				steps = n
				i++
			} else {
				nameFilter = strings.Trim(from[i], "[]")
				i++
			}
		}
		f = resolve.NavigateChain(f, dir, steps, nameFilter)
		if f.Call == nil {
			return ""
		}
	}

	// F3: receiver resolve def [resolve def ...] then site
	// also: from receiver resolve def argument[0]
	for i < len(from) && from[i] == "receiver" {
		i++
		// optional resolve def hops
		for i+1 < len(from) && from[i] == "resolve" && (from[i+1] == "def" || from[i+1] == "def*") {
			star := from[i+1] == "def*"
			i += 2
			recv := resolve.ReceiverExpr(f.Call)
			if recv == nil {
				return ""
			}
			if star {
				// walk collecting — for single take argument later we only move focus to outermost call
				rhs := resolve.ResolveDef(f.Pkg, f.File, recv, 8)
				if nf, ok := resolve.FocusFromExpr(f.Pkg, f.File, rhs); ok {
					f = nf
				} else if ce, ok := rhs.(*ast.CallExpr); ok {
					f = resolve.Focus{Pkg: f.Pkg, File: f.File, Call: ce, Name: resolve.CallName(ce)}
				} else {
					return ""
				}
			} else {
				rhs := resolve.ResolveDef(f.Pkg, f.File, recv, 4)
				if nf, ok := resolve.FocusFromExpr(f.Pkg, f.File, rhs); ok {
					f = nf
				} else if ce, ok := asCall(rhs); ok {
					f = resolve.Focus{Pkg: f.Pkg, File: f.File, Call: ce, Name: resolve.CallName(ce)}
				} else {
					// resolved to non-call (e.g. just ident) — cannot take argument
					return ""
				}
			}
		}
		// if only "receiver" without resolve, focus becomes... not a call; name take from receiver ident
		if i < len(from) && from[i] != "argument" && from[i] != "arg" && from[i] != "call" {
			// fall through if next is argument on current focus after resolve
		}
	}

	// site
	if i < len(from) && (from[i] == "argument" || from[i] == "arg") {
		idx := 0
		if i+1 < len(from) {
			idx = parseIndex(from[i+1])
		} else if strings.Contains(from[i], "[") {
			// argument[0] glued
			idx = parseIndex(from[i])
		}
		if len(take) == 0 || take[0] == "value" {
			return resolve.ArgString(f, idx)
		}
	}

	// from argument[0] without separate token
	if i < len(from) && strings.HasPrefix(from[i], "argument[") {
		idx := parseIndex(from[i])
		if len(take) == 0 || take[0] == "value" {
			return resolve.ArgString(f, idx)
		}
	}

	if i < len(from) && from[i] == "call" {
		if len(take) > 0 && take[0] == "name" {
			return f.Name
		}
	}

	// from chain ... take name  (no argument)
	if len(take) > 0 && take[0] == "name" {
		return f.Name
	}

	if from[0] == "literal" && len(from) >= 2 {
		return strings.Trim(from[1], `"`)
	}

	// plain from argument at start (no chain)
	if from[0] == "argument" || from[0] == "arg" {
		idx := 0
		if len(from) >= 2 {
			idx = parseIndex(from[1])
		}
		if len(take) == 0 || take[0] == "value" {
			return resolve.ArgString(f, idx)
		}
	}
	if strings.HasPrefix(from[0], "argument[") {
		if len(take) == 0 || take[0] == "value" {
			return resolve.ArgString(f, parseIndex(from[0]))
		}
	}
	if from[0] == "call" && len(take) > 0 && take[0] == "name" {
		return f.Name
	}

	return ""
}

func asCall(e ast.Expr) (*ast.CallExpr, bool) {
	switch v := e.(type) {
	case *ast.CallExpr:
		return v, true
	case *ast.ParenExpr:
		return asCall(v.X)
	default:
		return nil, false
	}
}

func isSiteToken(s string) bool {
	switch s {
	case "argument", "arg", "call", "receiver", "literal":
		return true
	}
	if strings.HasPrefix(s, "argument[") || strings.HasPrefix(s, "arg[") {
		return true
	}
	return false
}

func parseIndex(s string) int {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "argument")
	s = strings.Trim(s, "[]")
	n := 0
	ok := false
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		ok = true
		n = n*10 + int(r-'0')
	}
	if !ok {
		return 0
	}
	return n
}
