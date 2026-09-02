package apiv2

import "x-rest-api/internal/xapi"

// TweetsEnvelope builds a full v2 response for one or more tweets: the field-
// selected data object(s), resolved includes for the requested expansions, and,
// for a list, a meta.result_count. c may issue expansion lookups; a nil c limits
// expansions to referenced_tweets.id. When list is false, data is the single
// object; otherwise data is an array and meta is set.
func TweetsEnvelope(c *xapi.XClient, tweets []xapi.Tweet, sel Selection, list bool) (Envelope, error) {
	inc, err := Resolve(tweets, nil, sel, c)
	if err != nil {
		return Envelope{}, err
	}
	env := Envelope{Includes: inc}
	if !list {
		if len(tweets) > 0 {
			env.Data = TweetObject(tweets[0], sel)
		}
		return env, nil
	}
	objs := make([]map[string]any, 0, len(tweets))
	for _, t := range tweets {
		objs = append(objs, TweetObject(t, sel))
	}
	env.Data = objs
	env.Meta = &Meta{ResultCount: len(objs)}
	return env, nil
}

// UsersEnvelope builds a full v2 response for one or more users, mirroring
// TweetsEnvelope. The only user expansion is pinned_tweet_id, which c resolves
// into includes.tweets.
func UsersEnvelope(c *xapi.XClient, users []xapi.XUser, sel Selection, list bool) (Envelope, error) {
	inc, err := Resolve(nil, users, sel, c)
	if err != nil {
		return Envelope{}, err
	}
	env := Envelope{Includes: inc}
	if !list {
		if len(users) > 0 {
			env.Data = UserObject(users[0], sel)
		}
		return env, nil
	}
	objs := make([]map[string]any, 0, len(users))
	for _, u := range users {
		objs = append(objs, UserObject(u, sel))
	}
	env.Data = objs
	env.Meta = &Meta{ResultCount: len(objs)}
	return env, nil
}
