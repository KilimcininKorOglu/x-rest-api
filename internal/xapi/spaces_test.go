package xapi

import (
	"encoding/json"
	"testing"
)

// TestParseLiveSpaces verifies parseLiveSpaces maps a real fleetline thread (a
// captured live-content audiospace) into a LiveSpace record.
func TestParseLiveSpaces(t *testing.T) {
	const raw = `{"threads":[{"fully_read":false,"live_content":{"audiospace":{"broadcast_id":"1SPACE0000001","id":"1SPACE0000001","title":"Example Space","media_key":"28_3001","state":"RUNNING","content_type":"visual_audio","creator_user_id":"1CREATOR00001","creator_twitter_user_id":"111","primary_admin_user_id":"1CREATOR00001","admin_twitter_user_ids":["111"],"language":"en","is_locked":false,"start":"2026-09-02T21:13:51.632000000Z","total_live_listeners":21,"total_participating":7}},"user_id":111}]}`

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := parseLiveSpaces(m)
	if len(got) != 1 {
		t.Fatalf("parseLiveSpaces returned %d spaces, want 1", len(got))
	}
	sp := got[0]
	if sp.ID != "1SPACE0000001" {
		t.Errorf("ID = %q, want 1SPACE0000001", sp.ID)
	}
	if sp.Title != "Example Space" {
		t.Errorf("Title = %q", sp.Title)
	}
	if sp.State != "RUNNING" {
		t.Errorf("State = %q, want RUNNING", sp.State)
	}
	if sp.CreatorUserID != "111" {
		t.Errorf("CreatorUserID = %q, want 111", sp.CreatorUserID)
	}
	if sp.TotalLiveListeners != 21 {
		t.Errorf("TotalLiveListeners = %d, want 21", sp.TotalLiveListeners)
	}
	if sp.StartedAt != "2026-09-02T21:13:51.632000000Z" {
		t.Errorf("StartedAt = %q", sp.StartedAt)
	}
	if len(sp.AdminUserIDs) != 1 || sp.AdminUserIDs[0] != "111" {
		t.Errorf("AdminUserIDs = %v, want [111]", sp.AdminUserIDs)
	}
}
