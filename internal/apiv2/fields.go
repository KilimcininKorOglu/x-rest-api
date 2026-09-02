package apiv2

import (
	"net/url"
	"strings"
)

// Selection is the parsed set of field and expansion parameters for one request.
// Each field map holds the requested field names for that object type, already
// merged with the v2 default set. Expansions holds the requested expansion names.
type Selection struct {
	Tweet      map[string]bool
	User       map[string]bool
	Media      map[string]bool
	Poll       map[string]bool
	Place      map[string]bool
	List       map[string]bool
	Space      map[string]bool
	Expansions map[string]bool
}

// Default field sets match X API v2: a tweet always carries id/text/
// edit_history_tweet_ids, a user always id/name/username. Requested fields add to
// these. Media/poll/place defaults are the v2 minimal keys used inside includes.
var (
	defaultTweetFields = []string{"id", "text", "edit_history_tweet_ids"}
	defaultUserFields  = []string{"id", "name", "username"}
	defaultMediaFields = []string{"media_key", "type"}
	defaultPollFields  = []string{"id", "options"}
	defaultPlaceFields = []string{"id", "full_name"}
	defaultListFields  = []string{"id", "name"}
	defaultSpaceFields = []string{"id", "state"}
)

// csvSet splits a comma-separated parameter into a set, seeded with the given
// defaults. Blank entries are dropped; whitespace is trimmed.
func csvSet(raw string, defaults []string) map[string]bool {
	set := make(map[string]bool, len(defaults)+4)
	for _, d := range defaults {
		set[d] = true
	}
	for part := range strings.SplitSeq(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			set[p] = true
		}
	}
	return set
}

// ParseSelection reads the v2 field/expansion query parameters from q. Unknown
// field names are kept in the set but simply never match a source value, so the
// full parameter surface is accepted without emitting fields we cannot populate.
func ParseSelection(q url.Values) Selection {
	return Selection{
		Tweet:      csvSet(q.Get("tweet.fields"), defaultTweetFields),
		User:       csvSet(q.Get("user.fields"), defaultUserFields),
		Media:      csvSet(q.Get("media.fields"), defaultMediaFields),
		Poll:       csvSet(q.Get("poll.fields"), defaultPollFields),
		Place:      csvSet(q.Get("place.fields"), defaultPlaceFields),
		List:       csvSet(q.Get("list.fields"), defaultListFields),
		Space:      csvSet(q.Get("space.fields"), defaultSpaceFields),
		Expansions: csvSet(q.Get("expansions"), nil),
	}
}
