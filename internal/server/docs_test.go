package server

import (
	"encoding/json"
	"testing"
)

// TestOpenAPISpec builds the real /v1 spec from the route table and asserts its
// shape offline (no network). It guards the data-wrapped responses, the shared
// Error component, the response schema components, and per-op bearer security.
func TestOpenAPISpec(t *testing.T) {
	s := &Server{}
	routes := s.v1Routes()
	s.buildSpec(routes)

	var doc map[string]any
	if err := json.Unmarshal(s.spec, &doc); err != nil {
		t.Fatalf("spec is not valid JSON: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Errorf("openapi version = %v, want 3.0.3", doc["openapi"])
	}

	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatalf("paths missing or empty")
	}

	ops := 0
	for path, raw := range paths {
		item := raw.(map[string]any)
		for method, rawOp := range item {
			ops++
			op := rawOp.(map[string]any)
			if _, ok := op["security"]; !ok {
				t.Errorf("%s %s: missing security (bearerAuth)", method, path)
			}
			assertDataWrapped(t, method, path, op)
		}
	}
	if ops != len(routes) {
		t.Errorf("operation count = %d, want %d", ops, len(routes))
	}

	comps := doc["components"].(map[string]any)
	schemas := comps["schemas"].(map[string]any)
	for _, name := range []string{"Tweet", "XUser", "TweetThread", "Error"} {
		if _, ok := schemas[name]; !ok {
			t.Errorf("components.schemas missing %q", name)
		}
	}
	ss := comps["securitySchemes"].(map[string]any)
	if _, ok := ss["bearerAuth"]; !ok {
		t.Errorf("securitySchemes missing bearerAuth")
	}
}

// assertDataWrapped verifies the success response's JSON schema wraps the payload
// under a "data" property.
func assertDataWrapped(t *testing.T, method, path string, op map[string]any) {
	t.Helper()
	resps := op["responses"].(map[string]any)
	for code, rawResp := range resps {
		if code == "default" {
			continue
		}
		schema := rawResp.(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("%s %s [%s]: success schema has no properties", method, path, code)
			return
		}
		if _, ok := props["data"]; !ok {
			t.Errorf("%s %s [%s]: success schema missing data wrapper", method, path, code)
		}
	}
}
