// Package xapi is a read-only client over x.com's private GraphQL API.
//
// Every op's queryId, the ~39 features flags x.com validates, and a variables
// template live in ops.json (extracted from real captured sessions). The client
// fills the dynamic variables and replays the request. It never calls the
// Create*/Favorite*/Retweet write mutations.
package xapi

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed ops.json
var opsRaw []byte

// OpSpec is one GraphQL operation: its queryId, HTTP method, and the captured
// features/variables templates x.com validates.
type OpSpec struct {
	QueryID   string         `json:"queryId"`
	Method    string         `json:"method"`
	Variables map[string]any `json:"variables"`
	Features  map[string]any `json:"features"`
}

var ops map[string]OpSpec

func init() {
	if err := json.Unmarshal(opsRaw, &ops); err != nil {
		panic(fmt.Sprintf("xapi: cannot parse embedded ops.json: %v", err))
	}
}

// spec returns the operation template for op, or an error if it is not captured.
func spec(op string) (OpSpec, error) {
	s, ok := ops[op]
	if !ok {
		return OpSpec{}, fmt.Errorf("op %q not in ops.json — needs a fresh capture", op)
	}
	return s, nil
}
