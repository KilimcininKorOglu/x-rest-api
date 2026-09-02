//go:build live

package xapi

import (
	"os"
	"testing"
)

// TestLiveSpaceStream resolves a live Space's stream status. It needs an active
// Space id in X_LIVE_SPACE_ID (a Space ends quickly, so ids go stale). Run with:
//
//	X_LIVE_SPACE_ID=1XxygwlgovNGM go test -tags live -run TestLiveSpaceStream -count=1 -v ./internal/xapi/
func TestLiveSpaceStream(t *testing.T) {
	acct := loadLiveAccount(t)
	id := os.Getenv("X_LIVE_SPACE_ID")
	if id == "" {
		t.Skip("set X_LIVE_SPACE_ID to a currently-live Space id")
	}
	sess, err := NewSession("", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	st, err := c.SpaceStreamStatus(id)
	if err != nil {
		t.Fatalf("SpaceStreamStatus: %v", err)
	}
	t.Logf("media_key=%s share_url=%s chat_permission=%s", st.MediaKey, st.ShareURL, st.ChatPermissionType)
	if st.Source != nil {
		t.Logf("source: status=%s stream_type=%s location=%.60s", st.Source.Status, st.Source.StreamType, st.Source.Location)
	}
	if st.MediaKey == "" {
		t.Error("expected a media key")
	}
}
