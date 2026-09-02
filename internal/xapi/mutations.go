package xapi

import "fmt"

// Write mutations. These are gated behind ENABLE_WRITES at the server layer and
// perform real account actions, so they are kept separate from the read surfaces.

// CreateTweet posts a tweet. When replyToID is set, it posts a reply; when
// mediaIDs are given (from UploadMedia), it attaches them.
func (c *XClient) CreateTweet(text, replyToID string, mediaIDs []string) (*Tweet, error) {
	vars := map[string]any{"tweet_text": text}
	if replyToID != "" {
		vars["reply"] = map[string]any{
			"in_reply_to_tweet_id":   replyToID,
			"exclude_reply_user_ids": []string{},
		}
	}
	if len(mediaIDs) > 0 {
		ents := make([]map[string]any, 0, len(mediaIDs))
		for _, id := range mediaIDs {
			ents = append(ents, map[string]any{"media_id": id, "tagged_users": []string{}})
		}
		vars["media"] = map[string]any{"media_entities": ents, "possibly_sensitive": false}
	}
	payload, err := c.call("CreateTweet", vars)
	if err != nil {
		return nil, err
	}
	res := asMap(dig(payload, "data", "create_tweet", "tweet_results", "result"))
	tw := parseTweet(res)
	if tw == nil {
		return nil, fmt.Errorf("CreateTweet: no tweet in response")
	}
	return tw, nil
}

// FavoriteTweet likes a tweet by id.
func (c *XClient) FavoriteTweet(tweetID string) error {
	_, err := c.call("FavoriteTweet", map[string]any{"tweet_id": tweetID})
	return err
}

// CreateRetweet reposts a tweet by id and returns the new retweet's rest_id.
func (c *XClient) CreateRetweet(tweetID string) (string, error) {
	payload, err := c.call("CreateRetweet", map[string]any{"tweet_id": tweetID, "dark_request": false})
	if err != nil {
		return "", err
	}
	return asString(dig(payload, "data", "create_retweet", "retweet_results", "result", "rest_id")), nil
}

// DeleteTweet deletes a tweet by id.
func (c *XClient) DeleteTweet(tweetID string) error {
	_, err := c.call("DeleteTweet", map[string]any{"tweet_id": tweetID})
	return err
}

// UnfavoriteTweet removes a like from a tweet by id.
func (c *XClient) UnfavoriteTweet(tweetID string) error {
	_, err := c.call("UnfavoriteTweet", map[string]any{"tweet_id": tweetID})
	return err
}

// DeleteRetweet removes a repost. The variable is source_tweet_id (the original
// tweet's id), not the retweet's id.
func (c *XClient) DeleteRetweet(sourceTweetID string) error {
	_, err := c.call("DeleteRetweet", map[string]any{"source_tweet_id": sourceTweetID})
	return err
}

// CreateBookmark bookmarks a tweet by id.
func (c *XClient) CreateBookmark(tweetID string) error {
	_, err := c.call("CreateBookmark", map[string]any{"tweet_id": tweetID})
	return err
}

// DeleteBookmark removes a bookmark by tweet id.
func (c *XClient) DeleteBookmark(tweetID string) error {
	_, err := c.call("DeleteBookmark", map[string]any{"tweet_id": tweetID})
	return err
}

// ScheduledTweets returns the account's scheduled (unsent) tweets as the raw
// GraphQL response, because the shape is not the standard timeline envelope.
func (c *XClient) ScheduledTweets() (map[string]any, error) {
	return c.call("FetchScheduledTweets", nil)
}

// ScheduleTweet schedules a tweet for a future time (executeAt is unix seconds),
// returning the raw response.
func (c *XClient) ScheduleTweet(text string, executeAt int64) (map[string]any, error) {
	vars := map[string]any{
		"post_tweet_request": map[string]any{
			"auto_populate_reply_metadata": false,
			"status":                       text,
			"exclude_reply_user_ids":       []string{},
			"media_ids":                    []string{},
		},
		"execute_at": executeAt,
	}
	return c.call("CreateScheduledTweet", vars)
}

// DeleteScheduledTweet cancels a scheduled tweet by its scheduled-tweet id.
func (c *XClient) DeleteScheduledTweet(id string) error {
	_, err := c.call("DeleteScheduledTweet", map[string]any{"scheduled_tweet_id": id})
	return err
}

// CreateNoteTweet posts a long-form tweet (X Premium). When replyToID is set, it
// posts a reply.
func (c *XClient) CreateNoteTweet(text, replyToID string) (*Tweet, error) {
	vars := map[string]any{"tweet_text": text}
	if replyToID != "" {
		vars["reply"] = map[string]any{
			"in_reply_to_tweet_id":   replyToID,
			"exclude_reply_user_ids": []string{},
		}
	}
	payload, err := c.call("CreateNoteTweet", vars)
	if err != nil {
		return nil, err
	}
	res := asMap(dig(payload, "data", "notetweet_create", "tweet_results", "result"))
	tw := parseTweet(res)
	if tw == nil {
		return nil, fmt.Errorf("CreateNoteTweet: no tweet in response")
	}
	return tw, nil
}
