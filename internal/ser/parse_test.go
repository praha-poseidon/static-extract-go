package ser

import "testing"

func TestParseHandleFuncRule(t *testing.T) {
	src := `
rule "net/http HandleFunc inbound"
endpoint HTTP inbound

find call HandleFunc

let path =
  from argument 0 take value

build {
  httpMethod: "ANY"
  path: path
}
`
	r, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "net/http HandleFunc inbound" {
		t.Fatalf("name: %q", r.Name)
	}
	if r.Find[0] != "call" || r.Find[1] != "HandleFunc" {
		t.Fatalf("find: %#v", r.Find)
	}
	if r.Build["httpMethod"].Const != "ANY" {
		t.Fatalf("build method: %#v", r.Build["httpMethod"])
	}
}

func TestParsePreservesWhereAndWhenFreeAtoms(t *testing.T) {
	src := `
rule "filtered calls"
fact call

find call Get
where package service/handler
where type name matches ".*Handler$"
when call name [Get, Post]
when call owner redis.Client

build {
  name: "matched"
}
`
	r, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Where) != 2 || len(r.When) != 2 {
		t.Fatalf("where=%#v when=%#v", r.Where, r.When)
	}
	if got := r.Where[1]; len(got) != 4 || got[0] != "type" || got[3] != `".*Handler$"` {
		t.Fatalf("where tokens: %#v", got)
	}
	if got := r.When[0]; len(got) != 4 || got[0] != "call" || got[2] != "[Get," || got[3] != "Post]" {
		t.Fatalf("when tokens: %#v", got)
	}
}
