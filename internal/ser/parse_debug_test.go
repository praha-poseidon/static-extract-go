
package ser

import "testing"

func TestParseArg0(t *testing.T) {
  r, err := Parse(`
rule "get"
fact http_call
find call Get
let path = from argument[0] take value
build { path: path }
`)
  if err != nil { t.Fatal(err) }
  t.Logf("From=%#v Take=%#v", r.Lets[0].Sources[0].From, r.Lets[0].Sources[0].Take)
}
