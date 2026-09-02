//go:build live

package xapi

import (
	"strings"
	"testing"
	"time"
)

// TestLiveModerate verifies the hide-reply op is wired correctly against the real
// API using only the live account. x.com refuses to hide a self-thread reply (you
// may only hide other users' replies), so a single account cannot reach the happy
// path; instead this asserts HideReply surfaces that authorization error rather
// than swallowing the 200-with-errors payload. A well-formed authorization error
// (not a 404) proves the queryId, variables, and transaction-id are all accepted.
// The happy path itself is verified from a real browser capture (a conversation
// author hiding another user's reply returns tweet_moderate_put:"Done"). Run with:
//
//	go test -tags live -run TestLiveModerate -count=1 -v ./internal/xapi/
func TestLiveModerate(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	stamp := time.Now().Format("150405")
	root, err := c.CreateTweet("hide-reply smoke root "+stamp, "", nil, "")
	if err != nil {
		t.Fatalf("CreateTweet root: %v", err)
	}
	t.Logf("root tweet: %s", root.RestID)
	defer func() {
		if err := c.DeleteTweet(root.RestID); err != nil {
			t.Errorf("DeleteTweet root (cleanup): %v", err)
		}
	}()

	reply, err := c.CreateTweet("hide-reply smoke reply "+stamp, root.RestID, nil, "")
	if err != nil {
		t.Fatalf("CreateTweet reply: %v", err)
	}
	t.Logf("reply tweet: %s", reply.RestID)
	defer func() {
		if err := c.DeleteTweet(reply.RestID); err != nil {
			t.Errorf("DeleteTweet reply (cleanup): %v", err)
		}
	}()

	// The account authored the conversation but the reply is its own (self-thread),
	// which x.com refuses. HideReply must surface that as an error.
	err = c.HideReply(reply.RestID)
	if err == nil {
		// It unexpectedly succeeded; reverse it so the tweet is left visible.
		_ = c.UnhideReply(reply.RestID)
		t.Fatal("HideReply on a self-thread reply unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "Authorization") {
		t.Fatalf("HideReply error is not an authorization error: %v", err)
	}
	t.Logf("HideReply correctly surfaced the authorization error: %v", err)
}
