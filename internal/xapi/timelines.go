package xapi

import (
	"strconv"
	"time"
)

// Pure parsers for x.com GraphQL responses: dict -> records, no network, so they
// can be unit-tested against captured HAR bodies.
//
// The trick that keeps this small: nearly every list endpoint (UserTweets, search,
// replies, media, list, community, home, bookmarks, retweeters, followers) returns
// the same envelope: instructions -> entries -> itemContent -> tweet_results |
// user_results. x.com only varies the root it hangs under, so a recursive walk to
// the first "instructions" list is more robust than N hardcoded paths that rot on
// every frontend release.

// ---- safe traversal helpers over decoded JSON (map[string]any) -------------- //

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// asInt coerces the loosely-typed JSON value into an int. x.com returns counts as
// numbers but the view count as a numeric string, so both are handled.
func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
	}
	return 0
}

// asInt64 coerces a JSON value to int64, for ms-epoch timestamps that overflow a
// 32-bit int.
func asInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		if i, err := strconv.ParseInt(n, 10, 64); err == nil {
			return i
		}
	}
	return 0
}

// asBool coerces a JSON value to bool (missing/other types yield false).
func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

// epochToISO renders an epoch value (ms as number or numeric string) as an
// RFC3339 UTC string; a non-numeric string is returned unchanged (already a
// date), and anything else yields "".
func epochToISO(v any) string {
	switch n := v.(type) {
	case float64:
		return time.UnixMilli(int64(n)).UTC().Format(time.RFC3339)
	case string:
		if ms, err := strconv.ParseInt(n, 10, 64); err == nil {
			return time.UnixMilli(ms).UTC().Format(time.RFC3339)
		}
		return n
	}
	return ""
}

// dig walks nested maps by key, returning the leaf value or nil.
func dig(m map[string]any, keys ...string) any {
	cur := any(m)
	for _, k := range keys {
		mm := asMap(cur)
		if mm == nil {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

// ---- envelope walk ---------------------------------------------------------- //

// findInstructions returns the first "instructions" list anywhere in obj,
// regardless of the op-specific root it hangs under.
func findInstructions(obj any) []any {
	switch v := obj.(type) {
	case map[string]any:
		if instr := asSlice(v["instructions"]); instr != nil {
			return instr
		}
		for _, val := range v {
			if r := findInstructions(val); r != nil {
				return r
			}
		}
	case []any:
		for _, val := range v {
			if r := findInstructions(val); r != nil {
				return r
			}
		}
	}
	return nil
}

// entries flattens AddEntries.entries and AddToModule.moduleItems into one list.
func entries(payload map[string]any) []any {
	root := any(payload)
	if d := asMap(payload["data"]); d != nil {
		root = d
	}
	var out []any
	for _, ins := range findInstructions(root) {
		m := asMap(ins)
		out = append(out, asSlice(m["entries"])...)
		out = append(out, asSlice(m["moduleItems"])...)
	}
	return out
}

// entryNode returns the content|item node of an entry.
func entryNode(entry any) map[string]any {
	e := asMap(entry)
	if node := asMap(e["content"]); node != nil {
		return node
	}
	return asMap(e["item"])
}

// itemContents yields every itemContent, unwrapping module entries
// (items[].item.itemContent).
func itemContents(es []any) []map[string]any {
	var out []map[string]any
	for _, entry := range es {
		node := entryNode(entry)
		if it := asMap(node["itemContent"]); it != nil {
			out = append(out, it)
		}
		for _, sub := range asSlice(node["items"]) {
			sm := asMap(sub)
			si := asMap(dig(sm, "item", "itemContent"))
			if si == nil {
				si = asMap(sm["itemContent"])
			}
			if si != nil {
				out = append(out, si)
			}
		}
	}
	return out
}

// cursor returns the paging cursor value of the given kind (default "Bottom").
func cursor(es []any, kind string) string {
	for _, entry := range es {
		node := entryNode(entry)
		if asString(node["entryType"]) == "TimelineTimelineCursor" &&
			asString(node["cursorType"]) == kind {
			return asString(node["value"])
		}
		if it := asMap(node["itemContent"]); it != nil && asString(it["cursorType"]) == kind {
			return asString(it["value"])
		}
	}
	return ""
}

// ---- tweet & user parsing --------------------------------------------------- //

// unwrapTweet resolves a tweet_results.result into a Tweet node, unwrapping
// TweetWithVisibilityResults and skipping TweetTombstone.
func unwrapTweet(result map[string]any) map[string]any {
	if result == nil {
		return nil
	}
	if asString(result["__typename"]) == "TweetWithVisibilityResults" {
		result = asMap(result["tweet"])
	}
	if asString(result["__typename"]) == "TweetTombstone" {
		return nil
	}
	return result
}

// tweetText returns the long-form note text when present, else legacy.full_text.
func tweetText(t, legacy map[string]any) string {
	note := asMap(dig(t, "note_tweet", "note_tweet_results", "result"))
	if txt := asString(note["text"]); txt != "" {
		return txt
	}
	return asString(legacy["full_text"])
}

// tweetAuthor returns the author's screen_name and display name from the tweet's
// embedded user_results.
func tweetAuthor(t map[string]any) (screenName, name string) {
	user := asMap(dig(t, "core", "user_results", "result"))
	uleg := asMap(user["legacy"])
	ucore := asMap(user["core"])
	screenName = asString(uleg["screen_name"])
	if screenName == "" {
		screenName = asString(ucore["screen_name"])
	}
	name = asString(uleg["name"])
	if name == "" {
		name = asString(ucore["name"])
	}
	return screenName, name
}

// parseTweet turns a tweet_results.result node into a Tweet, or nil when the node
// is not a usable Tweet.
func parseTweet(result map[string]any) *Tweet {
	return parseTweetDepth(result, 0)
}

// maxNestDepth bounds quote/retweet recursion so a quote-of-a-quote does not walk
// forever; one level of nesting is enough for callers.
const maxNestDepth = 1

// parseTweetDepth parses a tweet, following quoted/retweeted nesting only while
// depth is below maxNestDepth.
func parseTweetDepth(result map[string]any, depth int) *Tweet {
	t := unwrapTweet(result)
	if t == nil {
		return nil
	}
	legacy := asMap(t["legacy"])
	if legacy == nil {
		return nil
	}
	screenName, name := tweetAuthor(t)
	restID := asString(t["rest_id"])
	if restID == "" {
		restID = asString(legacy["id_str"])
	}
	_, isRetweet := legacy["retweeted_status_result"]
	quote, _ := legacy["is_quote_status"].(bool)
	tw := &Tweet{
		RestID:              restID,
		AuthorID:            asString(dig(t, "core", "user_results", "result", "rest_id")),
		UserScreenName:      screenName,
		UserName:            name,
		CreatedAt:           asString(legacy["created_at"]),
		Text:                tweetText(t, legacy),
		Lang:                asString(legacy["lang"]),
		ReplyCount:          asInt(legacy["reply_count"]),
		RetweetCount:        asInt(legacy["retweet_count"]),
		LikeCount:           asInt(legacy["favorite_count"]),
		QuoteCount:          asInt(legacy["quote_count"]),
		ViewCount:           asInt(dig(t, "views", "count")),
		BookmarkCount:       asInt(legacy["bookmark_count"]),
		IsRetweet:           isRetweet,
		IsQuote:             quote,
		ConversationID:      asString(legacy["conversation_id_str"]),
		InReplyToTweetID:    asString(legacy["in_reply_to_status_id_str"]),
		InReplyToUserID:     asString(legacy["in_reply_to_user_id_str"]),
		InReplyToScreenName: asString(legacy["in_reply_to_screen_name"]),
		Source:              sourceLabel(asString(t["source"])),
		Hashtags:            hashtags(legacy, "hashtags"),
		Cashtags:            hashtags(legacy, "symbols"),
		Mentions:            mentions(legacy),
		Links:               entityLinks(asMap(dig(legacy, "entities"))),
		Media:               parseMedia(legacy),
		Card:                parseCard(asMap(t["card"])),
		Article:             parseEmbeddedArticle(t),
		Place:               parsePlace(asMap(legacy["place"])),
		Coordinates:         parseCoordinates(legacy),
		CommunityNote:       parseCommunityNote(t),
		URL:                 tweetURL(screenName, restID),
	}
	tw.Attribution, tw.AttributionLink = parseAttribution(legacy)
	if depth < maxNestDepth {
		tw.Quoted = parseTweetDepth(nestedResult(t, legacy, "quoted_status_result"), depth+1)
		tw.Retweeted = parseTweetDepth(nestedResult(t, legacy, "retweeted_status_result"), depth+1)
	}
	return tw
}

// nestedResult finds a quoted/retweeted status result, checking both the tweet
// node and its legacy block, because the two schema generations place it
// differently.
func nestedResult(t, legacy map[string]any, key string) map[string]any {
	if r := asMap(dig(t, key, "result")); r != nil {
		return r
	}
	return asMap(dig(legacy, key, "result"))
}

// pickCount reads a count from the legacy block when that key is present, else
// from the newer node, because the two schema generations place counts
// differently and legacy is empty on the new one.
func pickCount(legacy map[string]any, legacyKey string, node map[string]any, nodeKey string) int {
	if v, ok := legacy[legacyKey]; ok {
		return asInt(v)
	}
	return asInt(node[nodeKey])
}

// parseUserResult turns a user_results.result node into an XUser.
func parseUserResult(result map[string]any) *XUser {
	if result == nil {
		return nil
	}
	legacy := asMap(result["legacy"])
	core := asMap(result["core"])
	location := asString(dig(result, "location", "location"))
	if location == "" {
		location = asString(legacy["location"])
	}
	screenName := asString(legacy["screen_name"])
	if screenName == "" {
		screenName = asString(core["screen_name"])
	}
	name := asString(legacy["name"])
	if name == "" {
		name = asString(core["name"])
	}
	createdAt := asString(legacy["created_at"])
	if createdAt == "" {
		createdAt = asString(core["created_at"])
	}
	// The newer schema drops legacy and moves the bio into profile_bio.
	bio := asMap(result["profile_bio"])
	description := asString(legacy["description"])
	if description == "" {
		description = asString(bio["description"])
	}
	descEntities := asMap(dig(legacy, "entities", "description"))
	if len(descEntities) == 0 {
		descEntities = asMap(dig(bio, "entities", "description"))
	}
	verified, _ := legacy["verified"].(bool)
	blue, _ := result["is_blue_verified"].(bool)
	protected, _ := legacy["protected"].(bool)
	avatar := asMap(result["avatar"])
	profileImage := asString(legacy["profile_image_url_https"])
	if profileImage == "" {
		profileImage = asString(avatar["image_url"])
	}
	blueType := asString(result["verified_type"])
	if blueType == "" {
		blueType = asString(legacy["verified_type"])
	}
	// followers/following live in relationship_counts and tweet/media counts in
	// tweet_counts on the newer schema, where legacy is empty.
	rc := asMap(result["relationship_counts"])
	tc := asMap(result["tweet_counts"])
	return &XUser{
		RestID:           asString(result["rest_id"]),
		ScreenName:       screenName,
		Name:             name,
		Description:      description,
		FollowersCount:   pickCount(legacy, "followers_count", rc, "followers"),
		FriendsCount:     pickCount(legacy, "friends_count", rc, "following"),
		StatusesCount:    pickCount(legacy, "statuses_count", tc, "tweets"),
		FavouritesCount:  asInt(legacy["favourites_count"]),
		ListedCount:      asInt(legacy["listed_count"]),
		MediaCount:       pickCount(legacy, "media_count", tc, "media_tweets"),
		Verified:         verified || blue,
		Blue:             blue,
		BlueType:         blueType,
		Protected:        protected,
		CreatedAt:        createdAt,
		Location:         location,
		URL:              userURL(screenName),
		ProfileImageURL:  profileImage,
		ProfileBannerURL: asString(legacy["profile_banner_url"]),
		PinnedTweetIDs:   stringList(legacy["pinned_tweet_ids_str"]),
		DescriptionLinks: entityLinks(descEntities),
	}
}

// parseUserByScreenName extracts the profile from a UserByScreenName response.
func parseUserByScreenName(payload map[string]any) *XUser {
	result := asMap(dig(payload, "data", "user", "result"))
	if result == nil {
		return nil
	}
	return parseUserResult(result)
}

// parseTimelineTweets returns the tweets and bottom cursor from any tweet timeline.
func parseTimelineTweets(payload map[string]any) ([]Tweet, string) {
	es := entries(payload)
	var tweets []Tweet
	for _, it := range itemContents(es) {
		if tw := parseTweet(asMap(dig(it, "tweet_results", "result"))); tw != nil {
			tweets = append(tweets, *tw)
		}
	}
	return tweets, cursor(es, "Bottom")
}

// parseTimelineUsers returns the users and bottom cursor from any user timeline
// (retweeters, followers, ...).
func parseTimelineUsers(payload map[string]any) ([]XUser, string) {
	es := entries(payload)
	var users []XUser
	for _, it := range itemContents(es) {
		if u := parseUserResult(asMap(dig(it, "user_results", "result"))); u != nil && u.RestID != "" {
			users = append(users, *u)
		}
	}
	return users, cursor(es, "Bottom")
}
