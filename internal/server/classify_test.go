package server

import (
	"testing"

	"x-rest-api/internal/xapi"
)

func TestClassifyUpstream(t *testing.T) {
	rlHas := &xapi.RateLimit{Remaining: 5}
	rlZero := &xapi.RateLimit{Remaining: 0, Reset: 1}
	cases := []struct {
		name string
		up   *xapi.UpstreamError
		rl   *xapi.RateLimit
		want upstreamKind
	}{
		{"auth code 32", &xapi.UpstreamError{Status: 401, Code: 32}, nil, kindBan},
		{"access denied 326", &xapi.UpstreamError{Status: 403, Code: 326}, nil, kindBan},
		{"code 88 with budget", &xapi.UpstreamError{Status: 429, Code: 88}, rlHas, kindBan},
		{"403 OK", &xapi.UpstreamError{Status: 403, Msg: "OK"}, nil, kindBan},
		{"features stale 336", &xapi.UpstreamError{Status: 400, Code: 336}, nil, kindFeaturesStale},
		{"loadshed -1", &xapi.UpstreamError{Status: 503, Code: -1}, nil, kindTransient},
		{"http 429", &xapi.UpstreamError{Status: 429}, nil, kindRateLimit},
		{"remaining 0", &xapi.UpstreamError{Status: 200}, rlZero, kindRateLimit},
		{"http 404", &xapi.UpstreamError{Status: 404}, nil, kindRateLimit},
		{"html block", &xapi.UpstreamError{Status: 403, HTML: true}, nil, kindHTMLBlock},
		{"unknown", &xapi.UpstreamError{Status: 500}, nil, kindOther},
	}
	for _, c := range cases {
		if got := classifyUpstream(c.up, c.rl); got != c.want {
			t.Errorf("%s: classifyUpstream = %d, want %d", c.name, got, c.want)
		}
	}
}
