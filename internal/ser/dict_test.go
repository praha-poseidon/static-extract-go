package ser

import "testing"

func TestDictBlock(t *testing.T) {
  r, err := Parse(`
rule "get"
endpoint HTTP outbound
find call Get
let path =
  from argument[0] take value
build {
  path: path
}
dict {
  example.com/t.load() = /v1/demo
}
`)
  if err != nil {
    t.Fatal(err)
  }
  if r.IdentityDict["example.com/t.load()"] != "/v1/demo" {
    t.Fatalf("%#v", r.IdentityDict)
  }
}
