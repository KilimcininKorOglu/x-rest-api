//go:build live

package xapi

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// urlishKey matches the JSON keys this API uses for links and media.
var urlishKey = regexp.MustCompile(`(?i)(^|_)(url|urls|image|banner|thumbnail)(_|$)`)

// assertAbsolute walks a parsed model's JSON and fails on any url-ish string that
// is not an absolute http(s) URL, which is what a client needs to fetch it.
func assertAbsolute(t *testing.T, label string, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("%s: unmarshal: %v", label, err)
	}
	walkURLs(t, label, "", tree)
	t.Logf("%s: every url-ish field is absolute", label)
}

// walkURLs descends the decoded JSON, checking every leaf string.
func walkURLs(t *testing.T, label, path string, node any) {
	t.Helper()
	switch n := node.(type) {
	case map[string]any:
		for k, child := range n {
			walkURLs(t, label, path+"."+k, child)
		}
	case []any:
		for _, child := range n {
			walkURLs(t, label, path+"[]", child)
		}
	case string:
		checkURLString(t, label, path, n)
	}
}

// checkURLString fails when a url-ish field holds a non-absolute value.
func checkURLString(t *testing.T, label, path, s string) {
	t.Helper()
	key := path[strings.LastIndex(path, ".")+1:]
	if s == "" || !urlishKey.MatchString(key) {
		return
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		t.Errorf("%s%s is not absolute: %q", label, path, s)
	}
}

// assertVideos checks every video in tweets carries variants and a picked url,
// and returns how many videos it saw.
func assertVideos(t *testing.T, tweets []Tweet) int {
	t.Helper()
	seen := 0
	for _, tw := range tweets {
		if tw.Media == nil || len(tw.Media.Videos) == 0 {
			continue
		}
		seen++
		v := tw.Media.Videos[0]
		if v.URL == "" {
			t.Errorf("tweet %s has %d variants but no picked url", tw.RestID, len(v.Variants))
		}
		if len(v.Variants) == 0 {
			t.Errorf("tweet %s has a video with no variants", tw.RestID)
		}
	}
	return seen
}

// TestLiveAbsoluteURLs checks that every url the parsers emit is absolute, so a
// client can fetch it without knowing an upstream host.
//
//	go test -tags live -run TestLiveAbsoluteURLs -count=1 -v ./internal/xapi/
func TestLiveAbsoluteURLs(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	u, err := c.GetUser("jack")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	assertAbsolute(t, "user", u)

	media, _, err := c.UserMedia("elonmusk", 5, "")
	if err != nil {
		t.Fatalf("UserMedia: %v", err)
	}
	assertAbsolute(t, "media tweets", media)

	// Search explicitly for videos, because a media timeline can return photos
	// only and then the video url path is never exercised.
	vids, _, err := c.Search("filter:videos", "Latest", 10, "")
	if err != nil {
		t.Fatalf("Search(filter:videos): %v", err)
	}
	assertAbsolute(t, "video tweets", vids)

	withVideo := assertVideos(t, media) + assertVideos(t, vids)
	if withVideo == 0 {
		t.Error("no video tweet reached the assertions, so the video url path is unproven")
	}
	t.Logf("tweets with a video: %d", withVideo)
}
