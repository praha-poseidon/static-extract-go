package extract_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/praha-poseidon/static-extract-go/internal/extract"
)

func TestSERFiltersDisambiguateSameNamedCalls(t *testing.T) {
	root := filterFixtureRoot(t)
	tests := []struct {
		name      string
		predicate string
		wantFile  string
	}{
		{
			name:      "call owner",
			predicate: "when call owner redis.Client",
			wantFile:  "input/redis/client.go",
		},
		{
			name:      "receiver type",
			predicate: "when receiver type redis.Client",
			wantFile:  "input/redis/client.go",
		},
		{
			name:      "enclosing type",
			predicate: "where type name RedisHandler",
			wantFile:  "input/redis/client.go",
		},
		{
			name:      "package suffix",
			predicate: "where package redis",
			wantFile:  "input/redis/client.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := extract.Run(extract.Request{
				ProjectRoot: root,
				Patterns:    []string{"./input/..."},
				RuleSources: []string{filterRule(tt.predicate)},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(facts) != 1 {
				t.Fatalf("want one filtered fact, got %d: %#v", len(facts), facts)
			}
			if facts[0].ProjectFilePath != tt.wantFile {
				t.Fatalf("want %q, got %q", tt.wantFile, facts[0].ProjectFilePath)
			}
		})
	}
}

func TestSERWithoutWhereOrWhenKeepsPreviousBehavior(t *testing.T) {
	facts, err := extract.Run(extract.Request{
		ProjectRoot: filterFixtureRoot(t),
		Patterns:    []string{"./input/..."},
		RuleSources: []string{filterRule("")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("unfiltered rule should keep both Get calls, got %d: %#v", len(facts), facts)
	}
}

func TestFindAndWhenSupportPackageCallOwner(t *testing.T) {
	tests := []string{
		"find call redis.Lookup",
		"find call Lookup\nwhen call owner redis",
	}
	for _, selector := range tests {
		facts, err := extract.Run(extract.Request{
			ProjectRoot: filterFixtureRoot(t),
			Patterns:    []string{"./input/..."},
			RuleSources: []string{`
rule "package owner"
fact call
` + selector + `
build {
  method: "Lookup"
}
`},
		})
		if err != nil {
			t.Fatalf("%q: %v", selector, err)
		}
		if len(facts) != 1 || facts[0].ProjectFilePath != "input/app/app.go" {
			t.Fatalf("%q: facts=%#v", selector, facts)
		}
	}
}

func filterFixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../examples/conformance/filters"))
}

func filterRule(predicate string) string {
	return `
rule "filtered Get calls"
fact call

find call Get
` + predicate + `

build {
  method: "Get"
}
`
}
