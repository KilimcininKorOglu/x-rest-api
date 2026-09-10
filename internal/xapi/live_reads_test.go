//go:build live

package xapi

import (
	"encoding/json"
	"regexp"
	"testing"
)

// findFolderID pulls the first bookmark-folder id out of a raw folders response.
func findFolderID(raw map[string]any) string {
	b, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`"(?:rest_id|id)":"(\d{6,})"`).FindStringSubmatch(string(b))
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

// Known targets the read smokes run against.
const (
	liveHandle  = "jack"
	liveTweetID = "20"
	liveQuery   = "twitter"
)

// Ids that may rotate or 404; each smoke reports the first that resolves.
var (
	liveListIDs = []string{
		"1455045069516357634", "1494877848087187461",
		"1729635365319802902", "1141162794290520064",
	}
	liveCommunityIDs = []string{
		"1501272736215322629", "1489422448332197888", "1783990533192651232",
	}
	liveSpaceIDs = []string{"1mrxmayRyrQxy", "1vOxwjaWEbdJB"}
)

// liveSmoke drives one live read sweep, logging ok/fail per op. It never fails
// the test on an upstream error, so a run shows exactly which endpoints work.
type liveSmoke struct {
	t  *testing.T
	c  *XClient
	ok int
	no int
}

// newLiveSmoke builds a client from cookie.txt, skipping when it is absent.
func newLiveSmoke(t *testing.T) *liveSmoke {
	t.Helper()
	acct := loadLiveAccount(t)
	sess, err := NewSession("", "", "", "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return &liveSmoke{t: t, c: NewClientFor(sess, acct)}
}

func (s *liveSmoke) report(name string, n int, err error) {
	if err != nil {
		s.no++
		s.t.Logf("  FAIL %-24s %v", name, err)
		return
	}
	s.ok++
	s.t.Logf("  ok   %-24s n=%d", name, n)
}

// tweets and users adapt the two paginated return shapes onto report.
func (s *liveSmoke) tweets(name string, f func() ([]Tweet, string, error)) {
	got, _, err := f()
	s.report(name, len(got), err)
}

func (s *liveSmoke) users(name string, f func() ([]XUser, string, error)) {
	got, _, err := f()
	s.report(name, len(got), err)
}

// firstTweets reports the first id whose read succeeds, else the last error.
func (s *liveSmoke) firstTweets(name string, ids []string, f func(string) ([]Tweet, string, error)) {
	var lastErr error
	for _, id := range ids {
		got, _, err := f(id)
		if err == nil {
			s.report(name+"("+id+")", len(got), nil)
			return
		}
		lastErr = err
	}
	s.report(name, 0, lastErr)
}

// firstUsers is firstTweets for the user-returning reads.
func (s *liveSmoke) firstUsers(name string, ids []string, f func(string) ([]XUser, string, error)) {
	var lastErr error
	for _, id := range ids {
		got, _, err := f(id)
		if err == nil {
			s.report(name+"("+id+")", len(got), nil)
			return
		}
		lastErr = err
	}
	s.report(name, 0, lastErr)
}

// firstRaw is firstTweets for raw-passthrough reads keyed by one id.
func (s *liveSmoke) firstRaw(name string, ids []string, f func(string) (map[string]any, error)) {
	var lastErr error
	for _, id := range ids {
		got, err := f(id)
		if err == nil {
			s.report(name+"("+id+")", len(got), nil)
			return
		}
		lastErr = err
	}
	s.report(name, 0, lastErr)
}

// done logs the tally. Some fails are environment WAF blocks (by-rest-id) or
// expired ephemeral ids rather than parser bugs.
func (s *liveSmoke) done() {
	s.t.Logf("SUMMARY: %d ok, %d fail (of %d)", s.ok, s.no, s.ok+s.no)
}

// TestLiveReadsUsers smokes the profile and batch-lookup surfaces.
//
//	go test -tags live -run TestLiveReads -count=1 -v ./internal/xapi/
func TestLiveReadsUsers(t *testing.T) {
	s := newLiveSmoke(t)
	defer s.done()

	u, err := s.c.GetUser(liveHandle)
	s.report("GetUser", 1, err)
	if err == nil {
		t.Logf("       %s rest_id=%s", liveHandle, u.RestID)
	}
	_, err = s.c.GetUserByID("12")
	s.report("GetUserByID", 1, err)
	_, err = s.c.UserAbout(liveHandle)
	s.report("UserAbout", 1, err)
	us, err := s.c.UsersByIDs([]string{"12"})
	s.report("UsersByIDs", len(us), err)
	tws, err := s.c.TweetsByIDs([]string{liveTweetID})
	s.report("TweetsByIDs", len(tws), err)
}

// TestLiveReadsTimelines smokes the profile timelines and search.
func TestLiveReadsTimelines(t *testing.T) {
	s := newLiveSmoke(t)
	defer s.done()

	s.tweets("UserTweets", func() ([]Tweet, string, error) { return s.c.UserTweets(liveHandle, 3, "") })
	s.tweets("UserReplies", func() ([]Tweet, string, error) { return s.c.UserReplies(liveHandle, 3, "") })
	s.tweets("UserMedia", func() ([]Tweet, string, error) { return s.c.UserMedia(liveHandle, 3, "") })
	s.tweets("UserHighlights", func() ([]Tweet, string, error) { return s.c.UserHighlights(liveHandle, 3, "") })
	s.tweets("Likes", func() ([]Tweet, string, error) { return s.c.Likes(liveHandle, 3, "") })
	s.tweets("Search(Top)", func() ([]Tweet, string, error) { return s.c.Search(liveQuery, "Top", 3, "") })
	s.tweets("Search(Latest)", func() ([]Tweet, string, error) { return s.c.Search(liveQuery, "Latest", 3, "") })
	s.users("SearchUsers", func() ([]XUser, string, error) { return s.c.SearchUsers(liveQuery, 3, "") })
}

// TestLiveReadsTweetDetail smokes the single-tweet and conversation surfaces.
func TestLiveReadsTweetDetail(t *testing.T) {
	s := newLiveSmoke(t)
	defer s.done()

	th, err := s.c.GetTweet(liveTweetID)
	if err == nil {
		s.report("GetTweet", len(th.Replies), nil)
	} else {
		s.report("GetTweet", 0, err)
	}
	_, err = s.c.GetTweetResult(liveTweetID)
	s.report("GetTweetResult", 1, err)
	thread, err := s.c.TweetThread(liveTweetID, "relevance")
	s.report("TweetThread", len(thread), err)
	replies, err := s.c.TweetReplies(liveTweetID, "relevance")
	s.report("TweetReplies", len(replies), err)
}

// TestLiveReadsGraph smokes the follower/engagement graph reads.
func TestLiveReadsGraph(t *testing.T) {
	s := newLiveSmoke(t)
	defer s.done()

	s.users("Followers", func() ([]XUser, string, error) { return s.c.Followers(liveHandle, 3, "") })
	s.users("Following", func() ([]XUser, string, error) { return s.c.Following(liveHandle, 3, "") })
	s.users("VerifiedFollowers", func() ([]XUser, string, error) { return s.c.VerifiedFollowers(liveHandle, 3, "") })
	s.users("Subscriptions", func() ([]XUser, string, error) { return s.c.Subscriptions(liveHandle, 3, "") })
	s.users("Retweeters", func() ([]XUser, string, error) { return s.c.Retweeters(liveTweetID, 3, "") })
	s.users("Favoriters", func() ([]XUser, string, error) { return s.c.Favoriters(liveTweetID, 3, "") })
}

// TestLiveReadsAccountScoped smokes the reads that depend on who is logged in.
func TestLiveReadsAccountScoped(t *testing.T) {
	s := newLiveSmoke(t)
	defer s.done()

	s.tweets("Home", func() ([]Tweet, string, error) { return s.c.Home(3, "") })
	s.tweets("HomeLatest", func() ([]Tweet, string, error) { return s.c.HomeLatest(3, "") })
	s.tweets("Bookmarks", func() ([]Tweet, string, error) { return s.c.Bookmarks(3, "") })
	sched, err := s.c.ScheduledTweets()
	s.report("ScheduledTweets", len(sched), err)

	notif, err := s.c.CallRaw("NotificationsTimeline", map[string]any{}, "", 5)
	s.report("Notifications", len(notif), err)
	trends, err := s.c.CallRaw("GenericTimelineById",
		map[string]any{"timelineId": "VGltZWxpbmU6DAC2CwABAAAACHRyZW5kaW5nAAA"}, "", 5)
	s.report("Trends", len(trends), err)
	folders, err := s.c.CallRaw("BookmarkFoldersSlice", map[string]any{}, "", 0)
	s.report("BookmarkFolders", len(folders), err)
	s.bookmarkFolderTweets(folders, err)
}

// bookmarkFolderTweets reads one folder's tweets, taking the id from the
// account's own folder slice. It is skipped when the account has no folder.
func (s *liveSmoke) bookmarkFolderTweets(folders map[string]any, err error) {
	if err != nil {
		return
	}
	fid := findFolderID(folders)
	if fid == "" {
		s.t.Log("  skip BookmarkFolderTweets: account has no bookmark folder")
		return
	}
	got, _, e := s.c.BookmarkFolderTweets(fid, 3, "")
	s.report("BookmarkFolderTweets("+fid+")", len(got), e)
}

// TestLiveReadsIDScoped smokes the reads keyed by a list, community or Space id.
// Those ids rotate, so each read reports the first one that resolves.
func TestLiveReadsIDScoped(t *testing.T) {
	s := newLiveSmoke(t)
	defer s.done()

	s.firstTweets("ListTweets", liveListIDs,
		func(id string) ([]Tweet, string, error) { return s.c.ListTweets(id, 3, "") })
	s.firstUsers("ListMembers", liveListIDs,
		func(id string) ([]XUser, string, error) { return s.c.ListMembers(id, 3, "") })
	s.firstTweets("CommunityTweets", liveCommunityIDs,
		func(id string) ([]Tweet, string, error) { return s.c.CommunityTweets(id, 3, "") })
	s.firstUsers("CommunityMembers", liveCommunityIDs,
		func(id string) ([]XUser, string, error) { return s.c.CommunityMembers(id, 3, "") })
	s.firstUsers("CommunityModerators", liveCommunityIDs,
		func(id string) ([]XUser, string, error) { return s.c.CommunityModerators(id, 3, "") })
	s.firstRaw("CommunityInfo", liveCommunityIDs, func(id string) (map[string]any, error) {
		return s.c.CallRaw("CommunityQuery", map[string]any{"communityId": id}, "", 0)
	})
	// Space ids are ephemeral; the op path is exercised even when they are gone.
	s.firstRaw("SpaceInfo", liveSpaceIDs, func(id string) (map[string]any, error) {
		return s.c.CallRaw("AudioSpaceById", map[string]any{"id": id}, "", 0)
	})
}
