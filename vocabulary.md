# Go extractor vocabulary (v1)

AST: `go/packages` + `go/types`.

## Find

```ser
find call <name>
find call [GET,POST,PUT]
find func <name>
find method <name>
```

## Sources (`from … take …`)

```ser
from argument 0 take value
from argument[0] take value
from call take name
from literal "x" take value

# same-expression call chain (receiver side = prev)
from chain prev take name
from chain prev argument[0] take value
from chain previous take name
from chain prev Group argument[0] take value

# cross-statement: receiver identifier → assignment RHS call
from receiver resolve def argument[0] take value
```

## Build

```ser
build {
  path: concat(base, path) | normalize slash
  path: concat(base, path) | normalize slash | normalize pathVariable
  httpMethod: "GET"
}
```

`normalize extractPath` strips `scheme://host` / `lb://svc` prefixes when present.

## Notes

- No built-in framework rules. Pass SER via `--rule` / `ruleSources`.
- `resolve def` is framework-agnostic def-use (same-file, lexical previous assign).
- Nested groups require multiple hops or nested SER; v1 one `resolve def` is one hop.
