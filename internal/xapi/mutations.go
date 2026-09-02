package xapi

import (
	"fmt"
	"strconv"
)

// Write mutations. These are gated behind ENABLE_WRITES at the server layer and
// perform real account actions, so they are kept separate from the read surfaces.

// CreateTweet posts a tweet. When replyToID is set it posts a reply; when quoteID
// is set it quotes that tweet; when mediaIDs are given (from UploadMedia) it
// attaches them.
func (c *XClient) CreateTweet(text, replyToID string, mediaIDs []string, quoteID string) (*Tweet, error) {
	vars := map[string]any{"tweet_text": text}
	if replyToID != "" {
		vars["reply"] = map[string]any{
			"in_reply_to_tweet_id":   replyToID,
			"exclude_reply_user_ids": []string{},
		}
	}
	if quoteID != "" {
		vars["attachment_url"] = "https://x.com/i/status/" + quoteID
	}
	if len(mediaIDs) > 0 {
		ents := make([]map[string]any, 0, len(mediaIDs))
		for _, id := range mediaIDs {
			ent := map[string]any{"tagged_users": []string{}}
			// x.com expects media_id as an int64, not a string.
			if n, err := strconv.ParseInt(id, 10, 64); err == nil {
				ent["media_id"] = n
			} else {
				ent["media_id"] = id
			}
			ents = append(ents, ent)
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
		return nil, fmt.Errorf("CreateTweet: %s", responseErr(payload))
	}
	return tw, nil
}

// responseErr renders a GraphQL 200-with-errors payload's first error message, or
// a generic note when there is none, so a mutation that returns no result still
// surfaces why.
func responseErr(payload map[string]any) string {
	for _, e := range asSlice(payload["errors"]) {
		if m := asString(asMap(e)["message"]); m != "" {
			return m
		}
	}
	return "no result in response"
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

// HideReply hides a reply to one of the account's tweets. The account must be the
// conversation author; otherwise x.com returns a 200 with an authorization error
// (code 37), so check the tweet_moderate_put:"Done" result rather than the status.
func (c *XClient) HideReply(tweetID string) error {
	payload, err := c.call("ModerateTweet", map[string]any{"tweetId": tweetID})
	if err != nil {
		return err
	}
	if asString(dig(payload, "data", "tweet_moderate_put")) != "Done" {
		return fmt.Errorf("ModerateTweet: %s", responseErr(payload))
	}
	return nil
}

// UnhideReply reverses HideReply, un-hiding a previously hidden reply by id. Like
// HideReply it must check tweet_unmoderate_put:"Done", because an authorization
// failure returns a 200 with an errors array.
func (c *XClient) UnhideReply(tweetID string) error {
	payload, err := c.call("UnmoderateTweet", map[string]any{"tweetId": tweetID})
	if err != nil {
		return err
	}
	if asString(dig(payload, "data", "tweet_unmoderate_put")) != "Done" {
		return fmt.Errorf("UnmoderateTweet: %s", responseErr(payload))
	}
	return nil
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

// RemoveFollower forces a user to stop following the authenticated account.
func (c *XClient) RemoveFollower(handleOrID string) error {
	uid, err := c.resolveUID(handleOrID)
	if err != nil {
		return err
	}
	_, err = c.call("RemoveFollower", map[string]any{"target_user_id": uid})
	return err
}

// CreateList creates a list and returns its id.
func (c *XClient) CreateList(name, description string, isPrivate bool) (string, error) {
	payload, err := c.call("CreateList", map[string]any{
		"isPrivate": isPrivate, "name": name, "description": description,
	})
	if err != nil {
		return "", err
	}
	// The created list arrives under data.create_list or data.list depending on
	// the schema generation; take id_str from whichever is present.
	id := asString(dig(payload, "data", "create_list", "id_str"))
	if id == "" {
		id = asString(dig(payload, "data", "list", "id_str"))
	}
	if id == "" {
		return "", fmt.Errorf("CreateList: %s", responseErr(payload))
	}
	return id, nil
}

// DeleteList deletes a list by id.
func (c *XClient) DeleteList(id string) error {
	_, err := c.call("DeleteList", map[string]any{"listId": id})
	return err
}

// UpdateList updates a list's name, description and/or visibility. Only non-nil
// fields are sent, so a caller can change one attribute without clobbering others.
func (c *XClient) UpdateList(id string, name, description *string, isPrivate *bool) error {
	vars := map[string]any{"listId": id}
	if name != nil {
		vars["name"] = *name
	}
	if description != nil {
		vars["description"] = *description
	}
	if isPrivate != nil {
		vars["isPrivate"] = *isPrivate
	}
	_, err := c.call("UpdateList", vars)
	return err
}

// ListAddMember adds a user to a list.
func (c *XClient) ListAddMember(listID, userID string) error {
	_, err := c.call("ListAddMember", map[string]any{"listId": listID, "userId": userID})
	return err
}

// ListRemoveMember removes a user from a list.
func (c *XClient) ListRemoveMember(listID, userID string) error {
	_, err := c.call("ListRemoveMember", map[string]any{"listId": listID, "userId": userID})
	return err
}

// MuteList mutes a list by id.
func (c *XClient) MuteList(id string) error {
	_, err := c.call("MuteList", map[string]any{"listId": id})
	return err
}

// UnmuteList unmutes a list by id.
func (c *XClient) UnmuteList(id string) error {
	_, err := c.call("UnmuteList", map[string]any{"listId": id})
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
