package extract_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/praha-poseidon/static-extract-go/internal/extract"
)

func TestExtractHTTPHandleFunc(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "../../examples/conformance/http-handlefunc")
	ser, err := os.ReadFile(filepath.Join(root, "rule.ser"))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := extract.Run(extract.Request{
		ProjectRoot: root,
		Patterns:    []string{"./input"},
		RuleSources: []string{string(ser)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) < 2 {
		t.Fatalf("want >=2 facts, got %d: %#v", len(facts), facts)
	}
	paths := map[string]bool{}
	for _, f := range facts {
		paths[f.Fields["path"]] = true
	}
	if !paths["/api/users"] || !paths["/api/health"] {
		t.Fatalf("paths: %#v", paths)
	}
}
