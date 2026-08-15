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

// packages import kept for Request.Packages type

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
	ProjectName    string // optional; default basename of ProjectRoot
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
	projectName := ProjectNameFromRoot(req.ProjectName, root)

	pkgs := req.Packages
	if len(pkgs) == 0 {
		var err error
		pkgs, err = ast.Load(ast.LoadConfig{Dir: root, Patterns: req.Patterns})
		if err != nil {
			return nil, err
		}
	}

	uriIndexByKey := map[string]int{}
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
			// Default direction into fields when SER build omitted it.
			if fields["direction"] == "" && classifiers["direction"] != "" {
				fields["direction"] = classifiers["direction"]
			}
			enclosing := enclosingFuncName(a.File, a.Node.Pos())
			// Declaration anchors: identity encloses the declared method itself (Java-like).
			if a.Kind == "method" || a.Kind == "func" {
				if a.RecvType != "" {
					enclosing = a.RecvType + "." + a.Name
				} else {
					enclosing = a.Name
				}
			}
			// Identity dict: key = {importPath}.{Recv?}.{Func}()
			callLabel := CallSiteLabel(a.Name, a.RecvType)
			importPath := "unknown"
			if a.Pkg != nil {
				if a.Pkg.PkgPath != "" {
					importPath = a.Pkg.PkgPath
				} else if a.Pkg.Types != nil && a.Pkg.Types.Path() != "" {
					importPath = a.Pkg.Types.Path()
				} else if a.Pkg.Name != "" {
					importPath = a.Pkg.Name
				}
			}
			ext := req.ExternalValues
			if len(rule.IdentityDict) > 0 {
				// SER-embedded dict takes precedence for this rule
				table := map[string][]string{}
				for k, v := range rule.IdentityDict {
					table[k] = []string{v}
				}
				ext = map[string]map[string][]string{IdentityNS: table}
			}
			ApplyPathDictWithRule(fields, classifiers, projectName, importPath, enclosing, callLabel, rule.Name, ext, uriIndexByKey)

			facts = append(facts, Fact{
				Rule:             rule.Name,
				FactType:         rule.FactType,
				Classifiers:      classifiers,
				Fields:           fields,
				ProjectFilePath:  filepath.ToSlash(rel),
				AbsoluteFilePath: abs,
				StartLine:        pos.Line,
				EndLine:          end.Line,
				EnclosingSymbol:  enclosing,
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

// evalBuild / evalSource → eval.go
