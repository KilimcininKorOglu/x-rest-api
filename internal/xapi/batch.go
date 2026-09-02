package xapi

// Batch lookups and single-tweet engagement reads that do NOT use the shared
// timeline envelope. UsersByRestIds/TweetResultsByRestIds return a flat array
// under data.users / data.tweetResult, so they need their own tiny parsers.

// maxBatchIDs caps a single batch lookup, because x.com rejects oversized id lists.
const maxBatchIDs = 100

// parseUsersByIds reads data.users[].result into XUsers (UsersByRestIds).
func parseUsersByIds(payload map[string]any) []XUser {
	var out []XUser
	for _, el := range asSlice(dig(payload, "data", "users")) {
		if u := parseUserResult(asMap(dig(asMap(el), "result"))); u != nil && u.RestID != "" {
			out = append(out, *u)
		}
	}
	return out
}

// parseTweetsByIds reads data.tweetResult[].result into Tweets
// (TweetResultsByRestIds). The response key is tweetResult even though it is a list.
func parseTweetsByIds(payload map[string]any) []Tweet {
	var out []Tweet
	for _, el := range asSlice(dig(payload, "data", "tweetResult")) {
		if tw := parseTweet(asMap(dig(asMap(el), "result"))); tw != nil {
			out = append(out, *tw)
		}
	}
	return out
}

// clampIDs trims a batch id list to maxBatchIDs.
func clampIDs(ids []string) []string {
	if len(ids) > maxBatchIDs {
		return ids[:maxBatchIDs]
	}
	return ids
}

// UsersByIDs fetches multiple profiles in one call (UsersByRestIds).
func (c *XClient) UsersByIDs(ids []string) ([]XUser, error) {
	payload, err := c.call("UsersByRestIds", map[string]any{"userIds": clampIDs(ids)})
	if err != nil {
		return nil, err
	}
	return parseUsersByIds(payload), nil
}

// identity reduces a profile to its id<->username mapping.
func identity(u XUser) UserIdentity {
	return UserIdentity{ID: u.RestID, Username: u.ScreenName}
}

// IdentitiesByIDs maps many numeric ids to {id, username} in one call, reusing
// UsersByIDs (UsersByRestIds).
func (c *XClient) IdentitiesByIDs(ids []string) ([]UserIdentity, error) {
	users, err := c.UsersByIDs(ids)
	if err != nil {
		return nil, err
	}
	out := make([]UserIdentity, 0, len(users))
	for _, u := range users {
		out = append(out, identity(u))
	}
	return out, nil
}

// ResolveHandles maps many @handles to {id, username}, one UserByScreenName call
// per handle (x.com has no batch handle lookup). Unknown handles are skipped.
func (c *XClient) ResolveHandles(handles []string) ([]UserIdentity, error) {
	out := make([]UserIdentity, 0, len(handles))
	for _, h := range clampIDs(handles) {
		u, err := c.GetUser(h)
		if err != nil {
			return nil, err
		}
		if u != nil && u.RestID != "" {
			out = append(out, identity(*u))
		}
	}
	return out, nil
}

// ProfilesByHandles fetches full profiles for many @handles, one UserByScreenName
// call per handle (x.com has no batch handle lookup). Unknown handles are skipped.
func (c *XClient) ProfilesByHandles(handles []string) ([]XUser, error) {
	out := make([]XUser, 0, len(handles))
	for _, h := range clampIDs(handles) {
		u, err := c.GetUser(h)
		if err != nil {
			return nil, err
		}
		if u != nil && u.RestID != "" {
			out = append(out, *u)
		}
	}
	return out, nil
}

// LatestTweet returns a user's most recent tweet (handle or numeric id), or nil
// when the user has no tweets. It reads a single-item timeline page.
func (c *XClient) LatestTweet(handleOrID string) (*Tweet, error) {
	tweets, _, err := c.UserTweets(handleOrID, 1, "")
	if err != nil {
		return nil, err
	}
	if len(tweets) == 0 {
		return nil, nil
	}
	return &tweets[0], nil
}

// LatestTweets returns the most recent tweet of each user (handle or numeric id),
// one UserTweets call per user. Users with no tweets are skipped.
func (c *XClient) LatestTweets(users []string) ([]Tweet, error) {
	out := make([]Tweet, 0, len(users))
	for _, u := range clampIDs(users) {
		t, err := c.LatestTweet(u)
		if err != nil {
			return nil, err
		}
		if t != nil {
			out = append(out, *t)
		}
	}
	return out, nil
}

// TweetsByIDs fetches multiple tweets in one call (TweetResultsByRestIds).
func (c *XClient) TweetsByIDs(ids []string) ([]Tweet, error) {
	payload, err := c.call("TweetResultsByRestIds", map[string]any{"tweetIds": clampIDs(ids)})
	if err != nil {
		return nil, err
	}
	return parseTweetsByIds(payload), nil
}

// Favoriters returns the users who liked a tweet (mirrors Retweeters).
func (c *XClient) Favoriters(tweetID string, count int, cursor string) ([]XUser, string, error) {
	return paginate(c, "Favoriters", map[string]any{"tweetId": tweetID},
		parseTimelineUsers, userKey, count, cursor)
}
