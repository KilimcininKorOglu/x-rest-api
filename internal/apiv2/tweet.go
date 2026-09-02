package apiv2

import "x-rest-api/internal/xapi"

// TweetObject renders a tweet into a v2 object holding only the selected fields.
// id/text/edit_history_tweet_ids are always present (v2 default set). Fields with
// no source value in our model (possibly_sensitive, reply_settings,
// context_annotations, note_tweet, non_public_metrics, ...) are accepted in the
// parameter but never emitted, since the upstream data does not carry them.
// attachments (media_keys) is likewise omitted: x.com's parsed media does not
// expose a stable media_key to key includes.media on.
func TweetObject(t xapi.Tweet, sel Selection) map[string]any {
	out := map[string]any{
		"id":                     t.RestID,
		"text":                   t.Text,
		"edit_history_tweet_ids": []string{t.RestID},
	}
	f := sel.Tweet
	setTweetScalars(out, t, f)
	setTweetMetrics(out, t, f)
	setTweetReferences(out, t, f)
	setTweetEntities(out, t, f)
	setTweetGeo(out, t, f)
	return out
}

// setTweetScalars fills the flat string fields plus created_at (ISO 8601).
func setTweetScalars(out map[string]any, t xapi.Tweet, f map[string]bool) {
	setStr(out, f, "author_id", t.AuthorID)
	setStr(out, f, "conversation_id", t.ConversationID)
	setStr(out, f, "in_reply_to_user_id", t.InReplyToUserID)
	setStr(out, f, "lang", t.Lang)
	setStr(out, f, "source", t.Source)
	if f["created_at"] {
		if iso := toISO8601(t.CreatedAt); iso != "" {
			out["created_at"] = iso
		}
	}
}

// setTweetMetrics fills public_metrics from the engagement counts.
func setTweetMetrics(out map[string]any, t xapi.Tweet, f map[string]bool) {
	set(out, f, "public_metrics", map[string]any{
		"retweet_count":    t.RetweetCount,
		"reply_count":      t.ReplyCount,
		"like_count":       t.LikeCount,
		"quote_count":      t.QuoteCount,
		"bookmark_count":   t.BookmarkCount,
		"impression_count": t.ViewCount,
	})
}

// setTweetReferences fills referenced_tweets from retweet/quote/reply links.
func setTweetReferences(out map[string]any, t xapi.Tweet, f map[string]bool) {
	if !f["referenced_tweets"] {
		return
	}
	var refs []map[string]any
	if t.Retweeted != nil {
		refs = append(refs, ref("retweeted", t.Retweeted.RestID))
	}
	if t.Quoted != nil {
		refs = append(refs, ref("quoted", t.Quoted.RestID))
	}
	if t.InReplyToTweetID != "" {
		refs = append(refs, ref("replied_to", t.InReplyToTweetID))
	}
	if len(refs) > 0 {
		out["referenced_tweets"] = refs
	}
}

// setTweetEntities fills entities from hashtags/cashtags/mentions/urls.
func setTweetEntities(out map[string]any, t xapi.Tweet, f map[string]bool) {
	if !f["entities"] {
		return
	}
	if ent := tweetEntities(t); len(ent) > 0 {
		out["entities"] = ent
	}
}

// tweetEntities builds the v2 entities object, omitting empty groups.
func tweetEntities(t xapi.Tweet) map[string]any {
	ent := map[string]any{}
	if len(t.Hashtags) > 0 {
		ent["hashtags"] = tags(t.Hashtags)
	}
	if len(t.Cashtags) > 0 {
		ent["cashtags"] = tags(t.Cashtags)
	}
	if len(t.Mentions) > 0 {
		ent["mentions"] = mentionEntities(t.Mentions)
	}
	if len(t.Links) > 0 {
		ent["urls"] = urlEntities(t.Links)
	}
	return ent
}

// setTweetGeo fills geo from a tagged place and/or coordinates.
func setTweetGeo(out map[string]any, t xapi.Tweet, f map[string]bool) {
	if !f["geo"] {
		return
	}
	geo := map[string]any{}
	if t.Place != nil && t.Place.ID != "" {
		geo["place_id"] = t.Place.ID
	}
	if t.Coordinates != nil {
		geo["coordinates"] = map[string]any{
			"type":        "Point",
			"coordinates": []float64{t.Coordinates.Longitude, t.Coordinates.Latitude},
		}
	}
	if len(geo) > 0 {
		out["geo"] = geo
	}
}
