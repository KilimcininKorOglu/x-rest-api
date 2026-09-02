package server

import (
	"encoding/xml"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"x-rest-api/internal/xapi"
)

// RSS 2.0 feed generation for tweet-list endpoints. A /rss route reuses the same
// timeline fetch as its JSON sibling, then writeResult renders the tweets as RSS
// instead of JSON. Namespace prefixes (dc:, atom:) are omitted on purpose: Go's
// encoding/xml mangles prefixed element names, and a plain RSS 2.0 feed validates
// on its own.

// twitterTimeLayout is x.com's created_at format (e.g. "Wed Sep 02 05:25:38 +0000 2026").
const twitterTimeLayout = "Mon Jan 02 15:04:05 -0700 2006"

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language"`
	TTL         int       `xml:"ttl"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	Description rssCDATA `xml:"description"`
	PubDate     string   `xml:"pubDate"`
	GUID        rssGUID  `xml:"guid"`
}

type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

// rssCDATA wraps HTML description content in a CDATA section so tweet markup is not
// double-escaped.
type rssCDATA struct {
	Value string `xml:",cdata"`
}

// wantsRSS reports whether the request path selects an RSS feed.
func wantsRSS(r *http.Request) bool {
	return strings.HasSuffix(r.URL.Path, "/rss")
}

// writeRSS renders a tweet list as an RSS 2.0 feed. It returns false (writing
// nothing) when data is not a tweet slice, so the caller can report the error.
func writeRSS(w http.ResponseWriter, r *http.Request, data any) bool {
	tweets, ok := data.([]xapi.Tweet)
	if !ok {
		return false
	}
	feed := rssFeed{Version: "2.0", Channel: rssChannel{
		Title:       rssTitle(r),
		Link:        rssChannelLink(r),
		Description: rssTitle(r),
		Language:    "en-us",
		TTL:         40,
		Items:       rssItems(tweets),
	}}
	w.Header().Set("content-type", "application/rss+xml; charset=utf-8")
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		log.Printf("rss: write header: %v", err)
		return true
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		log.Printf("rss: encode: %v", err)
	}
	return true
}

// rssItems turns tweets into feed items, following the retweet to its source so the
// item shows the original author and text.
func rssItems(tweets []xapi.Tweet) []rssItem {
	items := make([]rssItem, 0, len(tweets))
	for i := range tweets {
		t := &tweets[i]
		if t.Retweeted != nil {
			t = t.Retweeted
		}
		items = append(items, rssItem{
			Title:       rssItemTitle(t),
			Link:        t.URL,
			Description: rssCDATA{Value: rssDescription(t)},
			PubDate:     rssPubDate(t.CreatedAt),
			GUID:        rssGUID{IsPermaLink: "false", Value: t.RestID},
		})
	}
	return items
}

// rssItemTitle is a one-line title: the author handle plus the collapsed tweet
// text, or a media placeholder when the tweet has no text.
func rssItemTitle(t *xapi.Tweet) string {
	text := strings.Join(strings.Fields(t.Text), " ")
	if text == "" && t.Media != nil {
		text = "Media"
	}
	if t.UserScreenName == "" {
		return text
	}
	return "@" + t.UserScreenName + ": " + text
}

// rssDescription builds the item body: the tweet text, an optional community note,
// and a link, all as light HTML.
func rssDescription(t *xapi.Tweet) string {
	var b strings.Builder
	if t.Text != "" {
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(t.Text, "\n", "<br>\n"))
		b.WriteString("</p>")
	}
	if t.CommunityNote != "" {
		b.WriteString("<p><b>Community note:</b> ")
		b.WriteString(t.CommunityNote)
		b.WriteString("</p>")
	}
	if t.URL != "" {
		b.WriteString(`<p><a href="`)
		b.WriteString(t.URL)
		b.WriteString(`">`)
		b.WriteString(t.URL)
		b.WriteString("</a></p>")
	}
	return b.String()
}

// rssPubDate converts x.com's created_at to RFC1123Z, or returns it unchanged when
// it does not parse.
func rssPubDate(createdAt string) string {
	if createdAt == "" {
		return ""
	}
	ts, err := time.Parse(twitterTimeLayout, createdAt)
	if err != nil {
		return createdAt
	}
	return ts.Format(time.RFC1123Z)
}

// rssTitle derives a channel title from the request path: a handle, a list id, or
// the search query.
func rssTitle(r *http.Request) string {
	if strings.Contains(r.URL.Path, "/search/") {
		if q := r.URL.Query().Get("q"); q != "" {
			return "Search: " + q
		}
		return "Search feed"
	}
	if h := chi.URLParam(r, "handle"); h != "" {
		return "@" + h
	}
	if id := chi.URLParam(r, "id"); id != "" {
		return "List " + id
	}
	return "x feed"
}

// rssChannelLink builds the human URL the feed points at.
func rssChannelLink(r *http.Request) string {
	if h := chi.URLParam(r, "handle"); h != "" {
		return "https://x.com/" + h
	}
	if id := chi.URLParam(r, "id"); id != "" {
		return "https://x.com/i/lists/" + id
	}
	return "https://x.com/search"
}
