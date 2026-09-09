//go:build live

package xapi

import "testing"

// TestLiveCredentialFreeTiers exercises both credential-free read tiers against
// the live surface. It needs no cookie: Tier 0 is the public syndication
// endpoint and Tier 1 mints its own guest token. Read-only, so it reverses
// nothing.
//
//	go test -tags live -run TestLiveCredentialFreeTiers -count=1 -v ./internal/xapi/
func TestLiveCredentialFreeTiers(t *testing.T) {
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	const tweetID = "20" // jack's first tweet: public and stable
	const handle = "jack"

	if tw, err := sess.FetchTweetSyndication(tweetID); err != nil {
		t.Errorf("Tier 0 syndication tweet: %v", err)
	} else {
		t.Logf("Tier 0 syndication tweet   OK  id=%s author=@%s text=%q",
			tw.RestID, tw.UserScreenName, tw.Text)
		if tw.RestID != tweetID {
			t.Errorf("Tier 0 returned id %q, want %q", tw.RestID, tweetID)
		}
	}

	if tw, err := sess.FetchTweetGuest(tweetID); err != nil {
		t.Errorf("Tier 1 guest tweet: %v", err)
	} else {
		t.Logf("Tier 1 guest tweet         OK  id=%s author=@%s", tw.RestID, tw.UserScreenName)
		if tw.RestID != tweetID {
			t.Errorf("Tier 1 returned id %q, want %q", tw.RestID, tweetID)
		}
	}

	if u, err := sess.FetchUserGuest(handle); err != nil {
		t.Errorf("Tier 1 guest user: %v", err)
	} else {
		t.Logf("Tier 1 guest user          OK  id=%s @%s followers=%d",
			u.RestID, u.ScreenName, u.FollowersCount)
	}

	// The second guest read must reuse the cached token rather than re-mint it.
	first, err := sess.guestToken()
	if err != nil {
		t.Fatalf("guestToken: %v", err)
	}
	second, err := sess.guestToken()
	if err != nil {
		t.Fatalf("guestToken (cached): %v", err)
	}
	if first != second {
		t.Errorf("guest token was re-minted instead of cached")
	}
}
