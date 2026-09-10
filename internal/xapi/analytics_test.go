package xapi

import (
	"encoding/json"
	"testing"
	"time"
)

// TestParseAccountAnalytics verifies parseAccountAnalytics reads the headline
// follower metrics and sums the daily engagement and follow series into
// per-window totals, folding verified and non-verified rows together.
func TestParseAccountAnalytics(t *testing.T) {
	const raw = `{"data":{"viewer_v2":{"user_results":{"result":{"__typename":"User","author_follower_metrics":{"active_followers":"100","active_verified_followers":"20"},"current_time_series":[{"count":5,"engagement_type":"Fav","is_engaging_user_verified":"true","timestamp":1780704000000},{"count":8,"engagement_type":"Fav","is_engaging_user_verified":"false","timestamp":1780704000000},{"count":3,"engagement_type":"Reply","is_engaging_user_verified":"false","timestamp":1780704000000},{"count":10,"engagement_type":"Fav","is_engaging_user_verified":"false","timestamp":1780790400000}],"legacy_current_follow_metrics":[{"metric_values":[{"metric_type":"Follows","metric_value":2},{"metric_type":"Unfollows","metric_value":1}],"timestamp":{"iso8601_time":"2026-06-06T00:00:00Z"}},{"metric_values":[{"metric_type":"Follows","metric_value":3},{"metric_type":"Unfollows"}],"timestamp":{"iso8601_time":"2026-06-07T00:00:00Z"}}],"previous_totals":[{"count":100,"engagement_type":"Fav","is_engaging_user_verified":"false","timestamp":1772928000000},{"count":20,"engagement_type":"Fav","is_engaging_user_verified":"true","timestamp":1772928000000},{"count":7,"engagement_type":"Reply","is_engaging_user_verified":"false","timestamp":1772928000000}],"legacy_previous_follow_metrics":[{"metric_values":[{"metric_type":"Follows","metric_value":9},{"metric_type":"Unfollows","metric_value":3}],"timestamp":{"iso8601_time":"2026-03-08T00:00:00Z"}}],"relationship_counts":{"followers":1000},"verified_follower_count":"42"}}}}}`

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	a := parseAccountAnalytics(m)
	if a == nil {
		t.Fatal("parseAccountAnalytics returned nil")
	}
	checkFields(t, []fieldCheck{
		{"Followers", a.Followers, 1000},
		{"VerifiedFollowers", a.VerifiedFollowers, 42},
		{"ActiveFollowers", a.ActiveFollowers, 100},
		{"ActiveVerifiedFollowers", a.ActiveVerifiedFollowers, 20},
		{"Current.Engagements[Fav]", a.Current.Engagements["Fav"], int64(23)},
		{"Current.Engagements[Reply]", a.Current.Engagements["Reply"], int64(3)},
		{"Current.Follows", a.Current.Follows, int64(5)},
		{"Current.Unfollows", a.Current.Unfollows, int64(1)},
		{"Previous.Engagements[Fav]", a.Previous.Engagements["Fav"], int64(120)},
		{"Previous.Engagements[Reply]", a.Previous.Engagements["Reply"], int64(7)},
		{"Previous.Follows", a.Previous.Follows, int64(9)},
		{"Previous.Unfollows", a.Previous.Unfollows, int64(3)},
	})
}

// TestAnalyticsVars verifies the previous window mirrors the current window's
// length and the backfill window covers the last two days of the current window.
func TestAnalyticsVars(t *testing.T) {
	from := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	v := AnalyticsVars(from, to)

	if v["current_from_iso"] != "2026-06-06T00:00:00.000Z" {
		t.Errorf("current_from_iso = %v", v["current_from_iso"])
	}
	if v["current_to"] != to.UnixMilli() {
		t.Errorf("current_to = %v, want %d", v["current_to"], to.UnixMilli())
	}
	// Previous window ends at from and has the current window's length.
	span := to.Sub(from)
	if v["prev_from"] != from.Add(-span).UnixMilli() {
		t.Errorf("prev_from = %v", v["prev_from"])
	}
	if v["prev_to"] != from.UnixMilli() {
		t.Errorf("prev_to = %v, want %d", v["prev_to"], from.UnixMilli())
	}
	if v["backfill_from"] != to.Add(-48*time.Hour).UnixMilli() {
		t.Errorf("backfill_from = %v", v["backfill_from"])
	}
	if v["show_verified_followers"] != true {
		t.Errorf("show_verified_followers = %v", v["show_verified_followers"])
	}
}
