//go:build live

package xapi

import (
	"fmt"
	"testing"
	"time"
)

// The write smokes exercise each write surface and immediately reverse it, so
// the account is left unchanged. Note tweets need X Premium and are expected to
// fail. Run them with:
//
//	go test -tags live -run TestLiveWrite -count=1 -v ./internal/xapi/

// liveWriteClient builds a client for the write smokes and a stamp that keeps
// each run's text unique.
func liveWriteClient(t *testing.T) (*XClient, int64) {
	t.Helper()
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return NewClientFor(sess, acct), time.Now().UnixNano()
}

// createAndDelete posts a tweet and deletes it again, so the account is left
// unchanged whatever the outcome.
func createAndDelete(t *testing.T, c *XClient, label, text, replyTo, quoteOf string) {
	t.Helper()
	tw, err := c.CreateTweet(text, replyTo, nil, quoteOf)
	if err != nil {
		t.Logf("  FAIL %-24s %v", label, err)
		return
	}
	t.Logf("  ok   %-24s rest_id=%s", label, tw.RestID)
	if err := c.DeleteTweet(tw.RestID); err != nil {
		t.Errorf("  cleanup DeleteTweet(%s): %v", tw.RestID, err)
	}
}

// TestLiveWriteTweets covers the plain, reply and quote forms of CreateTweet.
func TestLiveWriteTweets(t *testing.T) {
	c, stamp := liveWriteClient(t)
	createAndDelete(t, c, "CreateTweet", fmt.Sprintf("hello %d", stamp), "", "")
	createAndDelete(t, c, "CreateTweet(reply)", fmt.Sprintf("reply %d", stamp), "20", "")
	createAndDelete(t, c, "CreateTweet(quote)", fmt.Sprintf("quote %d", stamp), "", "20")
}

// TestLiveWriteFavorite likes a tweet and unlikes it again.
func TestLiveWriteFavorite(t *testing.T) {
	c, _ := liveWriteClient(t)
	if err := c.FavoriteTweet("20"); err != nil {
		t.Logf("  FAIL FavoriteTweet           %v", err)
		return
	}
	t.Log("  ok   FavoriteTweet")
	if err := c.UnfavoriteTweet("20"); err != nil {
		t.Errorf("  cleanup UnfavoriteTweet: %v", err)
	}
}

// TestLiveWriteScheduled schedules a tweet far enough ahead to cancel it again.
func TestLiveWriteScheduled(t *testing.T) {
	c, stamp := liveWriteClient(t)
	m, err := c.ScheduleTweet(fmt.Sprintf("later %d", stamp), time.Now().Add(48*time.Hour).Unix())
	if err != nil {
		t.Logf("  FAIL ScheduleTweet           %v", err)
		return
	}
	id := digScheduledID(m)
	t.Logf("  ok   ScheduleTweet           id=%s", id)
	if id == "" {
		t.Error("ScheduleTweet returned no id, so the scheduled tweet cannot be cancelled")
		return
	}
	if err := c.DeleteScheduledTweet(id); err != nil {
		t.Errorf("  cleanup DeleteScheduledTweet: %v", err)
	}
}

// TestLiveWriteNoteTweet is expected to fail without X Premium. It deletes the
// note when the account can post one, so the write is still reversed.
func TestLiveWriteNoteTweet(t *testing.T) {
	c, stamp := liveWriteClient(t)
	tw, err := c.CreateNoteTweet(fmt.Sprintf("note %d", stamp), "")
	if err != nil {
		t.Logf("  note CreateNoteTweet needs Premium: %v", err)
		return
	}
	t.Logf("  ok   CreateNoteTweet (account has Premium) rest_id=%s", tw.RestID)
	if err := c.DeleteTweet(tw.RestID); err != nil {
		t.Errorf("  cleanup DeleteTweet(%s): %v", tw.RestID, err)
	}
}

// digScheduledID pulls the scheduled-tweet rest_id out of the raw response.
func digScheduledID(m map[string]any) string {
	return asString(dig(m, "data", "tweet", "rest_id"))
}
