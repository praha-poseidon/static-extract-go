package find

import "testing"

func TestMatchNameSupportsListsWithSpaces(t *testing.T) {
	if !matchName([]string{"[Get,", "Post]"}, "Post", "") {
		t.Fatal("unqualified method list should match")
	}
	if !matchName([]string{"redis.Client.[Get,", "Set]"}, "Set", "redis.Client") {
		t.Fatal("owner-qualified method list should match")
	}
	if matchName([]string{"redis.Client.[Get,", "Set]"}, "Get", "http.Header") {
		t.Fatal("owner-qualified method list must reject a different owner")
	}
}
