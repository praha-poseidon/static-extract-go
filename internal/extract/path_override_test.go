package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMethodKeyNoProject(t *testing.T) {
	// importPath.Recv.Func()
	got := MethodKey("example.com/t/handler", "Server", "ListOrders", 0)
	if got != "example.com/t/handler.Server.ListOrders()" {
		t.Fatalf("key=%q", got)
	}
	// package func
	got0 := MethodKey("example.com/t", "", "ListOrders", 0)
	if got0 != "example.com/t.ListOrders()" {
		t.Fatalf("pkg func key=%q", got0)
	}
	got1 := MethodKey("example.com/t", "", "ListOrders", 1)
	if got1 != "example.com/t.ListOrders().1" {
		t.Fatalf("indexed key=%q", got1)
	}
}

func TestApplyPathDictHitMiss(t *testing.T) {
	key := "example.com/t.fetch()"
	counter := map[string]int{}
	fields := map[string]string{"direction": "outbound", "path": "buildUrl()"}
	classifiers := map[string]string{"category": "HTTP", "direction": "outbound"}
	external := map[string]map[string][]string{
		IdentityNS: {key: {"v1/items"}},
	}
	gotKey, hit := ApplyPathDict(fields, classifiers, "demo", "example.com/t", "fetch", "Get", external, counter)
	if !hit || gotKey != key {
		t.Fatalf("hit=%v key=%q", hit, gotKey)
	}
	if fields["path"] != "/v1/items" || fields["parseLevel"] != "config" {
		t.Fatalf("fields=%v", fields)
	}

	fields2 := map[string]string{"direction": "outbound", "path": "/api/clean"}
	counter2 := map[string]int{}
	_, hit2 := ApplyPathDict(fields2, classifiers, "demo", "example.com/t", "otherFn", "Post", external, counter2)
	if hit2 {
		t.Fatal("expected miss")
	}
	if fields2["path"] != "/api/clean" {
		t.Fatalf("MISS should keep SER path, got %q", fields2["path"])
	}
}

func TestInboundWithoutDictKeepsSer(t *testing.T) {
	fields := map[string]string{"direction": "inbound", "path": "/api/x"}
	classifiers := map[string]string{"category": "HTTP", "direction": "inbound"}
	counter := map[string]int{}
	_, hit := ApplyPathDict(fields, classifiers, "demo", "example.com/t", "Handle", "GET", nil, counter)
	if hit {
		t.Fatal("no dict should not hit")
	}
	if fields["path"] != "/api/x" {
		t.Fatalf("inbound path mutated: %v", fields)
	}
}

func writeGoModMain(t *testing.T, dir, mainSrc string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/t\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}
}

const outboundGetRule = `
rule "HTTP Get Outbound"
endpoint HTTP outbound
find call Get
let path =
  from argument[0] take value
build {
  path: path
  direction: "outbound"
}
`

func TestExtractPathDictHit(t *testing.T) {
	dir := t.TempDir()
	writeGoModMain(t, dir, `package main
import "net/http"
func load() { http.Get(buildURL()) }
func buildURL() string { return "x" }
`)
	// package main under module example.com/t → import path example.com/t
	key := "example.com/t.load()"
	facts, err := Run(Request{
		ProjectRoot: dir,
		ProjectName: "demo",
		Patterns:    []string{"./..."},
		RuleSources: []string{outboundGetRule},
		ExternalValues: map[string]map[string][]string{
			IdentityNS: {key: {"/v1/demo"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range facts {
		if f.Fields["path"] == "/v1/demo" && f.Fields["parseLevel"] == "config" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected dict path, facts=%#v", facts)
	}
}

func TestExtractMissKeepsLiteral(t *testing.T) {
	dir := t.TempDir()
	writeGoModMain(t, dir, `package main
import "net/http"
func load() { http.Get("/api/clean") }
`)
	facts, err := Run(Request{
		ProjectRoot: dir,
		ProjectName: "demo",
		Patterns:    []string{"./..."},
		RuleSources: []string{outboundGetRule},
		ExternalValues: map[string]map[string][]string{
			IdentityNS: {"other": {"/x"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var ok bool
	for _, f := range facts {
		if f.Fields["path"] == "/api/clean" {
			ok = true
		}
		if strings.HasPrefix(f.Fields["path"], "unresolved:") {
			t.Fatalf("should not force unresolved: %q", f.Fields["path"])
		}
	}
	if !ok {
		t.Fatalf("want SER literal kept, facts=%#v", facts)
	}
}

const inboundHandleRule = `
rule "HandleFunc inbound"
endpoint HTTP inbound
find call HandleFunc
let path =
  from argument[0] take value
build {
  path: path
  direction: "inbound"
  httpMethod: "ANY"
}
`

func TestExtractInboundDictHitAndMiss(t *testing.T) {
	dir := t.TempDir()
	writeGoModMain(t, dir, `package main
func HandleFunc(p string, h any) {}
func init() {
	HandleFunc("/legacy", nil)
	HandleFunc("/other", nil)
}
`)
	key0 := "example.com/t.init()"
	facts, err := Run(Request{
		ProjectRoot: dir,
		ProjectName: "demo",
		Patterns:    []string{"./..."},
		RuleSources: []string{inboundHandleRule},
		ExternalValues: map[string]map[string][]string{
			IdentityNS: {
				key0: {"/v1/from-dict"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var hit, other bool
	for _, f := range facts {
		if f.Fields["path"] == "/v1/from-dict" && f.Fields["parseLevel"] == "config" {
			hit = true
		}
		if f.Fields["path"] == "/other" {
			other = true
		}
	}
	if !hit {
		t.Fatalf("expected first call HIT, facts=%#v", facts)
	}
	if !other {
		t.Fatalf("expected second call keep SER, facts=%#v", facts)
	}
}
