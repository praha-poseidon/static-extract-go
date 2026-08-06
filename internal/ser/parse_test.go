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
