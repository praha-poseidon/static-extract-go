package extract

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Identity dictionary (redesign) — applied after SER take/build.
//
// User-facing dict: flat only
//
//	{
//	  "git.example.com/org/proj/handler.Server.ListOrders()": "/api/orders"
//	}
//
// Key (Go): {importPath}.{RecvType?}.{Func}()
//   - RecvType without leading *
//   - no project prefix, no endpointPathOverrides wrapper, no value arrays
//
// Multi-hit: ...Func().1, ...Func().2
//
// HIT → parseLevel=config; MISS → keep SER fields

const IdentityNS = "identity"

// MethodKey builds a collision-resistant identity key (no project name).
// recvType empty → package function; non-empty → method on type.
func MethodKey(importPath, recvType, funcName string, siteIndex int) string {
	importPath = blankToUnknown(importPath)
	funcName = blankToUnknown(funcName)
	recvType = strings.TrimSpace(strings.TrimPrefix(recvType, "*"))
	// A declaration receiver is local to its package. go/types may render it as
	// "pkg.Type"; the package/import path is already the key prefix.
	if dot := strings.LastIndex(recvType, "."); dot >= 0 {
		recvType = recvType[dot+1:]
	}
	var base string
	if recvType == "" {
		base = fmt.Sprintf("%s.%s()", importPath, funcName)
	} else {
		base = fmt.Sprintf("%s.%s.%s()", importPath, recvType, funcName)
	}
	if siteIndex > 0 {
		return base + "." + itoa(siteIndex)
	}
	return base
}

// PathSiteKey ignored site; uses MethodKey(importPath, "", enclosingFunc, siteIndex) for package funcs.
// Prefer MethodKey with explicit recv when available.
func PathSiteKey(project, pkg, enclosingFunc, site string, siteIndex int) string {
	// project ignored (redesign); pkg treated as import path
	return MethodKey(pkg, "", enclosingFunc, siteIndex)
}

// OutboundCallKey: project ignored; call ignored for key.
func OutboundCallKey(project, pkg, enclosingFunc, call string, uriIndex int) string {
	return MethodKey(pkg, "", enclosingFunc, uriIndex)
}

// identityTable extracts flat methodKey -> value from external map.
// Wire: map["identity"][key] = []string{value}
// Also accepts if external itself is only identity-level (host may pass wire form only).
func identityTable(external map[string]map[string][]string) map[string]string {
	if external == nil {
		return nil
	}
	table := external[IdentityNS]
	if len(table) == 0 {
		return nil
	}
	out := make(map[string]string, len(table))
	for k, vals := range table {
		if p := firstNonBlank(vals); p != "" {
			out[k] = p
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// LookupOverridePath looks up identity value by full method key.
func LookupOverridePath(external map[string]map[string][]string, key string) string {
	if key == "" {
		return ""
	}
	t := identityTable(external)
	if t == nil {
		return ""
	}
	return t[key]
}

// ApplyPathDict optionally overrides identity field from dictionary.
func ApplyPathDict(
	fields map[string]string,
	classifiers map[string]string,
	projectName, pkg, enclosing, call string,
	external map[string]map[string][]string,
	uriIndexByKey map[string]int,
) (key string, hit bool) {
	return ApplyPathDictWithRule(fields, classifiers, projectName, pkg, enclosing, call, "", external, uriIndexByKey)
}

// ApplyPathDictWithRule applies method-level identity override.
// pkg = import path; enclosing = Func or Type.Func; recv is parsed from enclosing if it contains '.'
func ApplyPathDictWithRule(
	fields map[string]string,
	classifiers map[string]string,
	projectName, pkg, enclosing, call, ruleName string,
	external map[string]map[string][]string,
	uriIndexByKey map[string]int,
) (key string, hit bool) {
	if !hasIdentityField(fields) {
		return "", false
	}
	if identityTable(external) == nil {
		return "", false
	}

	recv, fn := splitRecvFunc(enclosing)
	methodScope := pkg + "#" + enclosing
	idx := 0
	if uriIndexByKey != nil {
		idx = uriIndexByKey[methodScope]
		uriIndexByKey[methodScope] = idx + 1
	}

	k := MethodKey(pkg, recv, fn, idx)
	if override := LookupOverridePath(external, k); override != "" {
		setIdentityField(fields, classifiers, override)
		fields["parseLevel"] = "config"
		fields["pathKey"] = k
		return k, true
	}
	fields["pathKey"] = k
	return k, false
}

// ApplyOutboundPathDict is an alias of ApplyPathDict.
func ApplyOutboundPathDict(
	fields map[string]string,
	classifiers map[string]string,
	projectName, pkg, enclosing, call string,
	external map[string]map[string][]string,
	uriIndexByKey map[string]int,
) (key string, hit bool) {
	return ApplyPathDict(fields, classifiers, projectName, pkg, enclosing, call, external, uriIndexByKey)
}

func splitRecvFunc(enclosing string) (recv, fn string) {
	enclosing = strings.TrimSpace(enclosing)
	if enclosing == "" {
		return "", "unknown"
	}
	// "Server.ListOrders" or "ListOrders"
	if i := strings.LastIndex(enclosing, "."); i >= 0 {
		return strings.TrimPrefix(enclosing[:i], "*"), enclosing[i+1:]
	}
	return "", enclosing
}

func hasIdentityField(fields map[string]string) bool {
	// Key present is enough (even empty after map miss) — Java still overrides by endpoint type.
	for _, k := range []string{"path", "url", "route", "topic", "key", "keyPattern", "table", "tableName", "text", "label", "uiText", "routePath"} {
		if _, ok := fields[k]; ok {
			return true
		}
	}
	return fields["endpointType"] != "" || fields["direction"] != ""
}

func setIdentityField(fields, classifiers map[string]string, override string) {
	v := strings.TrimSpace(override)
	cat := strings.ToUpper(classifiers["category"])
	if cat == "" {
		cat = strings.ToUpper(fields["endpointType"])
	}
	switch cat {
	case "MQ":
		fields["topic"] = v
	case "REDIS":
		fields["keyPattern"] = v
		fields["key"] = v
	case "DB":
		fields["tableName"] = v
		fields["table"] = v
	default:
		fields["path"] = v
	}
}

func ProjectNameFromRoot(projectName, projectRoot string) string {
	if strings.TrimSpace(projectName) != "" {
		return strings.TrimSpace(projectName)
	}
	if projectRoot == "" {
		return "unknown"
	}
	base := filepath.Base(filepath.Clean(projectRoot))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "unknown"
	}
	return base
}

func CallSiteLabel(name, recv string) string {
	name = strings.TrimSpace(name)
	recv = strings.TrimSpace(recv)
	if name == "" {
		return "call"
	}
	if recv == "" {
		return name
	}
	if i := strings.LastIndex(recv, "."); i >= 0 {
		recv = recv[i+1:]
	}
	recv = strings.TrimPrefix(recv, "*")
	return recv + "." + name
}

func firstNonBlank(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func blankToUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return strings.TrimSpace(s)
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
