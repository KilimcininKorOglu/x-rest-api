package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"x-rest-api/internal/apiv2"
	"x-rest-api/internal/xapi"
)

// mountV2 registers the X API v2 compatible read endpoints on the /2 group. The
// group already has request logging and Bearer auth applied by the caller.
func (s *Server) mountV2(v chi.Router) {
	v.Get("/users/by/username/{username}", s.v2UserByUsername)
	v.Get("/users/by", s.v2UsersByUsernames)
	v.Get("/users/me", s.v2Me)
	v.Get("/users/{id}", s.v2UserByID)
	v.Get("/users", s.v2UsersByIDs)
	v.Get("/users/{id}/tweets", s.v2UserTweets)
	v.Get("/tweets/search/recent", s.v2SearchRecent)
	v.Get("/tweets/{id}/retweeted_by", s.v2RetweetedBy)
	v.Get("/tweets/{id}/liking_users", s.v2LikingUsers)
	v.Get("/tweets/{id}", s.v2Tweet)
	v.Get("/tweets", s.v2TweetsByIDs)
	v.Get("/users/{id}/liked_tweets", s.v2LikedTweets)
	v.Get("/users/{id}/followers", s.v2Followers)
	v.Get("/users/{id}/following", s.v2FollowingList)
	v.Get("/users/{id}/timelines/reverse_chronological", s.v2ReverseChronological)
	v.Get("/users/{id}/bookmarks", s.v2Bookmarks)
	v.Get("/lists/{id}", s.v2List)
	v.Get("/lists/{id}/tweets", s.v2ListTweets)
	v.Get("/lists/{id}/members", s.v2ListMembers)
	v.Get("/dm_events", s.v2DMEvents)
	v.Get("/spaces/{id}", s.v2Space)
	v.Get("/users/{id}/blocking", s.v2Blocking)
}

// v2Blocking serves GET /2/users/{id}/blocking, the account's blocked users. It is
// account-scoped and ignores {id}.
func (s *Server) v2Blocking(w http.ResponseWriter, r *http.Request) {
	s.v2UsersPage(w, r, true, "BlockedAccountsAll",
		func(c *xapi.XClient, max int, cur string) ([]xapi.XUser, string, error) {
			return c.BlockedAccounts(max, cur)
		})
}

// v2Space serves GET /2/spaces/{id}.
func (s *Server) v2Space(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sel := apiv2.ParseSelection(r.URL.Query())
	s.serveV2(w, r, false, "AudioSpaceById", func(c *xapi.XClient) (apiv2.Envelope, error) {
		sp, err := c.SpaceInfo(id)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		return apiv2.Envelope{Data: apiv2.SpaceObject(*sp, sel)}, nil
	})
}

// v2DMEvents serves GET /2/dm_events, the account's direct-message events. It is
// account-scoped and flattens the inbox conversations into a v2 dm_events list.
func (s *Server) v2DMEvents(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("pagination_token")
	s.serveV2(w, r, true, "DMInboxInitial", func(c *xapi.XClient) (apiv2.Envelope, error) {
		inbox, err := c.Inbox(cursor)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		events := apiv2.DMEvents(inbox)
		env := apiv2.Envelope{Data: events, Meta: &apiv2.Meta{ResultCount: len(events)}}
		if inbox.Cursor != "" {
			env.Meta.NextToken = inbox.Cursor
		}
		return env, nil
	})
}

// v2List serves GET /2/lists/{id}.
func (s *Server) v2List(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sel := apiv2.ParseSelection(r.URL.Query())
	s.serveV2(w, r, false, "ListByRestId", func(c *xapi.XClient) (apiv2.Envelope, error) {
		l, err := c.ListInfo(id)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		return apiv2.Envelope{Data: apiv2.ListObject(*l, sel)}, nil
	})
}

// v2ListTweets serves GET /2/lists/{id}/tweets.
func (s *Server) v2ListTweets(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.v2TweetsPage(w, r, false, "ListLatestTweetsTimeline",
		func(c *xapi.XClient, max int, cur string) ([]xapi.Tweet, string, error) {
			return c.ListTweets(id, max, cur)
		})
}

// v2ListMembers serves GET /2/lists/{id}/members.
func (s *Server) v2ListMembers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.v2UsersPage(w, r, false, "ListMembers",
		func(c *xapi.XClient, max int, cur string) ([]xapi.XUser, string, error) {
			return c.ListMembers(id, max, cur)
		})
}

// v2Bookmarks serves GET /2/users/{id}/bookmarks, the account's bookmarked tweets.
// It is account-scoped and ignores {id}, serving the pinned account's bookmarks.
func (s *Server) v2Bookmarks(w http.ResponseWriter, r *http.Request) {
	s.v2TweetsPage(w, r, true, "Bookmarks",
		func(c *xapi.XClient, max int, cur string) ([]xapi.Tweet, string, error) {
			return c.Bookmarks(max, cur)
		})
}

// withMissing appends resource-not-found errors for requested ids/handles absent
// from the envelope's data array, so a v2 batch lookup reports missing items the
// way the official API does. It is a no-op when data is not an object array.
func withMissing(env apiv2.Envelope, requested []string, keyField, resourceType, parameter string) apiv2.Envelope {
	if objs, ok := env.Data.([]map[string]any); ok {
		env.Errors = apiv2.MissingErrors(requested, objs, keyField, resourceType, parameter)
	}
	return env
}

// requireCSV reads a required comma-separated parameter, writing a v2 invalid
// request when it is absent. It returns ok=false when the response was written.
func requireCSV(w http.ResponseWriter, r *http.Request, key string) ([]string, bool) {
	vals := csvParam(r, key)
	if len(vals) == 0 {
		writeJSON(w, http.StatusBadRequest,
			apiv2.Invalid(key, "The `"+key+"` query parameter is required."))
		return nil, false
	}
	return vals, true
}

// v2UserByUsername serves GET /2/users/by/username/{username}.
func (s *Server) v2UserByUsername(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	sel := apiv2.ParseSelection(r.URL.Query())
	s.serveV2(w, r, false, "UserByScreenName", func(c *xapi.XClient) (apiv2.Envelope, error) {
		u, err := c.GetUser(username)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		if u == nil {
			return apiv2.NotFound("user", "username", username), nil
		}
		return apiv2.UsersEnvelope(c, []xapi.XUser{*u}, sel, false)
	})
}

// v2Me serves GET /2/users/me, the authenticated account's own profile. It is
// account-scoped (Viewer).
func (s *Server) v2Me(w http.ResponseWriter, r *http.Request) {
	sel := apiv2.ParseSelection(r.URL.Query())
	s.serveV2(w, r, true, "Viewer", func(c *xapi.XClient) (apiv2.Envelope, error) {
		u, err := c.Me()
		if err != nil {
			return apiv2.Envelope{}, err
		}
		return apiv2.Envelope{Data: apiv2.UserObject(*u, sel)}, nil
	})
}

// v2UserByID serves GET /2/users/{id}.
func (s *Server) v2UserByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sel := apiv2.ParseSelection(r.URL.Query())
	s.serveV2(w, r, false, "UserByRestId", func(c *xapi.XClient) (apiv2.Envelope, error) {
		u, err := c.GetUserByID(id)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		if u == nil {
			return apiv2.NotFound("user", "id", id), nil
		}
		return apiv2.UsersEnvelope(c, []xapi.XUser{*u}, sel, false)
	})
}

// v2UsersByIDs serves GET /2/users?ids=1,2.
func (s *Server) v2UsersByIDs(w http.ResponseWriter, r *http.Request) {
	ids, ok := requireCSV(w, r, "ids")
	if !ok {
		return
	}
	sel := apiv2.ParseSelection(r.URL.Query())
	s.serveV2(w, r, false, "UsersByRestIds", func(c *xapi.XClient) (apiv2.Envelope, error) {
		users, err := c.UsersByIDs(ids)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		env, err := apiv2.UsersEnvelope(c, users, sel, true)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		return withMissing(env, ids, "id", "user", "id"), nil
	})
}

// v2UsersByUsernames serves GET /2/users/by?usernames=a,b.
func (s *Server) v2UsersByUsernames(w http.ResponseWriter, r *http.Request) {
	names, ok := requireCSV(w, r, "usernames")
	if !ok {
		return
	}
	sel := apiv2.ParseSelection(r.URL.Query())
	s.serveV2(w, r, false, "UserByScreenName", func(c *xapi.XClient) (apiv2.Envelope, error) {
		users, err := c.ProfilesByHandles(names)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		env, err := apiv2.UsersEnvelope(c, users, sel, true)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		return withMissing(env, names, "username", "user", "username"), nil
	})
}

// v2Tweet serves GET /2/tweets/{id}.
func (s *Server) v2Tweet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sel := apiv2.ParseSelection(r.URL.Query())
	s.serveV2(w, r, false, "TweetResultByRestId", func(c *xapi.XClient) (apiv2.Envelope, error) {
		tw, err := c.GetTweetResult(id)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		if tw == nil {
			return apiv2.NotFound("tweet", "id", id), nil
		}
		return apiv2.TweetsEnvelope(c, []xapi.Tweet{*tw}, sel, false)
	})
}

// v2TweetsByIDs serves GET /2/tweets?ids=1,2.
func (s *Server) v2TweetsByIDs(w http.ResponseWriter, r *http.Request) {
	ids, ok := requireCSV(w, r, "ids")
	if !ok {
		return
	}
	sel := apiv2.ParseSelection(r.URL.Query())
	s.serveV2(w, r, false, "TweetResultsByRestIds", func(c *xapi.XClient) (apiv2.Envelope, error) {
		tws, err := c.TweetsByIDs(ids)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		env, err := apiv2.TweetsEnvelope(c, tws, sel, true)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		return withMissing(env, ids, "id", "tweet", "id"), nil
	})
}

// v2SearchRecent serves GET /2/tweets/search/recent, the recent (reverse-
// chronological) tweet search with v2 max_results/next_token paging and
// newest_id/oldest_id meta.
func (s *Server) v2SearchRecent(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		writeJSON(w, http.StatusBadRequest,
			apiv2.Invalid("query", "The `query` query parameter is required."))
		return
	}
	sel := apiv2.ParseSelection(r.URL.Query())
	max := v2MaxResults(r)
	token := r.URL.Query().Get("next_token")
	s.serveV2(w, r, false, "SearchTimeline", func(c *xapi.XClient) (apiv2.Envelope, error) {
		tws, next, err := c.Search(query, "Latest", max, token)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		env, err := apiv2.TweetsEnvelope(c, tws, sel, true)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		if env.Meta != nil {
			env.Meta.NextToken = next
			if len(tws) > 0 {
				env.Meta.NewestID = tws[0].RestID
				env.Meta.OldestID = tws[len(tws)-1].RestID
			}
		}
		return env, nil
	})
}

// v2UserTweets serves GET /2/users/{id}/tweets, the author timeline.
func (s *Server) v2UserTweets(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.v2TweetsPage(w, r, false, "UserTweets",
		func(c *xapi.XClient, max int, cur string) ([]xapi.Tweet, string, error) {
			return c.UserTweets(id, max, cur)
		})
}

// v2RetweetedBy serves GET /2/tweets/{id}/retweeted_by.
func (s *Server) v2RetweetedBy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.v2UsersPage(w, r, false, "Retweeters",
		func(c *xapi.XClient, max int, cur string) ([]xapi.XUser, string, error) {
			return c.Retweeters(id, max, cur)
		})
}

// v2LikingUsers serves GET /2/tweets/{id}/liking_users.
func (s *Server) v2LikingUsers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.v2UsersPage(w, r, false, "Favoriters",
		func(c *xapi.XClient, max int, cur string) ([]xapi.XUser, string, error) {
			return c.Favoriters(id, max, cur)
		})
}

// v2LikedTweets serves GET /2/users/{id}/liked_tweets.
func (s *Server) v2LikedTweets(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.v2TweetsPage(w, r, false, "Likes",
		func(c *xapi.XClient, max int, cur string) ([]xapi.Tweet, string, error) {
			return c.Likes(id, max, cur)
		})
}

// v2Followers serves GET /2/users/{id}/followers.
func (s *Server) v2Followers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.v2UsersPage(w, r, false, "Followers",
		func(c *xapi.XClient, max int, cur string) ([]xapi.XUser, string, error) {
			return c.Followers(id, max, cur)
		})
}

// v2FollowingList serves GET /2/users/{id}/following.
func (s *Server) v2FollowingList(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.v2UsersPage(w, r, false, "Following",
		func(c *xapi.XClient, max int, cur string) ([]xapi.XUser, string, error) {
			return c.Following(id, max, cur)
		})
}

// v2ReverseChronological serves GET /2/users/{id}/timelines/reverse_chronological,
// the account's home timeline. It is account-scoped and ignores {id}, serving the
// pinned account's own home feed.
func (s *Server) v2ReverseChronological(w http.ResponseWriter, r *http.Request) {
	s.v2TweetsPage(w, r, true, "HomeLatest",
		func(c *xapi.XClient, max int, cur string) ([]xapi.Tweet, string, error) {
			return c.HomeLatest(max, cur)
		})
}
