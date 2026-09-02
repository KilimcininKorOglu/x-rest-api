//go:build live

package xapi

import (
	"fmt"
	"testing"
	"time"
)

// TestLiveWrites smoke-tests each write surface and immediately reverses it, so
// the account is left unchanged. Note tweets need X Premium and are expected to
// fail. Run with:
//
//	go test -tags live -run TestLiveWrites -count=1 -v ./internal/xapi/
func TestLiveWrites(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)
	stamp := time.Now().UnixNano()

	// plain tweet -> delete
	if tw, e := c.CreateTweet(fmt.Sprintf("hello %d", stamp), "", nil, ""); e == nil {
		t.Logf("  ok   CreateTweet             rest_id=%s", tw.RestID)
		if e := c.DeleteTweet(tw.RestID); e != nil {
			t.Errorf("  cleanup DeleteTweet: %v", e)
		}
	} else {
		t.Logf("  FAIL CreateTweet             %v", e)
	}

	// reply -> delete
	if tw, e := c.CreateTweet(fmt.Sprintf("reply %d", stamp), "20", nil, ""); e == nil {
		t.Logf("  ok   CreateTweet(reply)      rest_id=%s", tw.RestID)
		_ = c.DeleteTweet(tw.RestID)
	} else {
		t.Logf("  FAIL CreateTweet(reply)      %v", e)
	}

	// quote -> delete
	if tw, e := c.CreateTweet(fmt.Sprintf("quote %d", stamp), "", nil, "20"); e == nil {
		t.Logf("  ok   CreateTweet(quote)      rest_id=%s", tw.RestID)
		_ = c.DeleteTweet(tw.RestID)
	} else {
		t.Logf("  FAIL CreateTweet(quote)      %v", e)
	}

	// like -> unlike
	if e := c.FavoriteTweet("20"); e == nil {
		t.Log("  ok   FavoriteTweet")
		if e := c.UnfavoriteTweet("20"); e != nil {
			t.Errorf("  cleanup UnfavoriteTweet: %v", e)
		}
	} else {
		t.Logf("  FAIL FavoriteTweet           %v", e)
	}

	// schedule -> delete
	if m, e := c.ScheduleTweet(fmt.Sprintf("later %d", stamp), time.Now().Add(48*time.Hour).Unix()); e == nil {
		id := digScheduledID(m)
		t.Logf("  ok   ScheduleTweet           id=%s", id)
		if id != "" {
			if e := c.DeleteScheduledTweet(id); e != nil {
				t.Errorf("  cleanup DeleteScheduledTweet: %v", e)
			}
		}
	} else {
		t.Logf("  FAIL ScheduleTweet           %v", e)
	}

	// note tweet: expected to fail without X Premium
	if _, e := c.CreateNoteTweet(fmt.Sprintf("note %d", stamp), ""); e == nil {
		t.Log("  ok   CreateNoteTweet (account has Premium)")
	} else {
		t.Logf("  note CreateNoteTweet needs Premium: %v", e)
	}
}

// digScheduledID pulls the scheduled-tweet rest_id out of the raw response.
func digScheduledID(m map[string]any) string {
	return asString(dig(m, "data", "tweet", "rest_id"))
}
