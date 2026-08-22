package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praha-poseidon/static-extract-go/internal/extract"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "run", "try":
		if err := runExtract(os.Args[2:]); err != nil {
			writeErr(err)
			os.Exit(1)
		}
	case "init":
		if err := runInit(os.Args[2:]); err != nil {
			writeErr(err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	default:
		writeErr(fmt.Errorf("unknown command: %s", os.Args[1]))
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`Usage: extract-go <command> [options]

Commands: init | try | run

Shared extraction options (aligned with extract-java CLI):
  --project <dir>          Project root (required for try/run).
  --project-name <name>    Optional project name (identity keys use importPath.Func()).
  --source <path>          Source pattern/dir; repeatable; default ./{...}
  --rule <file>            SER rule file (may include trace { } and dict { }); repeatable
  --rule-text <ser>        Inline SER (may include dict { }); repeatable
  --rule-source <ser>      Alias for --rule-text
  --external-values <json-or-file>
                           Identity dict: flat JSON {"importPath.Type.Func()": "value"}
                           as file path or inline (same role as Java --external-values).
  --dictionary <json-or-file>
                           Alias for --external-values.
  --out <file>             Optional JSONL output (run).
`)
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	project := fs.String("project", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *project == "" {
		return fmt.Errorf("missing --project")
	}
	for _, d := range []string{".ser/generated", ".ser/rules", ".ser/out"} {
		if err := os.MkdirAll(filepath.Join(*project, d), 0o755); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "OK", "project": *project})
}

func runExtract(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	project := fs.String("project", "", "")
	projectName := fs.String("project-name", "", "")
	outPath := fs.String("out", "", "")
	var sources, ruleFiles, ruleTexts []string
	var extVal string
	fs.Func("source", "", func(s string) error { sources = append(sources, s); return nil })
	fs.Func("rule", "", func(s string) error { ruleFiles = append(ruleFiles, s); return nil })
	fs.Func("rule-text", "", func(s string) error { ruleTexts = append(ruleTexts, s); return nil })
	fs.Func("rule-source", "", func(s string) error { ruleTexts = append(ruleTexts, s); return nil })
	fs.Func("external-values", "", func(s string) error { extVal = s; return nil })
	fs.Func("dictionary", "", func(s string) error { extVal = s; return nil })
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *project == "" {
		return fmt.Errorf("missing --project")
	}
	for _, f := range ruleFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		ruleTexts = append(ruleTexts, string(b))
	}
	if len(ruleTexts) == 0 {
		return fmt.Errorf("pass at least one --rule or --rule-text")
	}
	patterns := sources
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	dict, err := loadDict(extVal)
	if err != nil {
		return err
	}
	facts, err := extract.Run(extract.Request{
		ProjectRoot:    *project,
		ProjectName:    *projectName,
		Patterns:       patterns,
		RuleSources:    ruleTexts,
		ExternalValues: dict,
	})
	if err != nil {
		return err
	}
	if *outPath != "" {
		var lines []string
		for _, f := range facts {
			b, _ := json.Marshal(f)
			lines = append(lines, string(b))
		}
		_ = os.MkdirAll(filepath.Dir(*outPath), 0o755)
		if err := os.WriteFile(*outPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			return err
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"status": "OK", "resultCount": len(facts), "results": facts})
}

// loadDict accepts flat identity JSON only:
//
//	{ "example.com/t.Handler.Send()": "topic_or_path" }
//
// Internally converted to wire map under "identity" (single-element string slices).
func loadDict(raw string) (map[string]map[string][]string, error) {
	if raw == "" {
		return map[string]map[string][]string{}, nil
	}
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "{") {
		b, err := os.ReadFile(s)
		if err != nil {
			return nil, err
		}
		s = string(b)
	}
	var flat map[string]any
	if err := json.Unmarshal([]byte(s), &flat); err != nil {
		return nil, fmt.Errorf("identity dict must be flat JSON object: %w", err)
	}
	if _, ok := flat["endpointPathOverrides"]; ok {
		return nil, fmt.Errorf("endpointPathOverrides is not supported; use flat {\"importPath.Func()\": \"value\"}")
	}
	if _, ok := flat["config"]; ok {
		return nil, fmt.Errorf("config namespace is not supported in identity dict")
	}
	table := map[string][]string{}
	for k, v := range flat {
		if k == "identity" {
			// optional wire form: { "identity": { "key()": "v" } } or { "identity": { "key()": ["v"] } }
			sub, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("identity must be an object of string values")
			}
			for sk, sv := range sub {
				if s := anyToString(sv); s != "" {
					table[sk] = []string{s}
				}
			}
			continue
		}
		// flat method keys only
		if !strings.Contains(k, "()") {
			return nil, fmt.Errorf("identity key must look like method key ending with (): %q", k)
		}
		s := anyToString(v)
		if s == "" {
			return nil, fmt.Errorf("identity value must be a non-empty string: key=%q", k)
		}
		table[k] = []string{s}
	}
	if len(table) == 0 {
		return map[string]map[string][]string{}, nil
	}
	return map[string]map[string][]string{extract.IdentityNS: table}, nil
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case []any:
		if len(t) == 0 {
			return ""
		}
		if s, ok := t[0].(string); ok {
			return strings.TrimSpace(s)
		}
		return ""
	default:
		return ""
	}
}

func writeErr(err error) {
	_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"status": "ERROR", "message": err.Error()})
}
