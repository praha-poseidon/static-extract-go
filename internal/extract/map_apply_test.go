package extract

import "testing"

func TestApplyMapping(t *testing.T) {
	if applyMapping("get", map[string]string{"get": "GET"}) != "GET" {
		t.Fatal("hit")
	}
	if applyMapping("foo", map[string]string{"get": "GET"}) != "" {
		t.Fatal("miss should be empty")
	}
	if applyMapping("foo", nil) != "foo" {
		t.Fatal("empty map passthrough")
	}
	if applyMapping("x", map[string]string{}) != "x" {
		t.Fatal("empty map passthrough 2")
	}
}

func TestPipelineMap(t *testing.T) {
	v := applyPipelineStep("get", "map { get: GET set: SET }")
	if v != "GET" {
		t.Fatalf("got %q", v)
	}
	v = applyPipelineStep("nope", "map { get: GET }")
	if v != "" {
		t.Fatalf("miss %q", v)
	}
}
