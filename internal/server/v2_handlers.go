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
	v.Get("/users/{id}", s.v2UserByID)
	v.Get("/users", s.v2UsersByIDs)
	v.Get("/users/{id}/tweets", s.v2UserTweets)
	v.Get("/tweets/{id}", s.v2Tweet)
	v.Get("/tweets", s.v2TweetsByIDs)
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
		return apiv2.UsersEnvelope(c, users, sel, true)
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
		return apiv2.UsersEnvelope(c, users, sel, true)
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
		return apiv2.TweetsEnvelope(c, tws, sel, true)
	})
}

// v2UserTweets serves GET /2/users/{id}/tweets, the reverse-chronological author
// timeline with v2 max_results/pagination_token paging.
func (s *Server) v2UserTweets(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sel := apiv2.ParseSelection(r.URL.Query())
	max := v2MaxResults(r)
	token := r.URL.Query().Get("pagination_token")
	s.serveV2(w, r, false, "UserTweets", func(c *xapi.XClient) (apiv2.Envelope, error) {
		tws, next, err := c.UserTweets(id, max, token)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		env, err := apiv2.TweetsEnvelope(c, tws, sel, true)
		if err != nil {
			return apiv2.Envelope{}, err
		}
		if env.Meta != nil {
			env.Meta.NextToken = next
		}
		return env, nil
	})
}
