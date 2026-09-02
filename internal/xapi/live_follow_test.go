//go:build live

package xapi

import (
	"encoding/json"
	"io"
	"testing"

	http "github.com/bogdanfinn/fhttp"
)

// TestLiveFollow verifies Follow/Unfollow without disturbing the account's real
// graph: it reads the target's initial follow state, follows, then unfollows only
// when we were not already following. Run with:
//
//	go test -tags live -run TestLiveFollow -count=1 -v ./internal/xapi/
func TestLiveFollow(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)
	const target = "jack"

	before, err := liveIsFollowing(c, target)
	if err != nil {
		t.Fatalf("show(before): %v", err)
	}
	t.Logf("before: following=%v", before)

	if err := c.Follow(target); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	t.Log("Follow: OK")

	if after, err := liveIsFollowing(c, target); err == nil {
		t.Logf("after follow: following=%v", after)
	}

	if before {
		t.Log("was already following before the test; leaving the graph unchanged")
		return
	}
	if err := c.Unfollow(target); err != nil {
		t.Fatalf("Unfollow (cleanup): %v", err)
	}
	t.Log("Unfollow: OK (restored: not following)")
}

// liveIsFollowing reads relationship.source.following from friendships/show.json.
func liveIsFollowing(c *XClient, target string) (bool, error) {
	uid, err := c.resolveUID(target)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequest(http.MethodGet,
		"https://x.com/i/api/1.1/friendships/show.json?target_id="+uid, nil)
	if err != nil {
		return false, err
	}
	req.Header = c.sess.headers(c.acct, "en", "")
	resp, err := c.sess.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return false, err
	}
	following, _ := dig(m, "relationship", "source", "following").(bool)
	return following, nil
}
