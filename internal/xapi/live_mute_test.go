//go:build live

package xapi

import (
	"encoding/json"
	"io"
	"testing"

	http "github.com/bogdanfinn/fhttp"
)

// TestLiveMute verifies Mute/Unmute without disturbing the account's real state:
// it reads the target's initial mute state, mutes, then unmutes only when we were
// not already muting. Run with:
//
//	go test -tags live -run TestLiveMute -count=1 -v ./internal/xapi/
func TestLiveMute(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)
	const target = "jack"

	before, err := liveIsMuting(c, target)
	if err != nil {
		t.Fatalf("show(before): %v", err)
	}
	t.Logf("before: muting=%v", before)

	if err := c.Mute(target); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	t.Log("Mute: OK")

	if after, err := liveIsMuting(c, target); err == nil {
		t.Logf("after mute: muting=%v", after)
	}

	if before {
		t.Log("was already muting before the test; leaving the state unchanged")
		return
	}
	if err := c.Unmute(target); err != nil {
		t.Fatalf("Unmute (cleanup): %v", err)
	}
	t.Log("Unmute: OK (restored: not muting)")
}

// liveIsMuting reads relationship.source.muting from friendships/show.json.
func liveIsMuting(c *XClient, target string) (bool, error) {
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
	muting, _ := dig(m, "relationship", "source", "muting").(bool)
	return muting, nil
}
