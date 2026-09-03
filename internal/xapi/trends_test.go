package xapi

import (
	"encoding/json"
	"testing"
)

// TestParseTrends verifies parseTrends maps ExploreSidebar TimelineTrend items,
// including a promoted trend, and extracts the search query from the deeplink.
func TestParseTrends(t *testing.T) {
	const raw = `{"data":{"explore_sidebar":{"timeline":{"instructions":[{"type":"TimelineClearCache"},{"entries":[{"content":{"__typename":"TimelineTimelineModule","entryType":"TimelineTimelineModule","items":[{"entryId":"trend-x-trend-1","item":{"itemContent":{"__typename":"TimelineTrend","description":"Big Sale","itemType":"TimelineTrend","name":"Acme Widget","promoted_metadata":{"advertiser_results":{"result":{"__typename":"User","core":{"screen_name":"acmebrand"},"rest_id":"111"}},"promotedTrendQueryTerm":"Acme Widget"},"trend_metadata":{"meta_description":"Promoted by acmebrand"},"trend_url":{"url":"twitter://search/?query=HUAWEI+WATCH+GT+7+Pro&src=promoted_trend_click"}}}},{"entryId":"trend-x-trend-Local Elections","item":{"itemContent":{"__typename":"TimelineTrend","itemType":"TimelineTrend","name":"Local Elections","trend_metadata":{"domain_context":"Trending in Wonderland"},"trend_url":{"url":"twitter://search/?query=%22Yallah+Arabistana%22&src=trend_click"}}}},{"entryId":"trend-x-trend-#günaydın","item":{"itemContent":{"__typename":"TimelineTrend","itemType":"TimelineTrend","name":"#günaydın","trend_metadata":{"domain_context":"Trending in Wonderland"},"trend_url":{"url":"twitter://search/?query=%23g%C3%BCnayd%C4%B1n&src=trend_click"}}}}]},"entryId":"trend-x","sortIndex":"1"},{"content":{"__typename":"TimelineTimelineCursor","cursorType":"Top","value":"DAAJAAA"},"entryId":"cursor-top-1","sortIndex":"2"}],"type":"TimelineAddEntries"}]}}}}`

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tr := parseTrends(m)
	if len(tr) != 3 {
		t.Fatalf("parseTrends returned %d trends, want 3", len(tr))
	}

	p := tr[0]
	if p.Name != "Acme Widget" {
		t.Errorf("promoted Name = %q", p.Name)
	}
	if !p.Promoted {
		t.Error("promoted trend not flagged Promoted")
	}
	if p.AdvertiserScreenName != "acmebrand" {
		t.Errorf("AdvertiserScreenName = %q", p.AdvertiserScreenName)
	}
	if p.MetaDescription != "Promoted by acmebrand" {
		t.Errorf("MetaDescription = %q", p.MetaDescription)
	}
	if p.Query != "Acme Widget" {
		t.Errorf("promoted Query = %q", p.Query)
	}

	o := tr[1]
	if o.Name != "Local Elections" || o.Promoted {
		t.Errorf("organic trend = %+v", o)
	}
	if o.DomainContext != "Trending in Wonderland" {
		t.Errorf("DomainContext = %q", o.DomainContext)
	}
	if o.Query != `"Local Elections"` {
		t.Errorf("organic Query = %q, want quoted phrase", o.Query)
	}

	h := tr[2]
	if h.Name != "#günaydın" {
		t.Errorf("hashtag Name = %q", h.Name)
	}
	if h.Query != "#günaydın" {
		t.Errorf("hashtag Query = %q", h.Query)
	}
}

// TestParseExploreTrends verifies parseTrends handles the deeper ExplorePage
// nesting and extracts AI "Today's News" story trends with their social_context.
func TestParseExploreTrends(t *testing.T) {
	const raw = `{"data":{"explore_page":{"body":{"initialTimeline":{"timeline":{"timeline":{"instructions":[{"type":"TimelineClearCache"},{"entries":[{"content":{"__typename":"TimelineTimelineItem","itemContent":{"__typename":"TimelineEventSummary","event":{"rest_id":"1"},"title":"Acme Widget"}},"entryId":"eventsummary-1"},{"content":{"__typename":"TimelineTimelineModule","header":{"text":"Today's News"},"items":[{"entryId":"stories-x-trend-1","item":{"itemContent":{"__typename":"TimelineTrend","is_ai_trend":true,"name":"New Feature Ships in Popular App","social_context":{"contextType":"Facepile","text":"11 hours ago · News · 2.1K posts","type":"TimelineGeneralContext"},"trend_url":{"url":"twitter://trending/1"}}}}]},"entryId":"stories-x"},{"content":{"__typename":"TimelineTimelineItem","itemContent":{"__typename":"TimelineTrend","name":"Weather","trend_metadata":{"domain_context":"Trending in Wonderland"},"trend_url":{"url":"twitter://search/?query=Weather&src=trend_click"}}},"entryId":"trend-Weather"},{"content":{"__typename":"TimelineTimelineCursor","cursorType":"Bottom","value":"DAAB"},"entryId":"cursor-bottom-x"}],"type":"TimelineAddEntries"}]}}}}}}}`

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tr := parseTrends(m)
	if len(tr) != 2 {
		t.Fatalf("parseTrends returned %d trends, want 2 (event summary excluded)", len(tr))
	}

	ai := tr[0]
	if !ai.IsAITrend {
		t.Error("AI story trend not flagged IsAITrend")
	}
	if ai.Name != "New Feature Ships in Popular App" {
		t.Errorf("AI Name = %q", ai.Name)
	}
	if ai.SocialContext != "11 hours ago · News · 2.1K posts" {
		t.Errorf("SocialContext = %q", ai.SocialContext)
	}
	if ai.Query != ai.Name {
		t.Errorf("AI Query = %q, want the name (deeplink has no query=)", ai.Query)
	}

	o := tr[1]
	if o.Name != "Weather" || o.IsAITrend {
		t.Errorf("organic trend = %+v", o)
	}
	if o.Query != "Weather" {
		t.Errorf("organic Query = %q", o.Query)
	}
}
