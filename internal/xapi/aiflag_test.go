package xapi

import (
	"encoding/json"
	"testing"
)

// tweetResult decodes one tweet_results.result node for the parser under test.
func tweetResult(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// TestParseTweetIsAI covers both sources x.com uses to label AI-generated media:
// the content_disclosure block and a grok_post_id on a media entry. The fixtures
// are synthetic, because the repo is public.
func TestParseTweetIsAI(t *testing.T) {
	const base = `"__typename":"Tweet","rest_id":"1001",`

	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "disclosure flag set",
			raw: `{` + base + `"content_disclosure":{"ai_generated_disclosure":{"has_ai_generated_media":true}},
			       "legacy":{"id_str":"1001","full_text":"generated"}}`,
			want: true,
		},
		{
			name: "disclosure flag present but false",
			raw: `{` + base + `"content_disclosure":{"ai_generated_disclosure":{"has_ai_generated_media":false}},
			       "legacy":{"id_str":"1001","full_text":"plain"}}`,
			want: false,
		},
		{
			name: "grok_post_id on extended_entities",
			raw: `{` + base + `"legacy":{"id_str":"1001","full_text":"grok media","extended_entities":{"media":[
			       {"type":"photo","media_url_https":"https://pbs/one.jpg","grok_post_id":"11111111-2222-3333-4444-555555555555"}]}}}`,
			want: true,
		},
		{
			name: "grok_post_id on entities only",
			raw: `{` + base + `"legacy":{"id_str":"1001","full_text":"grok media","entities":{"media":[
			       {"type":"photo","media_url_https":"https://pbs/one.jpg","grok_post_id":"11111111-2222-3333-4444-555555555555"}]}}}`,
			want: true,
		},
		{
			name: "media with an empty grok_post_id",
			raw: `{` + base + `"legacy":{"id_str":"1001","full_text":"plain photo","extended_entities":{"media":[
			       {"type":"photo","media_url_https":"https://pbs/one.jpg","grok_post_id":""}]}}}`,
			want: false,
		},
		{
			name: "no disclosure and no media",
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
			if tw.IsAI != c.want {
				t.Errorf("IsAI = %v, want %v", tw.IsAI, c.want)
			}
		})
	}
}
