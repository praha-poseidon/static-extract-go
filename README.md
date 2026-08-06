# static-extract-go

Go SER extractor. AST: `go/packages`. No built-in rules.

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go test ./...
go build -o bin/static-extract-go ./cmd/static-extract-go
./bin/static-extract-go run --project examples/conformance/http-handlefunc --source ./input --rule examples/conformance/http-handlefunc/rule.ser
```

## Library


Public package is  (not internal).

## Library

Public package: `extractapi` (callers must not import `internal/...`).

```go
import "github.com/praha-poseidon/static-extract-go/extractapi"

facts, err := extractapi.Run(extractapi.Request{
  ProjectRoot: "/path",
  RuleSources: []string{serText},
  // Packages: preloaded, // shared AST with code-graph-parser-go
})
```
