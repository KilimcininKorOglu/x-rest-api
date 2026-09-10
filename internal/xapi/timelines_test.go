package xapi

import (
	"encoding/json"
	"testing"
)

// decode is a test helper that unmarshals a JSON literal into a payload map.
func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return m
}

// tweetTimelineFixture is a minimal but real-shaped tweet timeline: the envelope
// hangs under data.user.result.timeline_v2, carries one tweet and a bottom cursor.
const tweetTimelineFixture = `{
  "data": {"user": {"result": {"timeline_v2": {"timeline": {"instructions": [
    {"type": "TimelineAddEntries", "entries": [
      {"entryId": "tweet-1", "content": {"entryType": "TimelineTimelineItem",
        "itemContent": {"tweet_results": {"result": {
          "__typename": "Tweet", "rest_id": "1",
          "core": {"user_results": {"result": {"legacy": {"screen_name": "alice", "name": "Alice"}}}},
          "legacy": {"full_text": "hello world", "created_at": "Mon Jan 01 00:00:00 +0000 2026",
            "favorite_count": 5, "reply_count": 1, "retweet_count": 2, "quote_count": 0,
            "lang": "en", "is_quote_status": false},
          "views": {"count": "123"}
        }}}}},
      {"entryId": "cursor-bottom-1", "content": {"entryType": "TimelineTimelineCursor",
        "cursorType": "Bottom", "value": "CURSOR123"}}
    ]}
  ]}}}}}
}`

func TestParseTimelineTweets(t *testing.T) {
	tweets, cur := parseTimelineTweets(decode(t, tweetTimelineFixture))
	if len(tweets) != 1 {
		t.Fatalf("want 1 tweet, got %d", len(tweets))
	}
	got := tweets[0]
	if got.RestID != "1" || got.UserScreenName != "alice" || got.UserName != "Alice" {
		t.Errorf("identity mismatch: %+v", got)
	}
	if got.Text != "hello world" || got.LikeCount != 5 || got.ViewCount != 123 {
		t.Errorf("field mismatch: %+v", got)
	}
	if got.URL != "https://x.com/alice/status/1" {
		t.Errorf("url mismatch: %s", got.URL)
	}
	if cur != "CURSOR123" {
		t.Errorf("cursor mismatch: %q", cur)
	}
}

// noteTweetFixture proves long-form text is read from note_tweet, not full_text.
const noteTweetFixture = `{
  "data": {"x": {"instructions": [{"type": "TimelineAddEntries", "entries": [
    {"entryId": "tweet-2", "content": {"itemContent": {"tweet_results": {"result": {
      "__typename": "TweetWithVisibilityResults",
      "tweet": {"__typename": "Tweet", "rest_id": "2",
        "legacy": {"full_text": "truncated..."},
        "note_tweet": {"note_tweet_results": {"result": {"text": "the full long-form body"}}}}
    }}}}}
  ]}]}}
}`

func TestParseNoteTweetAndVisibilityUnwrap(t *testing.T) {
	tweets, _ := parseTimelineTweets(decode(t, noteTweetFixture))
	if len(tweets) != 1 {
		t.Fatalf("want 1 tweet, got %d", len(tweets))
	}
	if tweets[0].RestID != "2" || tweets[0].Text != "the full long-form body" {
		t.Errorf("note/visibility unwrap failed: %+v", tweets[0])
	}
}

// tombstoneFixture proves TweetTombstone entries are skipped.
const tombstoneFixture = `{
  "data": {"instructions": [{"type": "TimelineAddEntries", "entries": [
    {"entryId": "tweet-3", "content": {"itemContent": {"tweet_results": {"result": {
      "__typename": "TweetTombstone"}}}}}
  ]}]}
}`

func TestParseTweetSkipsTombstone(t *testing.T) {
	tweets, _ := parseTimelineTweets(decode(t, tombstoneFixture))
	if len(tweets) != 0 {
		t.Errorf("want 0 tweets, got %d", len(tweets))
	}
}

// userTimelineFixture is a followers-style timeline with one user_results entry.
const userTimelineFixture = `{
  "data": {"user": {"result": {"timeline": {"timeline": {"instructions": [
    {"type": "TimelineAddEntries", "entries": [
      {"entryId": "user-9", "content": {"itemContent": {"user_results": {"result": {
        "rest_id": "9",
        "legacy": {"screen_name": "bob", "name": "Bob", "followers_count": 42, "verified": false},
        "is_blue_verified": true
      }}}}},
      {"entryId": "cursor-bottom-9", "content": {"entryType": "TimelineTimelineCursor",
        "cursorType": "Bottom", "value": "UCURSOR"}}
    ]}
  ]}}}}}
}`

func TestParseTimelineUsers(t *testing.T) {
	users, cur := parseTimelineUsers(decode(t, userTimelineFixture))
	if len(users) != 1 {
		t.Fatalf("want 1 user, got %d", len(users))
	}
	u := users[0]
	if u.RestID != "9" || u.ScreenName != "bob" || u.FollowersCount != 42 {
		t.Errorf("field mismatch: %+v", u)
	}
	if !u.Verified {
		t.Errorf("is_blue_verified should set Verified")
	}
	if cur != "UCURSOR" {
		t.Errorf("cursor mismatch: %q", cur)
	}
}

// userByScreenNameFixture is a UserByScreenName response.
const userByScreenNameFixture = `{
  "data": {"user": {"result": {
    "rest_id": "100",
    "legacy": {"screen_name": "carol", "name": "Carol", "followers_count": 7}
  }}}
}`

func TestParseUserByScreenName(t *testing.T) {
	u := parseUserByScreenName(decode(t, userByScreenNameFixture))
	if u == nil || u.RestID != "100" || u.ScreenName != "carol" || u.FollowersCount != 7 {
		t.Fatalf("profile mismatch: %+v", u)
	}
	if u.URL != "https://x.com/carol" {
		t.Errorf("url mismatch: %s", u.URL)
	}
}

// newSchemaUserFixture has an empty legacy block; counts live in
// relationship_counts/tweet_counts and the bio in profile_bio, as x.com serves
// on the newer schema.
const newSchemaUserFixture = `{
  "data": {"user": {"result": {
    "__typename": "User", "rest_id": "12", "is_blue_verified": true,
    "legacy": {},
    "core": {"screen_name": "jack", "name": "jack", "created_at": "Tue Mar 21 20:50:14 +0000 2006"},
    "relationship_counts": {"followers": 11936016, "following": 3},
    "tweet_counts": {"tweets": 30970, "media_tweets": 2974},
    "profile_bio": {"description": "no state is the best state",
      "entities": {"description": {"urls": [{"expanded_url": "https://go.dev", "display_url": "go.dev", "url": "https://t.co/x"}]}}}
  }}}
}`

func TestParseUserNewSchema(t *testing.T) {
	u := parseUserByScreenName(decode(t, newSchemaUserFixture))
	if u == nil {
		t.Fatal("nil user")
	}
	if len(u.DescriptionLinks) != 1 {
		t.Fatalf("profile_bio description links not read: %+v", u.DescriptionLinks)
	}
	if u.CreatedAt == "" {
		t.Error("CreatedAt is empty, want the core value")
	}
	checkFields(t, []fieldCheck{
		{"FollowersCount", u.FollowersCount, 11936016},
		{"FriendsCount", u.FriendsCount, 3},
		{"StatusesCount", u.StatusesCount, 30970},
		{"MediaCount", u.MediaCount, 2974},
		{"Description", u.Description, "no state is the best state"},
		{"DescriptionLinks[0].URL", u.DescriptionLinks[0].URL, "https://go.dev"},
		{"ScreenName", u.ScreenName, "jack"},
		{"Blue", u.Blue, true},
	})
}

// richTweetFixture carries media, entities, a quoted tweet, and a poll card.
const richTweetFixture = `{
  "data": {"x": {"instructions": [{"type": "TimelineAddEntries", "entries": [
    {"entryId": "tweet-r", "content": {"itemContent": {"tweet_results": {"result": {
      "__typename": "Tweet", "rest_id": "50",
      "core": {"user_results": {"result": {"legacy": {"screen_name": "rich", "name": "Rich"}}}},
      "legacy": {
        "full_text": "look #golang @bob https://t.co/x", "created_at": "d",
        "conversation_id_str": "50", "in_reply_to_status_id_str": "49", "in_reply_to_screen_name": "carol",
        "bookmark_count": 3, "quoted_status_result": {"result": {
          "__typename": "Tweet", "rest_id": "40",
          "core": {"user_results": {"result": {"legacy": {"screen_name": "q", "name": "Q"}}}},
          "legacy": {"full_text": "quoted body"}}},
        "entities": {
          "hashtags": [{"text": "golang"}], "symbols": [{"text": "GO"}],
          "user_mentions": [{"id_str": "7", "screen_name": "bob", "name": "Bob"}],
          "urls": [{"expanded_url": "https://go.dev", "display_url": "go.dev", "url": "https://t.co/x"}]},
        "extended_entities": {"media": [
          {"type": "photo", "media_url_https": "https://pbs/photo.jpg"},
          {"type": "video", "media_url_https": "https://pbs/thumb.jpg",
            "expanded_url": "https://x.com/orig/status/999/video/1",
            "additional_media_info": {"source_user": {"id_str": "8", "screen_name": "orig", "name": "Orig"}},
            "video_info": {"duration_millis": 5000, "variants": [
              {"content_type": "video/mp4", "bitrate": 256000, "url": "https://v/lo.mp4"},
              {"content_type": "video/mp4", "bitrate": 832000, "url": "https://v/hi.mp4"}]}}]}
      },
      "birdwatch_pivot": {"subtitle": {"text": "Readers added context"}},
      "card": {"legacy": {"name": "poll2choice_text_only", "binding_values": [
        {"key": "choice1_label", "value": {"string_value": "Yes"}},
        {"key": "choice1_count", "value": {"string_value": "10"}},
        {"key": "choice2_label", "value": {"string_value": "No"}},
        {"key": "choice2_count", "value": {"string_value": "3"}},
        {"key": "counts_are_final", "value": {"boolean_value": true}}]}}
    }}}}}
  ]}]}}
}`

// richTweet parses the rich fixture and returns its single tweet. The rich
// assertions are split across focused tests, so they all start here.
func richTweet(t *testing.T) Tweet {
	t.Helper()
	tweets, _ := parseTimelineTweets(decode(t, richTweetFixture))
	if len(tweets) != 1 {
		t.Fatalf("want 1 tweet, got %d", len(tweets))
	}
	return tweets[0]
}

func TestParseRichTweetReplyFields(t *testing.T) {
	tw := richTweet(t)
	checkFields(t, []fieldCheck{
		{"ConversationID", tw.ConversationID, "50"},
		{"InReplyToTweetID", tw.InReplyToTweetID, "49"},
		{"InReplyToScreenName", tw.InReplyToScreenName, "carol"},
		{"BookmarkCount", tw.BookmarkCount, 3},
		{"CommunityNote", tw.CommunityNote, "Readers added context"},
	})
}

func TestParseRichTweetEntities(t *testing.T) {
	tw := richTweet(t)
	if len(tw.Hashtags) != 1 || len(tw.Cashtags) != 1 {
		t.Fatalf("hashtags/cashtags: %+v / %+v", tw.Hashtags, tw.Cashtags)
	}
	if len(tw.Mentions) != 1 {
		t.Fatalf("mentions: %+v", tw.Mentions)
	}
	if len(tw.Links) != 1 {
		t.Fatalf("links: %+v", tw.Links)
	}
	checkFields(t, []fieldCheck{
		{"Hashtags[0]", tw.Hashtags[0], "golang"},
		{"Cashtags[0]", tw.Cashtags[0], "GO"},
		{"Mentions[0].ScreenName", tw.Mentions[0].ScreenName, "bob"},
		{"Links[0].URL", tw.Links[0].URL, "https://go.dev"},
		{"Links[0].TCoURL", tw.Links[0].TCoURL, "https://t.co/x"},
	})
}

func TestParseRichTweetMedia(t *testing.T) {
	tw := richTweet(t)
	if tw.Media == nil || len(tw.Media.Photos) != 1 || len(tw.Media.Videos) != 1 {
		t.Fatalf("media: %+v", tw.Media)
	}
	checkFields(t, []fieldCheck{
		{"Videos[0].DurationMS", tw.Media.Videos[0].DurationMS, 5000},
		{"len(Videos[0].Variants)", len(tw.Media.Videos[0].Variants), 2},
	})
}

func TestParseRichTweetQuotedAndPoll(t *testing.T) {
	tw := richTweet(t)
	if tw.Quoted == nil {
		t.Fatal("quoted is nil")
	}
	if tw.Card == nil || tw.Card.Poll == nil || len(tw.Card.Poll.Options) != 2 {
		t.Fatalf("card/poll: %+v", tw.Card)
	}
	checkFields(t, []fieldCheck{
		{"Quoted.RestID", tw.Quoted.RestID, "40"},
		{"Quoted.Text", tw.Quoted.Text, "quoted body"},
		{"Card.Type", tw.Card.Type, "poll"},
		{"Poll.Options[0].Label", tw.Card.Poll.Options[0].Label, "Yes"},
		{"Poll.Options[0].Votes", tw.Card.Poll.Options[0].Votes, 10},
		{"Poll.Finished", tw.Card.Poll.Finished, true},
	})
}

func TestParseRichTweetAttribution(t *testing.T) {
	tw := richTweet(t)
	if tw.Attribution == nil {
		t.Fatal("attribution is nil")
	}
	checkFields(t, []fieldCheck{
		{"Attribution.ScreenName", tw.Attribution.ScreenName, "orig"},
		{"Attribution.RestID", tw.Attribution.RestID, "8"},
		{"AttributionLink", tw.AttributionLink, "/orig/status/999"},
	})
}

// richUserFixture carries profile images, blue, protected, pinned ids, and counts.
const richUserFixture = `{
  "data": {"user": {"result": {
    "rest_id": "200", "is_blue_verified": true, "verified_type": "Business",
    "legacy": {"screen_name": "acme", "name": "Acme", "followers_count": 9,
      "favourites_count": 4, "listed_count": 2, "media_count": 8, "protected": true,
      "profile_image_url_https": "https://pbs/pic.jpg", "profile_banner_url": "https://pbs/ban.jpg",
      "pinned_tweet_ids_str": ["111"],
      "entities": {"description": {"urls": [{"expanded_url": "https://acme.co", "display_url": "acme.co", "url": "https://t.co/y"}]}}}
  }}}
}`

func TestParseRichUser(t *testing.T) {
	u := parseUserByScreenName(decode(t, richUserFixture))
	if u == nil {
		t.Fatal("nil user")
	}
	if len(u.PinnedTweetIDs) != 1 {
		t.Fatalf("pinned: %+v", u.PinnedTweetIDs)
	}
	if len(u.DescriptionLinks) != 1 {
		t.Fatalf("description links: %+v", u.DescriptionLinks)
	}
	checkFields(t, []fieldCheck{
		{"Blue", u.Blue, true},
		{"BlueType", u.BlueType, "Business"},
		{"Protected", u.Protected, true},
		{"FavouritesCount", u.FavouritesCount, 4},
		{"ListedCount", u.ListedCount, 2},
		{"MediaCount", u.MediaCount, 8},
		{"ProfileImageURL", u.ProfileImageURL, "https://pbs/pic.jpg"},
		{"ProfileBannerURL", u.ProfileBannerURL, "https://pbs/ban.jpg"},
		{"PinnedTweetIDs[0]", u.PinnedTweetIDs[0], "111"},
		{"DescriptionLinks[0].URL", u.DescriptionLinks[0].URL, "https://acme.co"},
	})
}

// TestOpsEmbedded confirms every wired op is present in the embedded ops.json.
func TestOpsEmbedded(t *testing.T) {
	wired := []string{
		"UserByScreenName", "UserTweets", "UserTweetsAndReplies", "UserMedia",
		"UserHighlightsTweets", "TweetDetail", "SearchTimeline", "Retweeters",
		"Followers", "Following", "ListLatestTweetsTimeline", "CommunityTweetsTimeline",
		"Bookmarks", "HomeTimeline",
	}
	for _, op := range wired {
		if _, err := spec(op); err != nil {
			t.Errorf("op %s missing from ops.json: %v", op, err)
		}
	}
}

// TestParseUsersByIds checks the UsersByRestIds flat-array parser (data.users[].result).
func TestParseUsersByIds(t *testing.T) {
	p := decode(t, `{"data":{"users":[
	  {"result":{"__typename":"User","rest_id":"11","legacy":{"screen_name":"a","name":"A","followers_count":3}}},
	  {"result":{"__typename":"User","rest_id":"22","legacy":{"screen_name":"b","name":"B"}}}
	]}}`)
	us := parseUsersByIds(p)
	if len(us) != 2 {
		t.Fatalf("got %d users, want 2", len(us))
	}
	if us[0].RestID != "11" || us[0].ScreenName != "a" || us[0].FollowersCount != 3 {
		t.Errorf("user[0] = %+v", us[0])
	}
	if us[1].RestID != "22" {
		t.Errorf("user[1] rest_id = %q", us[1].RestID)
	}
}

// TestParseTweetsByIds checks the TweetResultsByRestIds parser (data.tweetResult[].result).
func TestParseTweetsByIds(t *testing.T) {
	p := decode(t, `{"data":{"tweetResult":[
	  {"result":{"__typename":"Tweet","rest_id":"1","core":{"user_results":{"result":{"legacy":{"screen_name":"al","name":"Al"}}}},"legacy":{"full_text":"hi","lang":"en"}}},
	  {"result":{"__typename":"Tweet","rest_id":"2","core":{"user_results":{"result":{"legacy":{"screen_name":"bo","name":"Bo"}}}},"legacy":{"full_text":"yo","lang":"en"}}}
	]}}`)
	tw := parseTweetsByIds(p)
	if len(tw) != 2 {
		t.Fatalf("got %d tweets, want 2", len(tw))
	}
	if tw[0].RestID != "1" || tw[0].Text != "hi" || tw[0].UserScreenName != "al" {
		t.Errorf("tweet[0] = %+v", tw[0])
	}
}
