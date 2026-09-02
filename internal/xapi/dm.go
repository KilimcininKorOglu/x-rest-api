package xapi

import "net/url"

// Direct messages. These are REST v1.1 endpoints (not GraphQL), so they hardcode
// the URL and send query/form params rather than a queryId. The inbox and a
// conversation share the same conversation/message raw shapes.

const (
	dmConversationURL  = "https://x.com/i/api/1.1/dm/conversation/"
	dmInboxInitialURL  = "https://x.com/i/api/1.1/dm/inbox_initial_state.json"
	dmInboxTimelineURL = "https://x.com/i/api/1.1/dm/inbox_timeline/trusted.json"
	dmDeleteURL        = "https://x.com/i/api/1.1/dm/"
	dmNewURL           = "https://x.com/i/api/1.1/dm/new2.json"
	// dmExt is x.com's pre-encoded ext param listing the message extensions to hydrate.
	dmExt = "mediaColor,altText,businessAffiliationsLabel,mediaStats,highlightedLabel,parodyCommentaryFanLabel,voiceInfo,birdwatchPivot,superFollowMetadata,unmentionInfo,editControl,article"
)

// baseDMParams renders the shared DM query params (BaseDMParams +
// DMUserIncludeParams). url.Values.Encode escapes commas in ext, matching x.com.
func baseDMParams() url.Values {
	return url.Values{
		"nsfw_filtering_enabled": {"false"}, "filter_low_quality": {"true"}, "include_quality": {"all"},
		"dm_secret_conversations_enabled": {"false"}, "krs_registration_enabled": {"false"},
		"cards_platform": {"Web-12"}, "include_cards": {"1"}, "include_ext_alt_text": {"true"},
		"include_ext_limited_action_results": {"true"}, "include_quote_count": {"true"},
		"include_reply_count": {"1"}, "tweet_mode": {"extended"}, "include_ext_views": {"true"},
		"include_groups": {"true"}, "include_inbox_timelines": {"true"}, "include_ext_media_color": {"true"},
		"supports_reactions": {"true"}, "supports_edit": {"true"}, "include_ext_edit_control": {"true"},
		"include_ext_business_affiliations_label": {"true"}, "ext": {dmExt},
		"include_profile_interstitial_type": {"1"}, "include_blocking": {"1"}, "include_blocked_by": {"1"},
		"include_followed_by": {"1"}, "include_want_retweets": {"1"}, "include_mute_edge": {"1"},
		"include_can_dm": {"1"}, "include_can_media_tag": {"1"}, "include_ext_is_blue_verified": {"1"},
		"include_ext_verified_type": {"1"}, "include_ext_profile_image_shape": {"1"}, "skip_status": {"1"},
	}
}

// Inbox returns the DM inbox. An empty cursor fetches the initial state; a cursor
// pages the inbox timeline forward.
func (c *XClient) Inbox(cursor string) (*Inbox, error) {
	if cursor == "" {
		p := baseDMParams()
		p.Set("dm_users", "true")
		p.Set("include_ext_parody_commentary_fan_label", "true")
		out, err := c.callFormGet("DMInboxInitial", dmInboxInitialURL, p)
		if err != nil {
			return nil, err
		}
		st := asMap(out["inbox_initial_state"])
		return buildInbox(st, asString(st["cursor"]), asString(st["last_seen_event_id"]),
			asString(st["trusted_last_seen_event_id"]), asString(st["untrusted_last_seen_event_id"])), nil
	}
	p := baseDMParams()
	p.Set("dm_users", "false")
	p.Set("max_id", cursor)
	out, err := c.callFormGet("DMInboxTimeline", dmInboxTimelineURL, p)
	if err != nil {
		return nil, err
	}
	st := asMap(out["inbox_timeline"])
	return buildInbox(st, asString(st["min_entry_id"]), "", "", ""), nil
}

// Conversation returns one conversation's messages (recent to oldest). A cursor
// (the oldest message id from a previous page) loads older history.
func (c *XClient) Conversation(id, cursor string) (*Conversation, error) {
	p := baseDMParams()
	p.Set("dm_users", "false")
	p.Set("include_conversation_info", "true")
	if cursor != "" {
		p.Set("max_id", cursor)
		p.Set("context", "FETCH_DM_CONVERSATION_HISTORY")
	} else {
		p.Set("context", "FETCH_DM_CONVERSATION")
	}
	out, err := c.callFormGet("DMConversation", dmConversationURL+id+".json", p)
	if err != nil {
		return nil, err
	}
	ct := asMap(out["conversation_timeline"])
	byConv := groupMessages(asSlice(ct["entries"]))
	convs := asMap(ct["conversations"])
	raw := asMap(convs[id])
	if raw == nil {
		for _, v := range convs { // fall back to the only conversation present
			raw = asMap(v)
			break
		}
	}
	return parseConversation(raw, byConv[id]), nil
}

// DeleteConversation leaves and removes a conversation from the inbox.
func (c *XClient) DeleteConversation(id string) error {
	form := url.Values{
		"dm_secret_conversations_enabled": {"false"}, "krs_registration_enabled": {"false"},
		"cards_platform": {"Web-12"}, "include_cards": {"1"}, "include_ext_alt_text": {"true"},
		"include_ext_limited_action_results": {"true"}, "include_quote_count": {"true"},
		"include_reply_count": {"1"}, "tweet_mode": {"extended"}, "include_ext_views": {"true"},
		"dm_users": {"false"}, "include_groups": {"true"}, "include_inbox_timelines": {"true"},
		"include_ext_media_color": {"true"}, "supports_reactions": {"true"}, "supports_edit": {"true"},
		"include_conversation_info": {"true"},
	}
	_, err := c.callForm("DMDeleteConversation", dmDeleteURL+id+"/delete.json", form)
	return err
}

// SendDirectMessage sends a text message into an existing conversation and returns
// the created message. conversationID is the inbox conversation id (for a 1:1 it
// is "senderId-recipientId"). The reply carries the new message under event.message.
func (c *XClient) SendDirectMessage(conversationID, text string) (*DirectMessage, error) {
	payload := map[string]any{
		"conversation_id":     conversationID,
		"recipient_ids":       false,
		"text":                text,
		"cards_platform":      "Web-12",
		"include_cards":       1,
		"include_quote_count": true,
		"dm_users":            false,
	}
	resp, err := c.callJSON("DMNew", dmNewURL, payload)
	if err != nil {
		return nil, err
	}
	if dm := parseDM(asMap(dig(resp, "event", "message"))); dm != nil {
		return dm, nil
	}
	// Fall back to what we know when the reply shape is unexpected, so a successful
	// send is never reported as a parse failure.
	return &DirectMessage{ConversationID: conversationID, Text: text}, nil
}

// buildInbox assembles an Inbox from a raw inbox state (initial or timeline).
func buildInbox(state map[string]any, cursor, lastSeen, trusted, untrusted string) *Inbox {
	if state == nil {
		return &Inbox{}
	}
	byConv := groupMessages(asSlice(state["entries"]))
	var convs []Conversation
	for cid, v := range asMap(state["conversations"]) {
		if conv := parseConversation(asMap(v), byConv[cid]); conv != nil {
			convs = append(convs, *conv)
		}
	}
	return &Inbox{
		Conversations: convs, Cursor: cursor, LastSeenEventID: lastSeen,
		TrustedLastSeenEventID: trusted, UntrustedLastSeenEventID: untrusted,
	}
}

// groupMessages parses entry[].message into DirectMessages keyed by conversation.
func groupMessages(entries []any) map[string][]DirectMessage {
	out := map[string][]DirectMessage{}
	for _, e := range entries {
		msg := asMap(dig(asMap(e), "message"))
		if msg == nil {
			continue
		}
		if dm := parseDM(msg); dm != nil {
			out[dm.ConversationID] = append(out[dm.ConversationID], *dm)
		}
	}
	return out
}

// parseDM builds a DirectMessage from a raw message object (message_data holds
// the fields; the outer object carries conversation_id).
func parseDM(msg map[string]any) *DirectMessage {
	md := asMap(msg["message_data"])
	if md == nil {
		md = msg
	}
	id := firstStr(asString(md["id"]), asString(msg["id"]))
	if id == "" {
		return nil
	}
	return &DirectMessage{
		ID:             id,
		ConversationID: firstStr(asString(msg["conversation_id"]), asString(md["conversation_id"])),
		SenderID:       firstStr(asString(md["sender_id"]), asString(msg["sender_id"])),
		RecipientID:    firstStr(asString(md["recipient_id"]), asString(msg["recipient_id"])),
		Text:           firstStr(asString(md["text"]), asString(msg["text"])),
		CreatedAt:      epochToISO(firstNonNil(md["time"], msg["time"])),
		EditCount:      asInt(md["edit_count"]),
		MediaURLs:      parseDMMedia(md),
	}
}

// parseDMMedia collects attachment urls: a quoted tweet's expanded_url plus any
// card image binding values.
func parseDMMedia(md map[string]any) []string {
	att := asMap(md["attachment"])
	if att == nil {
		return nil
	}
	var urls []string
	if u := asString(dig(att, "tweet", "expanded_url")); u != "" {
		urls = append(urls, u)
	}
	bv := asMap(dig(att, "card", "binding_values"))
	for _, k := range []string{"thumbnail_image", "photo_image_full_size", "summary_photo_image"} {
		if u := asString(dig(bv, k, "image_value", "url")); u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

// parseConversation builds a Conversation from a raw conversation object plus its
// already-parsed messages.
func parseConversation(conv map[string]any, msgs []DirectMessage) *Conversation {
	if conv == nil {
		return nil
	}
	id := asString(conv["conversation_id"])
	if id == "" {
		return nil
	}
	typ := asString(conv["type"])
	if typ == "" {
		typ = "ONE_TO_ONE"
	}
	var parts []string
	for _, p := range asSlice(conv["participants"]) {
		if uid := asString(asMap(p)["user_id"]); uid != "" {
			parts = append(parts, uid)
		}
	}
	avatar := asString(conv["avatar_image_https"])
	if avatar == "" {
		avatar = asString(dig(conv, "avatar", "image", "original_info", "url"))
	}
	return &Conversation{
		ID:                    id,
		Type:                  typ,
		Name:                  asString(conv["name"]),
		AvatarURL:             avatar,
		Participants:          parts,
		Trusted:               asBool(conv["trusted"]),
		Muted:                 asBool(conv["muted"]),
		NotificationsDisabled: asBool(conv["notifications_disabled"]),
		HasMore:               asString(conv["status"]) == "HAS_MORE",
		LastActivityAt:        epochToISO(conv["sort_timestamp"]),
		LastMessageID:         asString(conv["sort_event_id"]),
		Messages:              msgs,
	}
}

// firstStr returns the first non-empty string.
func firstStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// firstNonNil returns the first non-nil value.
func firstNonNil(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}
