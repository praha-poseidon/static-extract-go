package extract

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/praha-poseidon/static-extract-go/internal/find"
)

func validateFilters(wheres, whens [][]string) error {
	empty := find.Anchor{}
	for _, predicate := range wheres {
		if _, err := evaluateWhere(empty, predicate); err != nil {
			return err
		}
	}
	for _, predicate := range whens {
		if _, err := evaluateWhen(empty, predicate); err != nil {
			return err
		}
	}
	return nil
}

// matchWhere evaluates scope predicates. All predicates are ANDed.
func matchWhere(a find.Anchor, predicates [][]string) (bool, error) {
	for _, predicate := range predicates {
		matched, err := evaluateWhere(a, predicate)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func evaluateWhere(a find.Anchor, predicate []string) (bool, error) {
	if len(predicate) == 0 {
		return false, fmt.Errorf("empty where predicate")
	}
	switch predicate[0] {
	case "package":
		if len(predicate) != 2 {
			return false, unsupportedPredicate("where", predicate)
		}
		return packageMatches(packagePath(a), atom(predicate[1])), nil
	case "type":
		return evaluateTypeWhere(a, predicate[1:])
	default:
		return false, unsupportedPredicate("where", predicate)
	}
}

func evaluateTypeWhere(a find.Anchor, predicate []string) (bool, error) {
	candidates := nonEmpty(a.EnclosingType, a.ReceiverType)
	if len(predicate) == 2 && predicate[0] == "name" {
		return anyIdentityMatches(candidates, atom(predicate[1])), nil
	}
	if len(predicate) == 2 && predicate[0] == "matches" {
		return anyRegexMatches(candidates, atom(predicate[1]))
	}
	if len(predicate) == 3 && predicate[0] == "name" && predicate[1] == "matches" {
		return anyRegexMatches(candidates, atom(predicate[2]))
	}
	return false, unsupportedPredicate("where type", predicate)
}

// matchWhen evaluates predicates on the selected anchor. All predicates are ANDed.
func matchWhen(a find.Anchor, predicates [][]string) (bool, error) {
	for _, predicate := range predicates {
		matched, err := evaluateWhen(a, predicate)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func evaluateWhen(a find.Anchor, predicate []string) (bool, error) {
	if len(predicate) == 0 {
		return false, fmt.Errorf("empty when predicate")
	}
	if len(predicate) >= 3 && predicate[0] == "call" && predicate[1] == "name" {
		if predicate[2] == "matches" {
			if len(predicate) != 4 {
				return false, unsupportedPredicate("when call name", predicate[2:])
			}
			matched, err := regexMatches(a.Name, atom(predicate[3]))
			return a.Kind == "call" && matched, err
		}
		if len(predicate) != 3 && !(strings.HasPrefix(predicate[2], "[") && strings.HasSuffix(predicate[len(predicate)-1], "]")) {
			return false, unsupportedPredicate("when call name", predicate[2:])
		}
		return a.Kind == "call" && nameListMatches(a.Name, predicate[2:]), nil
	}
	if len(predicate) == 3 && predicate[0] == "call" && predicate[1] == "owner" {
		return a.Kind == "call" && identityMatches(a.CallOwner, atom(predicate[2])), nil
	}
	if len(predicate) == 3 && predicate[0] == "receiver" && predicate[1] == "type" {
		return identityMatches(a.ReceiverType, atom(predicate[2])), nil
	}
	if len(predicate) == 2 && predicate[0] == "name" {
		return a.Name == atom(predicate[1]), nil
	}
	if len(predicate) == 3 && predicate[0] == "name" && predicate[1] == "matches" {
		return regexMatches(a.Name, atom(predicate[2]))
	}
	return false, unsupportedPredicate("when", predicate)
}

func nameListMatches(actual string, atoms []string) bool {
	raw := strings.TrimSpace(strings.Join(atoms, " "))
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	for _, candidate := range strings.Split(raw, ",") {
		candidate = atom(strings.TrimSpace(candidate))
		if candidate == "*" || actual == candidate {
			return true
		}
	}
	return false
}

func packagePath(a find.Anchor) string {
	if a.Pkg == nil {
		return ""
	}
	if a.Pkg.PkgPath != "" {
		return a.Pkg.PkgPath
	}
	if a.Pkg.Types != nil {
		return a.Pkg.Types.Path()
	}
	return ""
}

func packageMatches(actual, wanted string) bool {
	actual = strings.TrimSuffix(strings.TrimSpace(actual), "/")
	wanted = strings.Trim(strings.TrimSpace(wanted), "/")
	return actual != "" && wanted != "" && (actual == wanted || strings.HasSuffix(actual, "/"+wanted))
}

func anyIdentityMatches(actual []string, wanted string) bool {
	for _, candidate := range actual {
		if identityMatches(candidate, wanted) {
			return true
		}
	}
	return false
}

func identityMatches(actual, wanted string) bool {
	actual = normalizeIdentity(actual)
	wanted = normalizeIdentity(wanted)
	if actual == "" || wanted == "" {
		return false
	}
	return actual == wanted ||
		strings.HasSuffix(actual, "."+wanted) ||
		strings.HasSuffix(actual, "/"+wanted) ||
		identityBase(actual) == wanted
}

func normalizeIdentity(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "*")
}

func identityBase(value string) string {
	if slash := strings.LastIndex(value, "/"); slash >= 0 {
		value = value[slash+1:]
	}
	if dot := strings.LastIndex(value, "."); dot >= 0 {
		return value[dot+1:]
	}
	return value
}

func anyRegexMatches(actual []string, pattern string) (bool, error) {
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid regex %q: %w", pattern, err)
	}
	for _, candidate := range actual {
		if compiled.MatchString(candidate) {
			return true, nil
		}
	}
	return false, nil
}

func regexMatches(actual, pattern string) (bool, error) {
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid regex %q: %w", pattern, err)
	}
	return compiled.MatchString(actual), nil
}

func atom(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	return value
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = normalizeIdentity(value)
		if value != "" && !seen[value] {
			out = append(out, value)
			seen[value] = true
		}
	}
	return out
}

func unsupportedPredicate(kind string, predicate []string) error {
	return fmt.Errorf("unsupported %s predicate: %s", kind, strings.Join(predicate, " "))
}
