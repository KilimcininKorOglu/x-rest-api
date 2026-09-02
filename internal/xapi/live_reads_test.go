//go:build live

package xapi

import (
	"fmt"
	"testing"
)

// TestLiveReads smoke-tests every read surface against the live API with known
// targets (jack / tweet 20 / "twitter"). It never fails on an upstream error;
// it logs ok/err/count per op and a final tally, so one run shows exactly which
// endpoints work live. Run with:
//   go test -tags live -run TestLiveReads -count=1 -v ./internal/xapi/
func TestLiveReads(t *testing.T) {
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	c := NewClientFor(sess, acct)

	var ok, fail int
	report := func(name string, n int, err error) {
		if err != nil {
			fail++
			t.Logf("  FAIL %-24s %v", name, err)
			return
		}
		ok++
		t.Logf("  ok   %-24s n=%d", name, n)
	}
	// helpers to adapt return shapes
	tw := func(name string, f func() ([]Tweet, string, error)) {
		s, _, e := f()
		report(name, len(s), e)
	}
	us := func(name string, f func() ([]XUser, string, error)) {
		s, _, e := f()
		report(name, len(s), e)
	}

	const h, id, q = "jack", "20", "twitter"

	// user surfaces
	if u, e := c.GetUser(h); e == nil {
		report("GetUser", 1, nil)
		t.Logf("       jack rest_id=%s", u.RestID)
	} else {
		report("GetUser", 0, e)
	}
	if _, e := c.GetUserByID("12"); true {
		report("GetUserByID", 1, e)
	}
	if _, e := c.UserAbout(h); true {
		report("UserAbout", 1, e)
	}
	if s, e := c.UsersByIDs([]string{"12"}); true {
		report("UsersByIDs", len(s), e)
	}
	if s, e := c.TweetsByIDs([]string{id}); true {
		report("TweetsByIDs", len(s), e)
	}

	// timelines
	tw("UserTweets", func() ([]Tweet, string, error) { return c.UserTweets(h, 3, "") })
	tw("UserReplies", func() ([]Tweet, string, error) { return c.UserReplies(h, 3, "") })
	tw("UserMedia", func() ([]Tweet, string, error) { return c.UserMedia(h, 3, "") })
	tw("UserHighlights", func() ([]Tweet, string, error) { return c.UserHighlights(h, 3, "") })
	tw("Likes", func() ([]Tweet, string, error) { return c.Likes(h, 3, "") })
	tw("Search(Top)", func() ([]Tweet, string, error) { return c.Search(q, "Top", 3, "") })
	tw("Search(Latest)", func() ([]Tweet, string, error) { return c.Search(q, "Latest", 3, "") })
	us("SearchUsers", func() ([]XUser, string, error) { return c.SearchUsers(q, 3, "") })

	// tweet detail
	if th, e := c.GetTweet(id); e == nil {
		report("GetTweet", len(th.Replies), nil)
	} else {
		report("GetTweet", 0, e)
	}
	if _, e := c.GetTweetResult(id); true {
		report("GetTweetResult", 1, e)
	}
	if s, e := c.TweetThread(id, "relevance"); true {
		report("TweetThread", len(s), e)
	}
	if s, e := c.TweetReplies(id, "relevance"); true {
		report("TweetReplies", len(s), e)
	}

	// social graph
	us("Followers", func() ([]XUser, string, error) { return c.Followers(h, 3, "") })
	us("Following", func() ([]XUser, string, error) { return c.Following(h, 3, "") })
	us("VerifiedFollowers", func() ([]XUser, string, error) { return c.VerifiedFollowers(h, 3, "") })
	us("Subscriptions", func() ([]XUser, string, error) { return c.Subscriptions(h, 3, "") })
	us("Retweeters", func() ([]XUser, string, error) { return c.Retweeters(id, 3, "") })
	us("Favoriters", func() ([]XUser, string, error) { return c.Favoriters(id, 3, "") })

	// account-scoped
	tw("Home", func() ([]Tweet, string, error) { return c.Home(3, "") })
	tw("HomeLatest", func() ([]Tweet, string, error) { return c.HomeLatest(3, "") })
	tw("Bookmarks", func() ([]Tweet, string, error) { return c.Bookmarks(3, "") })
	if s, e := c.ScheduledTweets(); true {
		report("ScheduledTweets", len(s), e)
	}

	// raw-passthrough reads that need no id
	if m, e := c.CallRaw("NotificationsTimeline", map[string]any{}, "", 5); true {
		report("Notifications", len(m), e)
	}
	if m, e := c.CallRaw("GenericTimelineById", map[string]any{"timelineId": "VGltZWxpbmU6DAC2CwABAAAACHRyZW5kaW5nAAA"}, "", 5); true {
		report("Trends", len(m), e)
	}
	if m, e := c.CallRaw("BookmarkFoldersSlice", map[string]any{}, "", 0); true {
		report("BookmarkFolders", len(m), e)
	}

	// id-scoped reads: try several known ids, report the first that resolves.
	listIDs := []string{"1455045069516357634", "1494877848087187461", "1729635365319802902", "1141162794290520064"}
	commIDs := []string{"1501272736215322629", "1489422448332197888", "1783990533192651232"}
	firstTweets := func(name string, ids []string, f func(id string) ([]Tweet, string, error)) {
		var lastErr error
		for _, gid := range ids {
			s, _, e := f(gid)
			if e == nil {
				report(name+"("+gid+")", len(s), nil)
				return
			}
			lastErr = e
		}
		report(name, 0, lastErr)
	}
	firstUsers := func(name string, ids []string, f func(id string) ([]XUser, string, error)) {
		var lastErr error
		for _, gid := range ids {
			s, _, e := f(gid)
			if e == nil {
				report(name+"("+gid+")", len(s), nil)
				return
			}
			lastErr = e
		}
		report(name, 0, lastErr)
	}
	firstTweets("ListTweets", listIDs, func(id string) ([]Tweet, string, error) { return c.ListTweets(id, 3, "") })
	firstUsers("ListMembers", listIDs, func(id string) ([]XUser, string, error) { return c.ListMembers(id, 3, "") })
	firstTweets("CommunityTweets", commIDs, func(id string) ([]Tweet, string, error) { return c.CommunityTweets(id, 3, "") })
	firstUsers("CommunityMembers", commIDs, func(id string) ([]XUser, string, error) { return c.CommunityMembers(id, 3, "") })
	firstUsers("CommunityModerators", commIDs, func(id string) ([]XUser, string, error) { return c.CommunityModerators(id, 3, "") })
	{
		var lastErr error
		done := false
		for _, gid := range commIDs {
			m, e := c.CallRaw("CommunityQuery", map[string]any{"communityId": gid}, "", 0)
			if e == nil {
				report("CommunityInfo("+gid+")", len(m), nil)
				done = true
				break
			}
			lastErr = e
		}
		if !done {
			report("CommunityInfo", 0, lastErr)
		}
	}

	t.Logf("SUMMARY: %d ok, %d fail (of %d)", ok, fail, ok+fail)
	if fail > 0 {
		t.Logf("NOTE: id-scoped reads (List/Community/Space/BookmarkFolder) are not covered here; they need real ids.")
	}
	_ = fmt.Sprint
}
