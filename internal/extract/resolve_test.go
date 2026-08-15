package extract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praha-poseidon/static-extract-go/internal/extract"
)

func writeMod(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/t\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChainPrevArgument(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir)
	src := `package t
type R struct{}
func (r *R) Group(p string) *R { return r }
func (r *R) GET(p string, h any) {}
func main() {
	var r R
	r.Group("/api").GET("/x", nil)
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	ser := `
rule "chain group get"
endpoint HTTP inbound
find call GET
let base =
  from chain prev argument[0] take value
  fallback ""
let path =
  from argument[0] take value
  fallback ""
build {
  path: concat(base, path) | normalize slash
}
`
	facts, err := extract.Run(extract.Request{
		ProjectRoot: dir,
		Patterns:    []string{"."},
		RuleSources: []string{ser},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("facts=%d %#v", len(facts), facts)
	}
	if facts[0].Fields["path"] != "/api/x" {
		t.Fatalf("path=%q want /api/x", facts[0].Fields["path"])
	}
}

func TestReceiverResolveDef(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir)
	src := `package t
type R struct{}
func (r *R) Group(p string) *R { return r }
func (r *R) GET(p string, h any) {}
func (r *R) POST(p string, h any) {}
func mount(r *R) {
	v1 := r.Group("/openapi/v1")
	v1.GET("/files", nil)
	v1.POST("/files/folds", nil)
	down := r.Group("/api/v1/files/downloads")
	down.GET("", nil)
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	serGET := `
rule "route GET"
endpoint HTTP inbound
find call GET
let base =
  from receiver resolve def argument[0] take value
  fallback ""
let path =
  from argument[0] take value
  fallback ""
build {
  httpMethod: "GET"
  path: concat(base, path) | normalize slash
}
`
	serPOST := `
rule "route POST"
endpoint HTTP inbound
find call POST
let base =
  from receiver resolve def argument[0] take value
  fallback ""
let path =
  from argument[0] take value
  fallback ""
build {
  httpMethod: "POST"
  path: concat(base, path) | normalize slash
}
`
	facts, err := extract.Run(extract.Request{
		ProjectRoot: dir,
		Patterns:    []string{"."},
		RuleSources: []string{serGET, serPOST},
	})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, f := range facts {
		paths[f.Fields["httpMethod"]+" "+f.Fields["path"]] = f.Fields["path"]
	}
	want := []string{
		"GET /openapi/v1/files",
		"POST /openapi/v1/files/folds",
		"GET /api/v1/files/downloads",
	}
	for _, w := range want {
		if _, ok := paths[w]; !ok {
			t.Fatalf("missing %q in %#v", w, paths)
		}
	}
}

func TestPlainArgumentStillWorks(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir)
	src := `package t
func HandleFunc(p string, h any) {}
func init() {
	HandleFunc("/api/users", nil)
	HandleFunc("/api/health", nil)
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	ser := `
rule "handle"
endpoint HTTP inbound
find call HandleFunc
let path = from argument 0 take value
build { path: path }
`
	facts, err := extract.Run(extract.Request{
		ProjectRoot: dir,
		Patterns:    []string{"."},
		RuleSources: []string{ser},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("got %d", len(facts))
	}
}
