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
