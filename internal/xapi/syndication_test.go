package xapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSyndicationToken checks the shape the endpoint actually requires. Measured
// against the live endpoint: a non-empty token returns the tweet whatever its
// content is, while a missing or empty token returns an empty object. So the
// derivation must always yield a non-empty, zero-free, deterministic value; its
// exact digits are not contractual.
func TestSyndicationToken(t *testing.T) {
	const id = "20" // jack's first tweet: a public, stable id
	got := syndicationToken(id)
	if got == "" {
		t.Fatal("syndicationToken returned empty; the endpoint answers {} without a token")
	}
	if strings.ContainsAny(got, "0.") {
		t.Errorf("token %q still carries a zero or a radix point", got)
	}
	if again := syndicationToken(id); again != got {
		t.Errorf("syndicationToken is not deterministic: %q then %q", got, again)
	}
	if other := syndicationToken("1001"); other == got {
		t.Error("syndicationToken returned the same token for two different ids")
	}
}

// TestSyndicationTokenRejectsNonNumeric verifies a non-numeric id yields an empty
// token, so the caller refuses the request instead of sending a bad one.
func TestSyndicationTokenRejectsNonNumeric(t *testing.T) {
	for _, id := range []string{"", "alice", "12a34", "-5"} {
		if got := syndicationToken(id); got != "" {
			t.Errorf("syndicationToken(%q) = %q, want empty", id, got)
		}
	}
}

// TestSynTweetToModel verifies the syndication payload maps onto the shared Tweet
// model, including the fields whose names differ from the GraphQL ones.
func TestSynTweetToModel(t *testing.T) {
	const raw = `{"id_str":"1001","text":"hello from the embed surface","created_at":"2026-01-02T03:04:05.000Z","lang":"en","favorite_count":7,"conversation_count":3,"user":{"id_str":"2002","screen_name":"alice","name":"Alice Example"}}`

	var st synTweet
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tw := synTweetToModel(st)
	if tw.RestID != "1001" {
		t.Errorf("RestID = %q, want 1001", tw.RestID)
	}
	if tw.UserScreenName != "alice" {
		t.Errorf("UserScreenName = %q, want alice", tw.UserScreenName)
	}
	if tw.UserName != "Alice Example" {
		t.Errorf("UserName = %q, want Alice Example", tw.UserName)
	}
	if tw.Text != "hello from the embed surface" {
		t.Errorf("Text = %q", tw.Text)
	}
	if tw.Lang != "en" {
		t.Errorf("Lang = %q, want en", tw.Lang)
	}
	if tw.LikeCount != 7 {
		t.Errorf("LikeCount = %d, want 7 (favorite_count)", tw.LikeCount)
	}
	if tw.ReplyCount != 3 {
		t.Errorf("ReplyCount = %d, want 3 (conversation_count)", tw.ReplyCount)
	}
	if tw.URL != "https://x.com/alice/status/1001" {
		t.Errorf("URL = %q", tw.URL)
	}
}
