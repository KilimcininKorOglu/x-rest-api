package apiv2

import "time"

// xLayout is the timestamp format x.com's GraphQL/legacy surface returns, e.g.
// "Wed Oct 10 20:19:24 +0000 2018".
const xLayout = "Mon Jan 02 15:04:05 -0700 2006"

// v2Layout is the ISO 8601 millisecond-UTC format X API v2 emits for created_at,
// e.g. "2018-10-10T20:19:24.000Z".
const v2Layout = "2006-01-02T15:04:05.000Z07:00"

// toISO8601 converts an x.com timestamp to the v2 ISO 8601 form. It returns "" when
// the input is empty or unparseable, so the caller can drop the field rather than
// emit a malformed value.
func toISO8601(x string) string {
	if x == "" {
		return ""
	}
	t, err := time.Parse(xLayout, x)
	if err != nil {
		return ""
	}
	return t.UTC().Format(v2Layout)
}
