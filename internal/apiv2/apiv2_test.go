package apiv2

import (
	"net/url"
	"testing"

	"x-rest-api/internal/xapi"
)

// parseFrom builds a Selection from a raw query string.
func parseFrom(t *testing.T, raw string) Selection {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query %q: %v", raw, err)
	}
	return ParseSelection(q)
}

func TestParseSelectionDefaults(t *testing.T) {
	sel := parseFrom(t, "")
	for _, k := range defaultTweetFields {
		if !sel.Tweet[k] {
			t.Errorf("default tweet field %q missing", k)
		}
	}
	for _, k := range defaultUserFields {
		if !sel.User[k] {
			t.Errorf("default user field %q missing", k)
		}
	}
	if len(sel.Expansions) != 0 {
		t.Errorf("expected no expansions, got %v", sel.Expansions)
	}
}

func TestParseSelectionAddsAndTrims(t *testing.T) {
	sel := parseFrom(t, "tweet.fields=created_at,+public_metrics+&expansions=author_id")
	if !sel.Tweet["created_at"] || !sel.Tweet["public_metrics"] {
		t.Errorf("requested tweet fields not merged: %v", sel.Tweet)
	}
	if !sel.Tweet["id"] {
		t.Error("default id dropped after merge")
	}
	if !sel.Expansions["author_id"] {
		t.Error("author_id expansion not parsed")
	}
}

func TestTweetObjectDefaultSet(t *testing.T) {
	tw := xapi.Tweet{RestID: "123", Text: "hi", CreatedAt: "Wed Oct 10 20:19:24 +0000 2018", Lang: "en"}
	obj := TweetObject(tw, parseFrom(t, ""))
	if obj["id"] != "123" || obj["text"] != "hi" {
		t.Fatalf("default fields wrong: %v", obj)
	}
	if _, ok := obj["edit_history_tweet_ids"]; !ok {
		t.Error("edit_history_tweet_ids always required")
	}
	if _, ok := obj["created_at"]; ok {
		t.Error("created_at emitted without being selected")
	}
	if _, ok := obj["lang"]; ok {
		t.Error("lang emitted without being selected")
	}
}

func TestTweetObjectSelectedFields(t *testing.T) {
	tw := xapi.Tweet{
		RestID: "1", Text: "x", AuthorID: "9",
		CreatedAt: "Wed Oct 10 20:19:24 +0000 2018", Lang: "en",
		LikeCount: 5, ViewCount: 7,
	}
	obj := TweetObject(tw, parseFrom(t, "tweet.fields=created_at,author_id,lang,public_metrics"))
	if obj["created_at"] != "2018-10-10T20:19:24.000Z" {
		t.Errorf("created_at not ISO 8601: %v", obj["created_at"])
	}
	if obj["author_id"] != "9" || obj["lang"] != "en" {
		t.Errorf("selected scalars wrong: %v", obj)
	}
	pm, ok := obj["public_metrics"].(map[string]any)
	if !ok {
		t.Fatalf("public_metrics missing: %v", obj)
	}
	if pm["like_count"] != 5 || pm["impression_count"] != 7 {
		t.Errorf("public_metrics mapping wrong: %v", pm)
	}
}

func TestTweetObjectUnknownFieldIgnored(t *testing.T) {
	tw := xapi.Tweet{RestID: "1", Text: "x"}
	obj := TweetObject(tw, parseFrom(t, "tweet.fields=possibly_sensitive,context_annotations"))
	if _, ok := obj["possibly_sensitive"]; ok {
		t.Error("field with no source value must not be emitted")
	}
	if len(obj) != 3 {
		t.Errorf("expected only the default set, got %v", obj)
	}
}

func TestUserObjectSelectedFields(t *testing.T) {
	u := xapi.XUser{
		RestID: "44", Name: "Jack", ScreenName: "jack",
		Description: "ceo", Protected: false, Verified: true,
		FollowersCount: 100, FriendsCount: 3, StatusesCount: 9,
		PinnedTweetIDs: []string{"555"},
	}
	obj := UserObject(u, parseFrom(t, "user.fields=description,verified,public_metrics,pinned_tweet_id"))
	if obj["id"] != "44" || obj["username"] != "jack" {
		t.Fatalf("default user fields wrong: %v", obj)
	}
	if obj["description"] != "ceo" || obj["verified"] != true {
		t.Errorf("selected user fields wrong: %v", obj)
	}
	if obj["pinned_tweet_id"] != "555" {
		t.Errorf("pinned_tweet_id wrong: %v", obj["pinned_tweet_id"])
	}
	pm, ok := obj["public_metrics"].(map[string]any)
	if !ok || pm["following_count"] != 3 || pm["tweet_count"] != 9 {
		t.Errorf("user public_metrics mapping wrong: %v", obj["public_metrics"])
	}
}

func TestToISO8601(t *testing.T) {
	cases := map[string]string{
		"Wed Oct 10 20:19:24 +0000 2018": "2018-10-10T20:19:24.000Z",
		"":                               "",
		"not a date":                     "",
	}
	for in, want := range cases {
		if got := toISO8601(in); got != want {
			t.Errorf("toISO8601(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestResolveReferencedTweetsNoClient(t *testing.T) {
	tweets := []xapi.Tweet{{
		RestID:    "1",
		Text:      "quote",
		Quoted:    &xapi.Tweet{RestID: "2", Text: "orig"},
		Retweeted: &xapi.Tweet{RestID: "3", Text: "rt"},
	}}
	sel := parseFrom(t, "expansions=referenced_tweets.id")
	inc, err := Resolve(tweets, nil, sel, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if inc == nil || len(inc.Tweets) != 2 {
		t.Fatalf("expected 2 referenced tweets, got %v", inc)
	}
}

func TestResolveDedupesReferencedTweets(t *testing.T) {
	shared := &xapi.Tweet{RestID: "9", Text: "shared"}
	tweets := []xapi.Tweet{
		{RestID: "1", Quoted: shared},
		{RestID: "2", Quoted: shared},
	}
	sel := parseFrom(t, "expansions=referenced_tweets.id")
	inc, err := Resolve(tweets, nil, sel, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if inc == nil || len(inc.Tweets) != 1 {
		t.Fatalf("expected 1 deduped referenced tweet, got %v", inc)
	}
}

func TestResolveEmptyWhenNoExpansion(t *testing.T) {
	tweets := []xapi.Tweet{{RestID: "1", Quoted: &xapi.Tweet{RestID: "2"}}}
	inc, err := Resolve(tweets, nil, parseFrom(t, ""), nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if inc != nil {
		t.Errorf("expected nil includes without expansions, got %v", inc)
	}
}

func TestNotFoundEnvelope(t *testing.T) {
	env := NotFound("tweet", "id", "123")
	if len(env.Errors) != 1 {
		t.Fatalf("expected one error, got %v", env.Errors)
	}
	e := env.Errors[0]
	if e.ResourceType != "tweet" || e.Value != "123" || e.Type != typeResourceNotFound {
		t.Errorf("not-found error fields wrong: %+v", e)
	}
}
