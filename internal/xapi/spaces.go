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

// fleetlineURL is the REST endpoint listing the live content (Spaces) surfaced
// from the account's network.
const fleetlineURL = "https://x.com/i/api/fleets/v1/fleetline"

// LiveSpaces returns the currently-live audio Spaces from the account's network.
// It is account-scoped, because the result depends on who the account follows.
func (c *XClient) LiveSpaces() ([]LiveSpace, error) {
	params := url.Values{"only_spaces": {"true"}}
	resp, err := c.callFormGet("Fleetline", fleetlineURL, params)
	if err != nil {
		return nil, err
	}
	return parseLiveSpaces(resp), nil
}

// parseLiveSpaces maps the fleetline threads to LiveSpace records, reading each
// thread's live_content.audiospace node.
func parseLiveSpaces(m map[string]any) []LiveSpace {
	var out []LiveSpace
	for _, t := range asSlice(m["threads"]) {
		a := asMap(dig(asMap(t), "live_content", "audiospace"))
		if a == nil {
			continue
		}
		id := asString(a["id"])
		if id == "" {
			continue
		}
		sp := LiveSpace{
			ID:                 id,
			BroadcastID:        asString(a["broadcast_id"]),
			Title:              asString(a["title"]),
			State:              asString(a["state"]),
			MediaKey:           asString(a["media_key"]),
			CreatorUserID:      asString(a["creator_twitter_user_id"]),
			Language:           asString(a["language"]),
			TotalLiveListeners: asInt(a["total_live_listeners"]),
			TotalParticipating: asInt(a["total_participating"]),
			StartedAt:          asString(a["start"]),
		}
		for _, adm := range asSlice(a["admin_twitter_user_ids"]) {
			if s := asString(adm); s != "" {
				sp.AdminUserIDs = append(sp.AdminUserIDs, s)
			}
		}
		out = append(out, sp)
	}
	return out
}

// SpaceInfo returns an Audio Space's metadata (AudioSpaceById). It works for
// ended Spaces too, because x.com still returns the metadata.
func (c *XClient) SpaceInfo(id string) (*Space, error) {
	payload, err := c.call("AudioSpaceById", map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	s := parseSpace(asMap(dig(payload, "data", "audioSpace")))
	if s == nil {
		return nil, fmt.Errorf("SpaceInfo: %s", responseErr(payload))
	}
	return s, nil
}

// parseSpace maps an audioSpace node to a Space, reading the creator id and admin
// handles from the metadata and participants.
func parseSpace(a map[string]any) *Space {
	md := asMap(a["metadata"])
	if md == nil {
		return nil
	}
	id := asString(md["rest_id"])
	if id == "" {
		return nil
	}
	s := &Space{
		ID:            id,
		State:         asString(md["state"]),
		Title:         asString(md["title"]),
		CreatorID:     asString(dig(md, "creator_results", "result", "rest_id")),
		MediaKey:      asString(md["media_key"]),
		CreatedAt:     asInt64(md["created_at"]),
		StartedAt:     asInt64(md["started_at"]),
		EndedAt:       asString(md["ended_at"]),
		LiveListeners: asInt(md["total_live_listeners"]),
	}
	for _, adm := range asSlice(dig(a, "participants", "admins")) {
		if sn := asString(asMap(adm)["twitter_screen_name"]); sn != "" {
			s.AdminScreenNames = append(s.AdminScreenNames, sn)
		}
	}
	return s
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
