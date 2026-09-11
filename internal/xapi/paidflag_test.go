package xapi

import "testing"

// TestParseTweetIsPaidPartnership covers the advertising_disclosure block x.com
// sets on a tweet it labels "Paid partnership". It sits beside the AI disclosure
// under the same content_disclosure node. The fixtures are synthetic, because the
// repo is public.
func TestParseTweetIsPaidPartnership(t *testing.T) {
	const base = `"__typename":"Tweet","rest_id":"1001",`

	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "paid promotion flag set",
			raw: `{` + base + `"content_disclosure":{"advertising_disclosure":{"is_paid_promotion":true}},
			       "legacy":{"id_str":"1001","full_text":"sponsored"}}`,
			want: true,
		},
		{
			name: "paid promotion flag present but false",
			raw: `{` + base + `"content_disclosure":{"advertising_disclosure":{"is_paid_promotion":false}},
			       "legacy":{"id_str":"1001","full_text":"plain"}}`,
			want: false,
		},
		{
			name: "only the AI disclosure is set",
			raw: `{` + base + `"content_disclosure":{"ai_generated_disclosure":{"has_ai_generated_media":true}},
			       "legacy":{"id_str":"1001","full_text":"generated"}}`,
			want: false,
		},
		{
			name: "no disclosure block",
			raw:  `{` + base + `"legacy":{"id_str":"1001","full_text":"plain"}}`,
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tw := parseTweet(tweetResult(t, c.raw))
			if tw == nil {
				t.Fatal("parseTweet returned nil")
			}
			if tw.IsPaidPartnership != c.want {
				t.Errorf("IsPaidPartnership = %v, want %v", tw.IsPaidPartnership, c.want)
			}
		})
	}
}
