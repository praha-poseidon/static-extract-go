// Package ast loads Go packages with types (go/packages).
package ast

import (
	"fmt"
	"path/filepath"

	"golang.org/x/tools/go/packages"
)

type LoadConfig struct {
	Dir      string
	Patterns []string
	Tests    bool
}

func Load(cfg LoadConfig) ([]*packages.Package, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("ast: Dir is required")
	}
	patterns := cfg.Patterns
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	abs, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return nil, err
	}
	mode := packages.NeedName |
		packages.NeedFiles |
		packages.NeedCompiledGoFiles |
		packages.NeedSyntax |
		packages.NeedTypes |
		packages.NeedTypesInfo |
		packages.NeedTypesSizes |
		packages.NeedModule |
		packages.NeedImports
	pkgs, err := packages.Load(&packages.Config{
		Mode:  mode,
		Dir:   abs,
		Tests: cfg.Tests,
	}, patterns...)
	if err != nil {
		return nil, err
	}
	var out []*packages.Package
	var firstErr error
	for _, p := range pkgs {
		if len(p.Errors) > 0 && firstErr == nil {
			firstErr = fmt.Errorf("%s: %v", p.PkgPath, p.Errors[0])
		}
		if len(p.Syntax) == 0 {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, fmt.Errorf("ast: no packages loaded from %s %v", abs, patterns)
	}
	return out, nil
}
