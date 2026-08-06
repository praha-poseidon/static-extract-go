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
	fmt.Print(`Usage: static-extract-go <command> [options]

Commands: init | try | run

  --project <dir>
  --source <path>          repeatable; default ./...
  --rule <file>            SER file, repeatable
  --rule-text <ser>        inline SER, repeatable
  --external-values <json-or-file>
  --out <file>             optional JSONL
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
	outPath := fs.String("out", "", "")
	var sources, ruleFiles, ruleTexts []string
	var extVal string
	fs.Func("source", "", func(s string) error { sources = append(sources, s); return nil })
	fs.Func("rule", "", func(s string) error { ruleFiles = append(ruleFiles, s); return nil })
	fs.Func("rule-text", "", func(s string) error { ruleTexts = append(ruleTexts, s); return nil })
	fs.Func("rule-source", "", func(s string) error { ruleTexts = append(ruleTexts, s); return nil })
	fs.Func("external-values", "", func(s string) error { extVal = s; return nil })
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
		ProjectRoot: *project, Patterns: patterns, RuleSources: ruleTexts, ExternalValues: dict,
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
	var m map[string]map[string][]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}

func writeErr(err error) {
	_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"status": "ERROR", "message": err.Error()})
}
