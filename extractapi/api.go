// Package extractapi is the public library surface for static-extract-go.
package extractapi

import (
	"github.com/praha-poseidon/static-extract-go/internal/extract"
	"golang.org/x/tools/go/packages"
)

// Fact is one extracted fact.
type Fact = extract.Fact

// Request is one extract invocation.
type Request struct {
	ProjectRoot    string
	ProjectName    string // optional metadata; identity keys use importPath, not project
	Patterns       []string
	RuleSources    []string
	Packages       []*packages.Package
	ExternalValues map[string]map[string][]string
}

// Run executes SER rules against Go packages.
func Run(req Request) ([]Fact, error) {
	return extract.Run(extract.Request{
		ProjectRoot:    req.ProjectRoot,
		ProjectName:    req.ProjectName,
		Patterns:       req.Patterns,
		RuleSources:    req.RuleSources,
		Packages:       req.Packages,
		ExternalValues: req.ExternalValues,
	})
}
