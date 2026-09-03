package xapi

import (
	"net/url"
	"strings"
)

// SidebarTrends returns the personalized "What's happening" trends
// (ExploreSidebar). The op takes no variables; x.com personalizes by the
// account whose cookies make the call.
func (c *XClient) SidebarTrends() ([]Trend, error) {
	payload, err := c.call("ExploreSidebar", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseTrends(payload), nil
}

// ExploreTrends returns the trends on the personalized Explore "For You" page
// (ExplorePage), including AI-summarized "Today's News" story trends. The page
// also carries tweets and users; use the raw endpoint to read those.
func (c *XClient) ExploreTrends() ([]Trend, error) {
	payload, err := c.call("ExplorePage", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseTrends(payload), nil
}

// parseTrends maps every TimelineTrend itemContent in the timeline to a Trend.
func parseTrends(payload map[string]any) []Trend {
	var out []Trend
	for _, it := range itemContents(entries(payload)) {
		if asString(it["__typename"]) != "TimelineTrend" {
			continue
		}
		out = append(out, trendFromItem(it))
	}
	return out
}

// trendFromItem maps one TimelineTrend itemContent to a Trend.
func trendFromItem(it map[string]any) Trend {
	meta := asMap(it["trend_metadata"])
	t := Trend{
		Name:            asString(it["name"]),
		Description:     asString(it["description"]),
		DomainContext:   asString(meta["domain_context"]),
		MetaDescription: asString(meta["meta_description"]),
		Query:           trendQuery(it),
		SocialContext:   asString(dig(it, "social_context", "text")),
		IsAITrend:       asBool(it["is_ai_trend"]),
	}
	if pm := asMap(it["promoted_metadata"]); pm != nil {
		t.Promoted = true
		t.AdvertiserScreenName = asString(dig(pm, "advertiser_results", "result", "core", "screen_name"))
	}
	return t
}

// trendQuery returns the exact search term x.com issues for a trend: the promoted
// query term when present, else the query parameter of the trend's deeplink, else
// the trend name.
func trendQuery(it map[string]any) string {
	if q := asString(dig(asMap(it["promoted_metadata"]), "promotedTrendQueryTerm")); q != "" {
		return q
	}
	raw := asString(dig(it, "trend_url", "url"))
	if raw == "" {
		raw = asString(dig(it, "trend_metadata", "url", "url"))
	}
	if q := queryParam(raw); q != "" {
		return q
	}
	return asString(it["name"])
}

// queryParam extracts and unescapes the query= parameter from a trend deeplink.
func queryParam(raw string) string {
	_, after, found := strings.Cut(raw, "query=")
	if !found {
		return ""
	}
	frag, _, _ := strings.Cut(after, "&")
	if dec, err := url.QueryUnescape(frag); err == nil {
		return dec
	}
	return ""
}
