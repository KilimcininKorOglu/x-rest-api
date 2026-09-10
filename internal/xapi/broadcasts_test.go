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
	if b.Broadcaster == nil {
		t.Fatal("Broadcaster is nil")
	}
	checkFields(t, []fieldCheck{
		{"ID", b.ID, "1BROADCAST001"},
		{"Title", b.Title, "Example Broadcast"},
		{"State", b.State, "RUNNING"},
		{"MediaKey", b.MediaKey, "28_4001"},
		{"TotalWatched", b.TotalWatched, 1423},
		{"StartTime", b.StartTime, int64(1757000000000)},
		{"EndTime", b.EndTime, int64(1757003600000)},
		{"ReplayStart", b.ReplayStart, int64(90)},
		{"AvailableForReplay", b.AvailableForReplay, true},
		{"Broadcaster.RestID", b.Broadcaster.RestID, "222"},
		{"Broadcaster.ScreenName", b.Broadcaster.ScreenName, "alice"},
	})
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
