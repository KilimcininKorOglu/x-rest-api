package xapi

import "fmt"

// Broadcasts are x.com's live video surface (BroadcastQuery), separate from audio
// Spaces. A Broadcast's stream is served over the same REST 1.1 endpoint Spaces
// use, keyed by the media key read from the Broadcast result.

// GetBroadcast returns a live video Broadcast's metadata (BroadcastQuery).
func (c *XClient) GetBroadcast(id string) (*Broadcast, error) {
	payload, err := c.call("BroadcastQuery", map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	b := parseBroadcast(asMap(dig(payload, "data", "broadcast")))
	if b == nil {
		return nil, fmt.Errorf("GetBroadcast: %s", responseErr(payload))
	}
	return b, nil
}

// BroadcastStreamStatus resolves a Broadcast's media key, then reads its live
// stream status. It errors when the Broadcast exposes no media key.
func (c *XClient) BroadcastStreamStatus(id string) (*LiveStreamStatus, error) {
	b, err := c.GetBroadcast(id)
	if err != nil {
		return nil, err
	}
	if b.MediaKey == "" {
		return nil, fmt.Errorf("broadcast %q has no media key (not live?)", id)
	}
	return c.streamStatusByMediaKey(b.MediaKey)
}

// parseBroadcast maps a broadcast node to a Broadcast. x.com sends the title under
// "status" and the broadcaster under user_result or user_results.
func parseBroadcast(b map[string]any) *Broadcast {
	if b == nil {
		return nil
	}
	id := asString(b["broadcast_id"])
	if id == "" {
		return nil
	}
	out := &Broadcast{
		ID:                 id,
		Title:              asString(b["status"]),
		State:              asString(b["state"]),
		ThumbnailURL:       asString(b["image_url"]),
		MediaKey:           asString(b["media_key"]),
		TotalWatched:       asInt(b["total_watched"]),
		StartTime:          asInt64(b["start_time"]),
		EndTime:            asInt64(b["end_time"]),
		ReplayStart:        asInt64(dig(b, "edited_replay", "start_time")),
		AvailableForReplay: asBool(b["available_for_replay"]),
	}
	user := asMap(dig(b, "user_result", "result"))
	if user == nil {
		user = asMap(dig(b, "user_results", "result"))
	}
	out.Broadcaster = parseUserResult(user)
	return out
}

// CommunityMedia returns a community's media-only timeline.
func (c *XClient) CommunityMedia(communityID string, count int, cursor string) ([]Tweet, string, error) {
	return paginate(c, "CommunityMediaTimeline", map[string]any{"communityId": communityID},
		parseTimelineTweets, tweetKey, count, cursor)
}

// CommunityHashtags returns a community's timeline filtered to one hashtag. The
// tag is sent without a leading '#', which x.com adds itself.
func (c *XClient) CommunityHashtags(communityID, hashtag string, count int, cursor string) ([]Tweet, string, error) {
	vars := map[string]any{"communityId": communityID, "hashtags": []string{hashtag}}
	return paginate(c, "CommunityHashtagsTimeline", vars, parseTimelineTweets, tweetKey, count, cursor)
}
