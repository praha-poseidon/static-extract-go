package extract

import (
	"testing"

	"github.com/praha-poseidon/static-extract-go/internal/find"
	"golang.org/x/tools/go/packages"
)

func TestWherePackageMatchesExactOrPathSuffix(t *testing.T) {
	anchor := find.Anchor{Pkg: &packages.Package{PkgPath: "example.com/acme/service/handler"}}
	for _, predicate := range [][][]string{
		{{"package", "example.com/acme/service/handler"}},
		{{"package", "service/handler"}},
		{{"package", "handler"}},
	} {
		matched, err := matchWhere(anchor, predicate)
		if err != nil || !matched {
			t.Fatalf("predicate %#v: matched=%v err=%v", predicate, matched, err)
		}
	}

	matched, err := matchWhere(anchor, [][]string{{"package", "other/handler"}})
	if err != nil {
		t.Fatal(err)
	}
	if matched {
		t.Fatal("unexpected package suffix match")
	}
}

func TestWhereTypeUsesEnclosingOrReceiverType(t *testing.T) {
	anchor := find.Anchor{EnclosingType: "api.Handler", ReceiverType: "redis.Client"}
	checks := [][][]string{
		{{"type", "name", "Handler"}},
		{{"type", "name", "redis.Client"}},
		{{"type", "matches", `^api\..*`}},
		{{"type", "name", "matches", `Client$`}},
	}
	for _, predicate := range checks {
		matched, err := matchWhere(anchor, predicate)
		if err != nil || !matched {
			t.Fatalf("predicate %#v: matched=%v err=%v", predicate, matched, err)
		}
	}
}

func TestWhenCallOwnerDisambiguatesSameName(t *testing.T) {
	redisGet := find.Anchor{
		Kind: "call", Name: "Get", CallOwner: "redis.Client", ReceiverType: "redis.Client",
	}
	headerGet := find.Anchor{
		Kind: "call", Name: "Get", CallOwner: "http.Header", ReceiverType: "http.Header",
	}
	predicate := [][]string{{"call", "name", "Get"}, {"call", "owner", "redis.Client"}}

	matched, err := matchWhen(redisGet, predicate)
	if err != nil || !matched {
		t.Fatalf("redis Get should match: matched=%v err=%v", matched, err)
	}
	matched, err = matchWhen(headerGet, predicate)
	if err != nil {
		t.Fatal(err)
	}
	if matched {
		t.Fatal("http.Header.Get must be excluded by call owner")
	}
}

func TestWhenSupportsNameListReceiverAndRegex(t *testing.T) {
	anchor := find.Anchor{
		Kind: "call", Name: "Get", CallOwner: "redis.Client", ReceiverType: "redis.Client",
	}
	predicates := [][][]string{
		{{"call", "name", "[Post,", "Get]"}},
		{{"receiver", "type", "Client"}},
		{{"name", "Get"}},
		{{"name", "matches", `^G.t$`}},
		{{"call", "name", "matches", `Get|Post`}},
	}
	for _, predicate := range predicates {
		matched, err := matchWhen(anchor, predicate)
		if err != nil || !matched {
			t.Fatalf("predicate %#v: matched=%v err=%v", predicate, matched, err)
		}
	}
}

func TestNoWhereOrWhenPreservesMatch(t *testing.T) {
	anchor := find.Anchor{Kind: "call", Name: "Anything"}
	whereMatched, err := matchWhere(anchor, nil)
	if err != nil || !whereMatched {
		t.Fatalf("empty where should match: matched=%v err=%v", whereMatched, err)
	}
	whenMatched, err := matchWhen(anchor, nil)
	if err != nil || !whenMatched {
		t.Fatalf("empty when should match: matched=%v err=%v", whenMatched, err)
	}
}

func TestUnsupportedAndInvalidFilterPredicatesFailClosed(t *testing.T) {
	anchor := find.Anchor{Kind: "call", Name: "Get", EnclosingType: "Handler"}
	if _, err := matchWhere(anchor, [][]string{{"class", "name", "Handler"}}); err == nil {
		t.Fatal("unsupported class predicate should fail")
	}
	if _, err := matchWhen(anchor, [][]string{{"annotation", "Route"}}); err == nil {
		t.Fatal("unsupported when predicate should fail")
	}
	if _, err := matchWhere(anchor, [][]string{{"type", "matches", "["}}); err == nil {
		t.Fatal("invalid regex should fail")
	}
	if err := validateFilters(nil, [][]string{{"call", "name", "matches", "["}}); err == nil {
		t.Fatal("invalid call-name regex should fail even without an anchor")
	}
}
