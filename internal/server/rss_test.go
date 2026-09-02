package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"x-rest-api/internal/xapi"
)

func TestRSSPubDate(t *testing.T) {
	got := rssPubDate("Wed Sep 02 05:25:38 +0000 2026")
	if got != "Wed, 02 Sep 2026 05:25:38 +0000" {
		t.Errorf("pubdate: %q", got)
	}
	if rssPubDate("not a date") != "not a date" {
		t.Error("unparseable date should pass through")
	}
	if rssPubDate("") != "" {
		t.Error("empty date should stay empty")
	}
}

func TestRSSItemTitleFollowsRetweet(t *testing.T) {
	rt := []xapi.Tweet{{
		UserScreenName: "reposter",
		Retweeted: &xapi.Tweet{
			RestID: "9", UserScreenName: "author", Text: "line one\nline two", URL: "https://x.com/author/status/9",
		},
	}}
	items := rssItems(rt)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].Title != "@author: line one line two" {
		t.Errorf("title should show the source author: %q", items[0].Title)
	}
	if items[0].GUID.Value != "9" || items[0].Link != "https://x.com/author/status/9" {
		t.Errorf("item should point at the source tweet: %+v", items[0])
	}
}

func TestWriteRSSFeed(t *testing.T) {
	data := []xapi.Tweet{{
		RestID: "1", UserScreenName: "naval", Text: "hello",
		CreatedAt: "Wed Sep 02 05:25:38 +0000 2026", URL: "https://x.com/naval/status/1",
		CommunityNote: "added context",
	}}
	r := httptest.NewRequest("GET", "/v1/users/naval/rss", nil)
	w := httptest.NewRecorder()
	writeResult(w, r, data, "")
	body := w.Body.String()
	if ct := w.Header().Get("content-type"); !strings.HasPrefix(ct, "application/rss+xml") {
		t.Errorf("content-type: %q", ct)
	}
	for _, want := range []string{
		`<rss version="2.0">`, "<item>", "<guid isPermaLink=\"false\">1</guid>",
		"Community note:", "https://x.com/naval/status/1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("feed missing %q\n%s", want, body)
		}
	}
}

func TestWriteRSSRejectsNonTweetList(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/users/naval/rss", nil)
	w := httptest.NewRecorder()
	writeResult(w, r, map[string]any{"data": 1}, "")
	if w.Code != 400 {
		t.Errorf("non-tweet payload on /rss should be 400, got %d", w.Code)
	}
}
