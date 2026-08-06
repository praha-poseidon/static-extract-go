package extract

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/praha-poseidon/static-extract-go/internal/ast"
	"github.com/praha-poseidon/static-extract-go/internal/find"
	"github.com/praha-poseidon/static-extract-go/internal/ser"
	"golang.org/x/tools/go/packages"
)

type Fact struct {
	Rule             string            `json:"rule"`
	FactType         string            `json:"factType"`
	Classifiers      map[string]string `json:"classifiers"`
	Fields           map[string]string `json:"fields"`
	ProjectFilePath  string            `json:"projectFilePath"`
	AbsoluteFilePath string            `json:"absoluteFilePath"`
	StartLine        int               `json:"startLine"`
	EndLine          int               `json:"endLine"`
	EnclosingSymbol  string            `json:"enclosingSymbol,omitempty"`
}

type Request struct {
	ProjectRoot    string
	Patterns       []string
	RuleSources    []string
	Packages       []*packages.Package
	ExternalValues map[string]map[string][]string
}

func Run(req Request) ([]Fact, error) {
	var rules []*ser.Rule
	for _, src := range req.RuleSources {
		parsed, err := ser.ParseAll(src)
		if err != nil {
			return nil, err
		}
		rules = append(rules, parsed...)
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("pass at least one ruleSources SER text")
	}

	root := req.ProjectRoot
	if root != "" {
		if a, err := filepath.Abs(root); err == nil {
			root = a
		}
	}

	pkgs := req.Packages
	if len(pkgs) == 0 {
		var err error
		pkgs, err = ast.Load(ast.LoadConfig{Dir: root, Patterns: req.Patterns})
		if err != nil {
			return nil, err
		}
	}

	var facts []Fact
	for _, rule := range rules {
		for _, a := range find.FindAnchors(pkgs, rule.Find) {
			if !matchWhen(a, rule.When) {
				continue
			}
			fields := evalBuild(rule, a)
			pos := a.Pkg.Fset.Position(a.Node.Pos())
			end := a.Pkg.Fset.Position(a.Node.End())
			abs := pos.Filename
			if !filepath.IsAbs(abs) {
				if a2, err := filepath.Abs(abs); err == nil {
					abs = a2
				}
			}
			rel := abs
			if root != "" {
				if r, err := filepath.Rel(root, abs); err == nil {
					rel = r
				}
			}
			classifiers := map[string]string{}
			if rule.EndpointType != "" {
				classifiers["category"] = rule.EndpointType
				classifiers["direction"] = rule.EndpointDirection
			}
			facts = append(facts, Fact{
				Rule:             rule.Name,
				FactType:         rule.FactType,
				Classifiers:      classifiers,
				Fields:           fields,
				ProjectFilePath:  filepath.ToSlash(rel),
				AbsoluteFilePath: abs,
				StartLine:        pos.Line,
				EndLine:          end.Line,
			})
		}
	}
	return facts, nil
}

func matchWhen(a find.Anchor, whens [][]string) bool {
	for _, w := range whens {
		if len(w) == 0 {
			continue
		}
		joined := strings.Join(w, " ")
		if strings.Contains(joined, "receiver") && strings.Contains(joined, "type") {
			want := w[len(w)-1]
			if a.RecvType != want && !strings.HasSuffix(a.RecvType, "."+want) && base(a.RecvType) != want {
				return false
			}
			continue
		}
		if len(w) >= 2 && w[0] == "name" && a.Name != w[1] {
			return false
		}
	}
	return true
}

func base(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

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
		lets[let.Name] = val
	}
	out := map[string]string{}
	for k, expr := range rule.Build {
		if expr.Const != "" {
			out[k] = expr.Const
			continue
		}
		if v, ok := lets[expr.Ref]; ok {
			out[k] = v
		} else {
			out[k] = expr.Ref
		}
	}
	return out
}

func evalSource(a find.Anchor, src ser.Source) string {
	from, take := src.From, src.Take
	if len(from) >= 1 && (from[0] == "argument" || from[0] == "arg") {
		idx := 0
		if len(from) >= 2 {
			s := strings.Trim(from[1], "[]")
			n := 0
			ok := true
			for _, r := range s {
				if r < '0' || r > '9' {
					ok = false
					break
				}
				n = n*10 + int(r-'0')
			}
			if ok {
				idx = n
			}
		}
		if len(take) == 0 || take[0] == "value" {
			return find.ArgString(a, idx)
		}
	}
	if len(from) >= 1 && from[0] == "call" && len(take) > 0 && take[0] == "name" {
		return a.Name
	}
	if len(from) >= 2 && from[0] == "literal" {
		return strings.Trim(from[1], `"`)
	}
	return ""
}
