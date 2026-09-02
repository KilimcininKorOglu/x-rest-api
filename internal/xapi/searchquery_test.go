package xapi

import "testing"

func TestBuildSearchQuery(t *testing.T) {
	cases := []struct {
		name string
		f    SearchFilters
		want string
	}{
		{"raw only", SearchFilters{Query: "golang"}, "golang"},
		{"empty", SearchFilters{}, ""},
		{"min_likes -> min_faves", SearchFilters{Query: "go", MinLikes: 10}, "go min_faves:10"},
		{"from+since+images", SearchFilters{FromUsers: []string{"naval"}, Since: "2024-01-01", HasImages: true},
			"from:naval filter:images since:2024-01-01"},
		{"multi from OR", SearchFilters{FromUsers: []string{"a", "b"}}, "(from:a OR from:b)"},
		{"exclude + hashtags", SearchFilters{ExcludeWords: []string{"spam"}, HashtagsAny: []string{"ai", "$TSLA"}},
			`-spam (#ai OR $TSLA)`},
		{"tweet_type originals", SearchFilters{Query: "x", TweetType: "originals_only"}, "x -filter:replies -filter:retweets"},
		{"quote phrase", SearchFilters{ExactPhrases: []string{"hello world"}}, `("hello world")`},
		{"lang+until", SearchFilters{Query: "z", Lang: "en", Until: "2025-01-01"}, "z lang:en until:2025-01-01"},
		{"utc strip", SearchFilters{Query: "z", Since: "2024-01-01_UTC"}, "z since:2024-01-01"},
	}
	for _, c := range cases {
		if got := BuildSearchQuery(c.f); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestBuildSearchQueryNoDuplicate checks the guard: a user operator in the raw
// query suppresses the matching structured filter.
func TestBuildSearchQueryNoDuplicate(t *testing.T) {
	if got := BuildSearchQuery(SearchFilters{Query: "from:elon", FromUsers: []string{"naval"}}); got != "from:elon" {
		t.Errorf("from guard: got %q", got)
	}
	if got := BuildSearchQuery(SearchFilters{Query: "min_faves:5", MinLikes: 99}); got != "min_faves:5" {
		t.Errorf("min guard: got %q", got)
	}
	if got := BuildSearchQuery(SearchFilters{Query: "filter:videos", HasImages: true}); got != "filter:videos" {
		t.Errorf("filter guard: got %q", got)
	}
	// A bare word must not trip the from: guard.
	if got := BuildSearchQuery(SearchFilters{Query: "california", FromUsers: []string{"a"}}); got != "california from:a" {
		t.Errorf("word must not match operator guard: got %q", got)
	}
}

func TestBuildSearchQueryIDsAndList(t *testing.T) {
	got := BuildSearchQuery(SearchFilters{
		Query: "golang", List: "123", QuotedTweetID: "456", SinceID: "10", MaxID: "99",
	})
	want := "golang list:123 quoted_tweet_id:456 since_id:10 max_id:99"
	if got != want {
		t.Errorf("id/list ops: got %q want %q", got, want)
	}
	// Raw query already carrying an operator must not be duplicated.
	if got := BuildSearchQuery(SearchFilters{Query: "list:9", List: "123"}); got != "list:9" {
		t.Errorf("list guard: got %q", got)
	}
}
