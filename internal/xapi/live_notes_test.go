//go:build live

package xapi

import "testing"

// TestLiveTweetNotes exercises the community-notes read and proves the tweet ops
// still work after includeHasBirdwatchNotes was added to their variables, because
// an unknown variable makes x.com answer 422.
//
//	go test -tags live -run TestLiveTweetNotes -count=1 -v ./internal/xapi/
func TestLiveTweetNotes(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	const id = "20" // jack's first tweet: public, and it carries proposed notes

	if _, err := c.GetTweet(id); err != nil {
		t.Errorf("GetTweet after the variables change: %v", err)
	}
	if _, err := c.GetTweetResult(id); err != nil {
		t.Errorf("GetTweetResult after the variables change: %v", err)
	}

	notes, err := c.TweetNotes(id)
	if err != nil {
		t.Fatalf("TweetNotes: %v", err)
	}
	t.Logf("misleading=%d not_misleading=%d can_write=%t",
		len(notes.Misleading), len(notes.NotMisleading), notes.CanWriteNote)
	if len(notes.Misleading)+len(notes.NotMisleading) == 0 {
		t.Skip("tweet returned no notes; nothing to assert on the mapping")
	}
	n := append(append([]CommunityNoteEntry{}, notes.Misleading...), notes.NotMisleading...)[0]
	t.Logf("note %s status=%q class=%q tags=%v alias=%q sources=%d text=%.60q",
		n.RestID, n.RatingStatus, n.Classification, n.Tags, n.AuthorAlias, len(n.Sources), n.Text)
	if n.RestID == "" || n.Text == "" {
		t.Errorf("note mapped without an id or text: %+v", n)
	}
}
