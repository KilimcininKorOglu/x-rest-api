package xapi

import (
	"fmt"
	"net/url"
)

// Spaces. A Space's live stream is served over a REST 1.1 endpoint keyed by the
// Space's media key, which is read from the AudioSpaceById GraphQL result.

const liveStreamStatusURL = "https://x.com/i/api/1.1/live_video_stream/status/"

// SpaceStreamStatus resolves a Space's media key via AudioSpaceById, then reads
// its live stream status. It errors when the Space exposes no media key (e.g. it
// has ended or never went live).
func (c *XClient) SpaceStreamStatus(spaceID string) (*LiveStreamStatus, error) {
	raw, err := c.call("AudioSpaceById", map[string]any{"id": spaceID})
	if err != nil {
		return nil, err
	}
	mediaKey := asString(dig(raw, "data", "audioSpace", "metadata", "media_key"))
	if mediaKey == "" {
		return nil, fmt.Errorf("space %q has no media key (not live?)", spaceID)
	}
	params := url.Values{
		"client":                   {"web"},
		"use_syndication_guest_id": {"false"},
		"cookie_set_host":          {"x.com"},
	}
	resp, err := c.callFormGet("LiveVideoStreamStatus", liveStreamStatusURL+mediaKey, params)
	if err != nil {
		return nil, err
	}
	return parseLiveStreamStatus(resp, mediaKey), nil
}

// parseLiveStreamStatus maps the camelCase stream-status reply to LiveStreamStatus.
func parseLiveStreamStatus(m map[string]any, mediaKey string) *LiveStreamStatus {
	st := &LiveStreamStatus{
		SessionID:          asString(m["sessionId"]),
		ChatToken:          asString(m["chatToken"]),
		LifecycleToken:     asString(m["lifecycleToken"]),
		ShareURL:           asString(m["shareUrl"]),
		ChatPermissionType: asString(m["chatPermissionType"]),
		MediaKey:           mediaKey,
	}
	if src := asMap(m["source"]); src != nil {
		st.Source = &LiveStreamSource{
			Location:              asString(src["location"]),
			NoRedirectPlaybackURL: asString(src["noRedirectPlaybackUrl"]),
			Status:                asString(src["status"]),
			StreamType:            asString(src["streamType"]),
		}
	}
	return st
}
