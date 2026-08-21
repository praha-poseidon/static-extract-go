# Go extractor vocabulary (v1)

AST: `go/packages` + `go/types`.

## Find

```ser
find call <name>
find call [GET,POST,PUT]
find func <name>
find method <name>
```

Call owners may be selected directly or filtered with `when`:

```ser
find call redis.Client.Get
find call redis.Client.[Get,Set]

# Equivalent filtering form
find call Get
when call owner redis.Client
```

## Filtering: `where` vs `when`

Filters run in this order: `find` → `where*` → `when*` → `let` → `build`.
Multiple `where` or `when` lines are ANDed.

| keyword | role | Go vocabulary |
|---------|------|---------------|
| `where` | scope: where the anchor lives | package path, enclosing/receiver type |
| `when` | conditions on the anchor itself | call name/owner, receiver type, name regex |

### Scope (`where`)

```ser
# Exact import path or path-segment suffix, matched against go/packages PkgPath
where package github.com/acme/service/handler
where package service/handler

# Go `type` is the counterpart of Java's enclosing `class` role.
# The Go extractor intentionally does not expose a `class` keyword.
where type name Handler
where type matches ".*Handler$"
where type name matches ".*Handler$"
```

For a call, type scope checks its enclosing method receiver and its actual call
receiver type. Package-level functions have no enclosing type.

### Anchor predicates (`when`)

```ser
when call name Get
when call name [Get,Post]
when call name matches "Get|Post"
when call owner redis.Client
when receiver type redis.Client
when name Get
when name matches "^G.*"
```

`when name <exact>` is retained for compatibility; `when call name ...` is the
clearer call-specific form for new rules.

`call owner` is the selected package identifier for package functions, or the
method receiver type for method calls. `receiver type` only matches a real
method receiver; it does not treat a package selector as a receiver.

Unsupported predicates and invalid regular expressions return an extraction
error instead of silently matching every anchor.

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
- `from method take value` reads the current function/method key from the rule's
  embedded `dict`. Keys are `import/path.Function()` or
  `import/path.Owner.Method()`.
- A declaration anchor fans out contiguous entries `key`, `key.1`, `key.2`, …
  into separate facts. A miss continues to the next `from` or `fallback`.
- `build { other: ... }` is preserved as endpoint metadata by the graph parser.
- `resolve def` is framework-agnostic def-use (same-file, lexical previous assign).
- Nested groups require multiple hops or nested SER; v1 one `resolve def` is one hop.
