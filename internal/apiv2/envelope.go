// Package apiv2 renders x-rest-api's typed models into the official X API v2 wire
// shape: the {data, includes, meta, errors} envelope, field selection
// (tweet.fields/user.fields/expansions), and the v2 problem-details error body.
// It is a pure mapping layer over internal/xapi models; it performs no I/O except
// the optional expansion lookups that take an *xapi.XClient.
package apiv2

// Envelope is the top-level X API v2 response body. Exactly one of Data or Errors
// is set for a successful or failed request; Includes and Meta are optional.
type Envelope struct {
	Data     any        `json:"data,omitempty"`
	Includes *Includes  `json:"includes,omitempty"`
	Meta     *Meta      `json:"meta,omitempty"`
	Errors   []APIError `json:"errors,omitempty"`
}

// Includes carries expansion payloads keyed by object type, matching the v2
// includes object. Each entry is a field-selected object map.
type Includes struct {
	Users  []map[string]any `json:"users,omitempty"`
	Tweets []map[string]any `json:"tweets,omitempty"`
	Media  []map[string]any `json:"media,omitempty"`
	Polls  []map[string]any `json:"polls,omitempty"`
	Places []map[string]any `json:"places,omitempty"`
}

// empty reports whether the includes object holds nothing, so the caller can drop
// it from the envelope instead of emitting an empty object.
func (i *Includes) empty() bool {
	return i == nil || (len(i.Users) == 0 && len(i.Tweets) == 0 &&
		len(i.Media) == 0 && len(i.Polls) == 0 && len(i.Places) == 0)
}

// Meta is the pagination/result metadata on list endpoints.
type Meta struct {
	ResultCount int    `json:"result_count"`
	NextToken   string `json:"next_token,omitempty"`
	NewestID    string `json:"newest_id,omitempty"`
	OldestID    string `json:"oldest_id,omitempty"`
}

// APIError is one entry in the v2 problem-details errors array.
type APIError struct {
	Title        string `json:"title"`
	Detail       string `json:"detail,omitempty"`
	Type         string `json:"type,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Parameter    string `json:"parameter,omitempty"`
	Value        string `json:"value,omitempty"`
}

// typeResourceNotFound is the v2 problem type URI for a missing resource.
const typeResourceNotFound = "https://api.twitter.com/2/problems/resource-not-found"

// NotFound builds a resource-not-found error envelope for a single lookup, e.g. a
// tweet or user id/username that does not resolve.
func NotFound(resourceType, parameter, value string) Envelope {
	return Envelope{Errors: []APIError{{
		Title:        "Not Found Error",
		Detail:       "Could not find " + resourceType + " with " + parameter + ": [" + value + "].",
		Type:         typeResourceNotFound,
		ResourceType: resourceType,
		ResourceID:   value,
		Parameter:    parameter,
		Value:        value,
	}}}
}

// typeInvalidRequest is the v2 problem type URI for a malformed request.
const typeInvalidRequest = "https://api.twitter.com/2/problems/invalid-request"

// Invalid builds an invalid-request error envelope, e.g. a missing required
// parameter or a bad value.
func Invalid(parameter, detail string) Envelope {
	return Envelope{Errors: []APIError{{
		Title:     "Invalid Request",
		Detail:    detail,
		Type:      typeInvalidRequest,
		Parameter: parameter,
	}}}
}
