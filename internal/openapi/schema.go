package openapi

// JSON Schema generation via reflection. A Go value's type is walked once and
// turned into an OpenAPI 3.0 schema node. Named structs become reusable
// components (referenced with $ref); everything else is inlined. This keeps the
// spec in sync with the real response types with no hand-written schema.

import (
	"reflect"
	"strings"
)

// registry collects named component schemas discovered while walking types.
type registry struct {
	schemas map[string]map[string]any
}

func newRegistry() *registry {
	return &registry{schemas: map[string]map[string]any{}}
}

// schemas returns the collected components/schemas map.
func (reg *registry) components() map[string]any {
	out := map[string]any{}
	for k, v := range reg.schemas {
		out[k] = v
	}
	return out
}

// schemaFor returns a schema node for t. A named struct is registered once and
// returned as a $ref, so recursive and shared types resolve to one definition.
func (reg *registry) schemaFor(t reflect.Type) map[string]any {
	t = deref(t)
	switch t.Kind() {
	case reflect.Struct:
		if t.Name() == "" {
			return reg.structSchema(t)
		}
		return reg.namedStruct(t)
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": reg.schemaFor(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": reg.schemaFor(t.Elem())}
	case reflect.Interface:
		return map[string]any{} // empty schema = any
	default:
		return scalarSchema(t.Kind())
	}
}

// namedStruct registers a named struct once (reserving its slot first, so a
// self-referential type does not recurse forever) and returns a $ref to it.
func (reg *registry) namedStruct(t reflect.Type) map[string]any {
	name := t.Name()
	if _, ok := reg.schemas[name]; !ok {
		reg.schemas[name] = map[string]any{} // reserve to break recursion
		reg.schemas[name] = reg.structSchema(t)
	}
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

// structSchema builds an object schema from a struct's exported JSON fields.
func (reg *registry) structSchema(t reflect.Type) map[string]any {
	props := map[string]any{}
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		name, ok := jsonFieldName(f.Tag.Get("json"), f.Name)
		if !ok {
			continue // json:"-"
		}
		props[name] = reg.schemaFor(f.Type)
	}
	return map[string]any{"type": "object", "properties": props}
}

// scalarSchema maps a primitive kind to its OpenAPI type.
func scalarSchema(k reflect.Kind) map[string]any {
	switch k {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	default:
		return map[string]any{} // unknown -> any
	}
}

// deref unwraps pointer types to their element.
func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// jsonFieldName resolves a field's JSON name from its tag, falling back to the
// Go field name. It returns ok=false when the field is skipped (json:"-").
func jsonFieldName(tag, fallback string) (string, bool) {
	if tag == "" {
		return fallback, true
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return "", false
	}
	if name == "" {
		return fallback, true
	}
	return name, true
}
