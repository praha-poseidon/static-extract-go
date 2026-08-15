package ser

import "testing"

func TestParseLetMap(t *testing.T) {
	src := `
rule "t"
endpoint DB outbound
find method
let tableName =
  from method take name
  map {
    add: t_tenant
    update: t_tenant
  }
build {
  tableName: tableName
}
`
	r, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lets) != 1 {
		t.Fatalf("lets %d", len(r.Lets))
	}
	m := r.Lets[0].Map
	if m["add"] != "t_tenant" || m["update"] != "t_tenant" {
		t.Fatalf("map %#v", m)
	}
}

func TestParseDict(t *testing.T) {
	src := `
rule "t"
endpoint REDIS outbound
find call get
build { keyPattern: k }
dict {
  example.com/p.Foo.Bar() = cooper-auth_x_%s
  example.com/p.Foo.Bar().1 = cooper-auth_y
}
`
	r, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if r.IdentityDict["example.com/p.Foo.Bar()"] != "cooper-auth_x_%s" {
		t.Fatalf("%#v", r.IdentityDict)
	}
	if r.IdentityDict["example.com/p.Foo.Bar().1"] != "cooper-auth_y" {
		t.Fatalf("%#v", r.IdentityDict)
	}
}
