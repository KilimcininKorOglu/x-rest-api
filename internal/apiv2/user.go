package apiv2

import "x-rest-api/internal/xapi"

// UserObject renders a profile into a v2 object holding only the selected fields.
// id/name/username are always present (v2 default set). most_recent_tweet_id and
// withheld have no source value in our model, so they are accepted in the
// parameter but never emitted.
func UserObject(u xapi.XUser, sel Selection) map[string]any {
	out := map[string]any{
		"id":       u.RestID,
		"name":     u.Name,
		"username": u.ScreenName,
	}
	f := sel.User
	setUserScalars(out, u, f)
	setUserMetrics(out, u, f)
	setUserEntities(out, u, f)
	return out
}

// setUserScalars fills the flat fields, created_at (ISO 8601), the boolean flags,
// and pinned_tweet_id.
func setUserScalars(out map[string]any, u xapi.XUser, f map[string]bool) {
	setStr(out, f, "description", u.Description)
	setStr(out, f, "location", u.Location)
	setStr(out, f, "url", u.URL)
	setStr(out, f, "profile_image_url", u.ProfileImageURL)
	setStr(out, f, "verified_type", u.BlueType)
	set(out, f, "protected", u.Protected)
	set(out, f, "verified", u.Verified)
	if f["created_at"] {
		if iso := toISO8601(u.CreatedAt); iso != "" {
			out["created_at"] = iso
		}
	}
	if f["pinned_tweet_id"] && len(u.PinnedTweetIDs) > 0 {
		out["pinned_tweet_id"] = u.PinnedTweetIDs[0]
	}
}

// setUserMetrics fills public_metrics from the follower/following/tweet counts.
func setUserMetrics(out map[string]any, u xapi.XUser, f map[string]bool) {
	set(out, f, "public_metrics", map[string]any{
		"followers_count": u.FollowersCount,
		"following_count": u.FriendsCount,
		"tweet_count":     u.StatusesCount,
		"listed_count":    u.ListedCount,
	})
}

// setUserEntities fills entities.description.urls from the profile description
// links.
func setUserEntities(out map[string]any, u xapi.XUser, f map[string]bool) {
	if !f["entities"] || len(u.DescriptionLinks) == 0 {
		return
	}
	out["entities"] = map[string]any{
		"description": map[string]any{"urls": urlEntities(u.DescriptionLinks)},
	}
}
