package xapi

// Community notes (Birdwatch). A tweet's parsed CommunityNote holds only the one
// note x.com decided to display. BirdwatchFetchNotes returns every note written
// on the tweet, including proposals still awaiting ratings, so a caller can see
// what was contested before anything reached the timeline.

import "fmt"

// TweetNotes returns every community note written on a tweet.
func (c *XClient) TweetNotes(tweetID string) (*CommunityNotes, error) {
	payload, err := c.call("BirdwatchFetchNotes", map[string]any{"tweet_id": tweetID})
	if err != nil {
		return nil, err
	}
	res := asMap(dig(payload, "data", "tweet_result_by_rest_id", "result"))
	if res == nil {
		return nil, fmt.Errorf("TweetNotes: %s", responseErr(payload))
	}
	return &CommunityNotes{
		TweetID:       tweetID,
		Misleading:    parseNoteList(dig(res, "misleading_birdwatch_notes", "notes")),
		NotMisleading: parseNoteList(dig(res, "not_misleading_birdwatch_notes", "notes")),
		CanWriteNote:  asBool(res["can_user_write_notes_on_post_author"]),
	}, nil
}

// parseNoteList maps one of the two note groups.
func parseNoteList(v any) []CommunityNoteEntry {
	var out []CommunityNoteEntry
	for _, it := range asSlice(v) {
		if n := parseNote(asMap(it)); n != nil {
			out = append(out, *n)
		}
	}
	return out
}

// parseNote maps a single note node. The note body and its classification live
// under data_v1, while the rating state sits on the note itself.
func parseNote(m map[string]any) *CommunityNoteEntry {
	if m == nil {
		return nil
	}
	id := asString(m["rest_id"])
	if id == "" {
		return nil
	}
	data := asMap(m["data_v1"])
	summary := asMap(data["summary"])
	return &CommunityNoteEntry{
		RestID:             id,
		Text:               asString(summary["text"]),
		Classification:     asString(data["classification"]),
		Tags:               noteTags(data),
		RatingStatus:       asString(m["rating_status"]),
		DecidedBy:          asString(m["decided_by"]),
		TrustworthySources: asBool(data["trustworthy_sources"]),
		IsMediaNote:        asBool(m["is_media_note"]),
		Language:           asString(m["language"]),
		CreatedAt:          asInt64(m["created_at"]),
		AuthorAlias:        asString(dig(m, "birdwatch_profile", "alias")),
		Sources:            noteSources(summary),
	}
}

// noteTags reads whichever tag list the classification put the note in, because
// x.com names the field after the verdict rather than using one key.
func noteTags(data map[string]any) []string {
	var out []string
	for _, key := range []string{"misleading_tags", "not_misleading_tags"} {
		for _, t := range asSlice(data[key]) {
			if s := asString(t); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// noteSources pulls the links cited in the note text. x.com sends only the t.co
// form here, with no expanded url, so that is what the caller gets.
func noteSources(summary map[string]any) []TextLink {
	var out []TextLink
	for _, e := range asSlice(summary["entities"]) {
		url := asString(dig(asMap(e), "ref", "url"))
		if url == "" {
			continue
		}
		out = append(out, TextLink{URL: url, TCoURL: url})
	}
	return out
}
