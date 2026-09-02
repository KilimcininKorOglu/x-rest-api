package apiv2

import "x-rest-api/internal/xapi"

// Resolve builds the includes object for the requested expansions over the given
// primary tweets and users. It may issue extra upstream lookups (author profiles,
// pinned tweets) through c; c may be nil, in which case only expansions satisfiable
// from data already in hand (referenced_tweets.id) are filled. It returns nil when
// no includes were produced.
//
// Supported expansions (all rest-id based, present in our model): author_id and
// in_reply_to_user_id -> includes.users; referenced_tweets.id -> includes.tweets
// (from the already-parsed quoted/retweeted nodes); pinned_tweet_id -> includes.
// tweets. Media/poll/place expansions are not resolved in phase 1 because x.com's
// parsed model exposes no stable media_key/poll_id to key them on.
func Resolve(tweets []xapi.Tweet, users []xapi.XUser, sel Selection, c *xapi.XClient) (*Includes, error) {
	inc := &Includes{}
	seenTweets := map[string]bool{}
	addReferencedTweets(inc, seenTweets, tweets, sel)
	if err := addUserIncludes(inc, tweets, sel, c); err != nil {
		return nil, err
	}
	if err := addPinnedTweets(inc, seenTweets, users, sel, c); err != nil {
		return nil, err
	}
	if inc.empty() {
		return nil, nil
	}
	return inc, nil
}

// addID appends id to ids when it is non-empty and unseen.
func addID(ids *[]string, seen map[string]bool, id string) {
	if id != "" && !seen[id] {
		seen[id] = true
		*ids = append(*ids, id)
	}
}

// collectUserIDs gathers the author and/or reply-target ids to expand into
// includes.users, deduped and in first-seen order.
func collectUserIDs(tweets []xapi.Tweet, sel Selection) []string {
	wantAuthor := sel.Expansions["author_id"]
	wantReply := sel.Expansions["in_reply_to_user_id"]
	if !wantAuthor && !wantReply {
		return nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, t := range tweets {
		if wantAuthor {
			addID(&ids, seen, t.AuthorID)
		}
		if wantReply {
			addID(&ids, seen, t.InReplyToUserID)
		}
	}
	return ids
}

// addUserIncludes fetches the expanded author/reply-target profiles and appends
// them to includes.users.
func addUserIncludes(inc *Includes, tweets []xapi.Tweet, sel Selection, c *xapi.XClient) error {
	ids := collectUserIDs(tweets, sel)
	if len(ids) == 0 || c == nil {
		return nil
	}
	profs, err := c.UsersByIDs(ids)
	if err != nil {
		return err
	}
	for _, u := range profs {
		inc.Users = append(inc.Users, UserObject(u, sel))
	}
	return nil
}

// addReferencedTweets appends the quoted/retweeted tweets already parsed on the
// primaries to includes.tweets.
func addReferencedTweets(inc *Includes, seen map[string]bool, tweets []xapi.Tweet, sel Selection) {
	if !sel.Expansions["referenced_tweets.id"] {
		return
	}
	for _, t := range tweets {
		appendTweet(inc, seen, t.Retweeted, sel)
		appendTweet(inc, seen, t.Quoted, sel)
	}
}

// appendTweet adds a tweet to includes.tweets when it is non-nil and unseen.
func appendTweet(inc *Includes, seen map[string]bool, tw *xapi.Tweet, sel Selection) {
	if tw == nil || tw.RestID == "" || seen[tw.RestID] {
		return
	}
	seen[tw.RestID] = true
	inc.Tweets = append(inc.Tweets, TweetObject(*tw, sel))
}

// collectPinnedIDs gathers each user's first pinned tweet id, deduped.
func collectPinnedIDs(users []xapi.XUser) []string {
	seen := map[string]bool{}
	var ids []string
	for _, u := range users {
		if len(u.PinnedTweetIDs) > 0 {
			addID(&ids, seen, u.PinnedTweetIDs[0])
		}
	}
	return ids
}

// addPinnedTweets fetches users' pinned tweets and appends them to includes.tweets.
func addPinnedTweets(inc *Includes, seen map[string]bool, users []xapi.XUser, sel Selection, c *xapi.XClient) error {
	if !sel.Expansions["pinned_tweet_id"] || c == nil {
		return nil
	}
	ids := collectPinnedIDs(users)
	if len(ids) == 0 {
		return nil
	}
	tws, err := c.TweetsByIDs(ids)
	if err != nil {
		return err
	}
	for i := range tws {
		appendTweet(inc, seen, &tws[i], sel)
	}
	return nil
}
