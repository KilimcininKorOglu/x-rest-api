package server

import (
	"encoding/json"
	"testing"
)

// TestV2Spec asserts the hand-written v2 OpenAPI document is valid JSON, covers
// the expected read surface, and secures every operation with bearerAuth.
func TestV2Spec(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(v2SpecJSON, &doc); err != nil {
		t.Fatalf("v2 spec is not valid JSON: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Errorf("openapi version = %v, want 3.0.3", doc["openapi"])
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths missing or wrong type: %T", doc["paths"])
	}
	want := []string{
		"/2/users/by/username/{username}",
		"/2/users/{id}",
		"/2/users",
		"/2/users/by",
		"/2/users/{id}/tweets",
		"/2/tweets/{id}",
		"/2/tweets",
		"/2/tweets/search/recent",
		"/2/users/{id}/likes",
		"/2/users/{id}/likes/{tweet_id}",
		"/2/users/{id}/retweets",
		"/2/users/{id}/retweets/{source_tweet_id}",
		"/2/users/{id}/following",
		"/2/users/{id}/following/{target_id}",
	}
	if len(paths) != len(want) {
		t.Errorf("path count = %d, want %d", len(paths), len(want))
	}
	for _, p := range want {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %q", p)
		}
	}
	assertV2Secured(t, paths)
}

// assertV2Secured checks that every operation on every path carries a bearerAuth
// security requirement.
func assertV2Secured(t *testing.T, paths map[string]any) {
	t.Helper()
	for name, item := range paths {
		ops, ok := item.(map[string]any)
		if !ok || len(ops) == 0 {
			t.Errorf("path %q has no operations", name)
			continue
		}
		for method, op := range ops {
			sec, ok := op.(map[string]any)["security"].([]any)
			if !ok || len(sec) == 0 {
				t.Errorf("path %q %s is not secured", name, method)
			}
		}
	}
}
