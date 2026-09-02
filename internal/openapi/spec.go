package openapi

// Build assembles an OpenAPI 3.0.3 document from a route table. The response and
// request-body schemas are derived from the real Go types by reflection (see
// schema.go), so the document stays in sync with the code. Successful responses
// are wrapped as {"data": <schema>}; failures share one Error component, matching
// the server's writeData/writeError envelopes.

import (
	"reflect"
	"strconv"
	"strings"
)

// Param describes one query or header parameter (path params are derived from
// the route path automatically).
type Param struct {
	Name     string
	In       string // "query" or "header"
	Desc     string
	Required bool
	Type     string // "string" (default), "integer", or "boolean"
}

// Route is one operation's metadata. Response and RequestBody hold a sample value
// of the real type; only its reflect.Type is used, never the value.
type Route struct {
	Method      string
	Path        string // full path, e.g. /v1/users/{handle}/tweets
	Summary     string
	Tag         string
	Params      []Param
	RequestBody any // nil when the operation has no body
	Response    any // success payload placed under "data"; nil for an empty object
	Status      int // success status, default 200
	Secured     bool
}

// Build returns the full OpenAPI document as a JSON-serializable map.
func Build(title, version, serverURL string, routes []Route) map[string]any {
	reg := newRegistry()
	paths := map[string]any{}
	for _, rt := range routes {
		item, _ := paths[rt.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
		}
		item[strings.ToLower(rt.Method)] = operation(reg, rt)
		paths[rt.Path] = item
	}

	comps := reg.components()
	comps["Error"] = errorSchema()
	return map[string]any{
		"openapi": "3.0.3",
		"info":    map[string]any{"title": title, "version": version},
		"servers": []any{map[string]any{"url": serverURL}},
		"paths":   paths,
		"components": map[string]any{
			"schemas": comps,
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"},
			},
		},
	}
}

// operation renders a single method on a path.
func operation(reg *registry, rt Route) map[string]any {
	op := map[string]any{
		"summary":    rt.Summary,
		"parameters": parameters(rt),
		"responses":  responses(reg, rt),
	}
	if rt.Tag != "" {
		op["tags"] = []any{rt.Tag}
	}
	if rt.RequestBody != nil {
		op["requestBody"] = map[string]any{
			"required": true,
			"content":  jsonContent(reg.schemaFor(reflect.TypeOf(rt.RequestBody))),
		}
	}
	if rt.Secured {
		op["security"] = []any{map[string]any{"bearerAuth": []any{}}}
	}
	return op
}

// parameters merges auto-derived path params, the optional X-Account header, and
// the route's explicit query/header params.
func parameters(rt Route) []map[string]any {
	out := pathParams(rt.Path)
	out = append(out, map[string]any{
		"name": "X-Account", "in": "header", "required": false,
		"description": "Pin the request to a specific account label instead of rotating.",
		"schema":      map[string]any{"type": "string"},
	})
	for _, p := range rt.Params {
		out = append(out, paramNode(p))
	}
	return out
}

// pathParams extracts {name} tokens from a path as required string params.
func pathParams(path string) []map[string]any {
	var out []map[string]any
	for seg := range strings.SplitSeq(path, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			out = append(out, map[string]any{
				"name": seg[1 : len(seg)-1], "in": "path", "required": true,
				"schema": map[string]any{"type": "string"},
			})
		}
	}
	return out
}

func paramNode(p Param) map[string]any {
	typ := p.Type
	if typ == "" {
		typ = "string"
	}
	return map[string]any{
		"name": p.Name, "in": p.In, "required": p.Required,
		"description": p.Desc,
		"schema":      map[string]any{"type": typ},
	}
}

// responses builds the success (data-wrapped) and default (error) responses.
func responses(reg *registry, rt Route) map[string]any {
	status := rt.Status
	if status == 0 {
		status = 200
	}
	dataSchema := map[string]any{"type": "object"}
	if rt.Response != nil {
		dataSchema = reg.schemaFor(reflect.TypeOf(rt.Response))
	}
	return map[string]any{
		strconv.Itoa(status): map[string]any{
			"description": "Success",
			"content": jsonContent(map[string]any{
				"type":       "object",
				"properties": map[string]any{"data": dataSchema},
			}),
		},
		"default": map[string]any{
			"description": "Error",
			"content":     jsonContent(map[string]any{"$ref": "#/components/schemas/Error"}),
		},
	}
}

func jsonContent(schema map[string]any) map[string]any {
	return map[string]any{"application/json": map[string]any{"schema": schema}}
}

// errorSchema is the shared {"error": {"message": string}} envelope.
func errorSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"error": map[string]any{
				"type":       "object",
				"properties": map[string]any{"message": map[string]any{"type": "string"}},
			},
		},
	}
}
