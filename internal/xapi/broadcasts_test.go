package xapi

import (
	"encoding/json"
	"testing"
)

// TestParseBroadcast verifies parseBroadcast maps a BroadcastQuery node, taking
// the title from "status" and the broadcaster from the newer core schema.
func TestParseBroadcast(t *testing.T) {
	const raw = `{"data":{"broadcast":{"broadcast_id":"1BROADCAST001","status":"Example Broadcast","state":"RUNNING","image_url":"https://example.invalid/thumb.jpg","media_key":"28_4001","total_watched":1423,"start_time":1757000000000,"end_time":1757003600000,"available_for_replay":true,"edited_replay":{"start_time":90},"user_result":{"result":{"__typename":"User","rest_id":"222","core":{"screen_name":"alice","name":"Alice Example"}}}}}}`

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	b := parseBroadcast(asMap(dig(m, "data", "broadcast")))
	if b == nil {
		t.Fatal("parseBroadcast returned nil")
	}
	if b.ID != "1BROADCAST001" {
		t.Errorf("ID = %q, want 1BROADCAST001", b.ID)
	}
	if b.Title != "Example Broadcast" {
		t.Errorf("Title = %q, want Example Broadcast", b.Title)
	}
	if b.State != "RUNNING" {
		t.Errorf("State = %q, want RUNNING", b.State)
	}
	if b.MediaKey != "28_4001" {
		t.Errorf("MediaKey = %q, want 28_4001", b.MediaKey)
	}
	if b.TotalWatched != 1423 {
		t.Errorf("TotalWatched = %d, want 1423", b.TotalWatched)
	}
	if b.StartTime != 1757000000000 {
		t.Errorf("StartTime = %d, want 1757000000000", b.StartTime)
	}
	if b.EndTime != 1757003600000 {
		t.Errorf("EndTime = %d, want 1757003600000", b.EndTime)
	}
	if b.ReplayStart != 90 {
		t.Errorf("ReplayStart = %d, want 90", b.ReplayStart)
	}
	if !b.AvailableForReplay {
		t.Error("AvailableForReplay = false, want true")
	}
	if b.Broadcaster == nil {
		t.Fatal("Broadcaster is nil")
	}
	if b.Broadcaster.RestID != "222" {
		t.Errorf("Broadcaster.RestID = %q, want 222", b.Broadcaster.RestID)
	}
	if b.Broadcaster.ScreenName != "alice" {
		t.Errorf("Broadcaster.ScreenName = %q, want alice", b.Broadcaster.ScreenName)
	}
}

// TestParseBroadcastRejectsEmpty verifies parseBroadcast returns nil rather than a
// half-built record when the node is missing or carries no broadcast_id.
func TestParseBroadcastRejectsEmpty(t *testing.T) {
	if got := parseBroadcast(nil); got != nil {
		t.Errorf("parseBroadcast(nil) = %+v, want nil", got)
	}
	if got := parseBroadcast(map[string]any{"status": "no id"}); got != nil {
		t.Errorf("parseBroadcast without broadcast_id = %+v, want nil", got)
	}
}
