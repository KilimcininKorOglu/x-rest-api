package xapi

import "time"

// AccountAnalytics returns the authenticated account's analytics overview for
// the [from, to) window (the /i/account_analytics daily overview). It is
// account-scoped, so it takes no target id. The previous window is the
// equal-length span ending at from, so the two windows compare like-for-like.
func (c *XClient) AccountAnalytics(from, to time.Time) (*AccountAnalytics, error) {
	payload, err := c.call("accountOverviewDailyQuery", AnalyticsVars(from, to))
	if err != nil {
		return nil, err
	}
	return parseAccountAnalytics(payload), nil
}

// AnalyticsVars builds the date-range variables the op expects: the current
// window, the equal-length previous window ending at from, and a backfill
// window covering the last two days of the current window, because x.com uses
// backfill to fill recent, still incomplete data.
func AnalyticsVars(from, to time.Time) map[string]any {
	from, to = from.UTC(), to.UTC()
	span := to.Sub(from)
	prevFrom := from.Add(-span)
	backfillFrom := to.Add(-48 * time.Hour)
	if backfillFrom.Before(from) {
		backfillFrom = from
	}
	iso := func(t time.Time) string { return t.Format("2006-01-02T15:04:05.000Z") }
	ms := func(t time.Time) int64 { return t.UnixMilli() }
	return map[string]any{
		"current_from":            ms(from),
		"current_from_iso":        iso(from),
		"current_to":              ms(to),
		"current_to_iso":          iso(to),
		"prev_from":               ms(prevFrom),
		"prev_from_iso":           iso(prevFrom),
		"prev_to":                 ms(from),
		"prev_to_iso":             iso(from),
		"backfill_from":           ms(backfillFrom),
		"backfill_to":             ms(to),
		"show_verified_followers": true,
	}
}

// parseAccountAnalytics maps the viewer_v2 analytics result into AccountAnalytics,
// summing the daily engagement and follow series into per-window totals.
func parseAccountAnalytics(payload map[string]any) *AccountAnalytics {
	res := asMap(dig(payload, "data", "viewer_v2", "user_results", "result"))
	if res == nil {
		return nil
	}
	a := &AccountAnalytics{
		Followers:               asInt(dig(res, "relationship_counts", "followers")),
		VerifiedFollowers:       asInt(res["verified_follower_count"]),
		ActiveFollowers:         asInt(dig(res, "author_follower_metrics", "active_followers")),
		ActiveVerifiedFollowers: asInt(dig(res, "author_follower_metrics", "active_verified_followers")),
	}
	a.Current.Engagements = sumEngagements(asSlice(res["current_time_series"]))
	a.Current.Follows, a.Current.Unfollows = sumFollowMetrics(asSlice(res["legacy_current_follow_metrics"]))
	a.Previous.Engagements = sumEngagements(asSlice(res["previous_totals"]))
	a.Previous.Follows, a.Previous.Unfollows = sumFollowMetrics(asSlice(res["legacy_previous_follow_metrics"]))
	return a
}

// sumEngagements totals a daily engagement series by engagement_type, folding
// the verified and non-verified rows for the same type together.
func sumEngagements(series []any) map[string]int64 {
	out := map[string]int64{}
	for _, e := range series {
		m := asMap(e)
		if m == nil {
			continue
		}
		out[asString(m["engagement_type"])] += asInt64(m["count"])
	}
	return out
}

// sumFollowMetrics totals the daily Follows and Unfollows metric series.
func sumFollowMetrics(series []any) (follows, unfollows int64) {
	for _, d := range series {
		for _, mv := range asSlice(dig(asMap(d), "metric_values")) {
			m := asMap(mv)
			switch asString(m["metric_type"]) {
			case "Follows":
				follows += asInt64(m["metric_value"])
			case "Unfollows":
				unfollows += asInt64(m["metric_value"])
			}
		}
	}
	return follows, unfollows
}
