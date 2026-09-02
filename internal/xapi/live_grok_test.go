//go:build live

package xapi

import "testing"

// TestLiveGrok opens a Grok conversation and sends one prompt. Grok may answer or
// return a quota block (Premium upsell); both are a pass. Run with:
//
//	go test -tags live -run TestLiveGrok -count=1 -v ./internal/xapi/
func TestLiveGrok(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	res, err := c.GrokChat([]GrokMessage{{Role: "user", Content: "Reply with the single word: pong"}}, "", false, false)
	if err != nil {
		t.Fatalf("GrokChat: %v", err)
	}
	t.Logf("conversation_id=%s", res.ConversationID)
	if res.RateLimit != nil && res.RateLimit.IsRateLimited {
		t.Logf("rate limited (Premium upsell): %q", res.RateLimit.Message)
		return
	}
	t.Logf("reply=%q webResults=%d", res.Message, len(res.WebResults))
	if res.ConversationID == "" {
		t.Error("expected a conversation id")
	}
}
