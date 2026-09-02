//go:build live

package xapi

import "testing"

// TestLiveDM smoke-tests the DM reads. It never calls DeleteConversation, because
// that destroys real data. Run with:
//
//	go test -tags live -run TestLiveDM -count=1 -v ./internal/xapi/
func TestLiveDM(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	inbox, err := c.Inbox("")
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	t.Logf("Inbox: %d conversations, cursor=%v", len(inbox.Conversations), inbox.Cursor != "")
	if len(inbox.Conversations) == 0 {
		t.Log("no conversations to load; Conversation not exercised")
		return
	}
	first := inbox.Conversations[0]
	t.Logf("  first conv: id=%s type=%s participants=%d messages=%d", first.ID, first.Type, len(first.Participants), len(first.Messages))

	conv, err := c.Conversation(first.ID, "")
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if conv == nil {
		t.Fatal("Conversation: nil")
	}
	t.Logf("Conversation: id=%s messages=%d hasMore=%v", conv.ID, len(conv.Messages), conv.HasMore)
	if len(conv.Messages) > 0 {
		m := conv.Messages[0]
		t.Logf("  msg: id=%s sender=%s textLen=%d", m.ID, m.SenderID, len(m.Text))
	}
}
