package xapi

import "testing"

func TestNormalizeHandle(t *testing.T) {
	cases := map[string]string{
		"naval":                         "naval",
		"@naval":                        "naval",
		"  @naval  ":                    "naval",
		"44196397":                      "44196397",
		"https://x.com/naval":           "naval",
		"https://www.x.com/naval":       "naval",
		"x.com/naval":                   "naval",
		"https://twitter.com/naval":     "naval",
		"https://x.com/naval/status/1":  "naval",
		"https://x.com/i/user/44196397": "44196397",
		"https://x.com/home":            "", // reserved path -> empty
		"https://x.com/i/lists/5":       "", // /i/ but not /i/user/<id>
		"https://example.com/naval":     "", // wrong host -> empty
	}
	for in, want := range cases {
		if got := normalizeHandle(in); got != want {
			t.Errorf("normalizeHandle(%q) = %q, want %q", in, got, want)
		}
	}
}
