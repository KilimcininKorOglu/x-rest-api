package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"x-rest-api/internal/apiv2"
	"x-rest-api/internal/xapi"
)

// mountV2Write registers the X API v2 compatible write endpoints on the /2 group.
// Every write goes through the write gate (enable_writes + can_write + a specific
// account), so it acts as one identity and never rotates.
func (s *Server) mountV2Write(v chi.Router) {
	v.Post("/tweets", s.v2CreateTweet)
	v.Delete("/tweets/{id}", s.v2DeleteTweet)
}

// v2CreateTweetBody is the JSON body for POST /2/tweets, mirroring the X API v2
// create-tweet request.
type v2CreateTweetBody struct {
	Text  string `json:"text"`
	Reply *struct {
		InReplyToTweetID string `json:"in_reply_to_tweet_id"`
	} `json:"reply"`
	QuoteTweetID string `json:"quote_tweet_id"`
	Media        *struct {
		MediaIDs []string `json:"media_ids"`
	} `json:"media"`
}

// v2CreateTweet serves POST /2/tweets.
func (s *Server) v2CreateTweet(w http.ResponseWriter, r *http.Request) {
	var body v2CreateTweetBody
	if !decodeV2Body(w, r, &body) {
		return
	}
	var mediaIDs []string
	if body.Media != nil {
		mediaIDs = body.Media.MediaIDs
	}
	if body.Text == "" && len(mediaIDs) == 0 && body.QuoteTweetID == "" {
		writeJSON(w, http.StatusBadRequest,
			apiv2.Invalid("text", "provide text, media.media_ids, or quote_tweet_id"))
		return
	}
	replyTo := ""
	if body.Reply != nil {
		replyTo = body.Reply.InReplyToTweetID
	}
	acct, ok := s.writeGuardV2(w, r, "CreateTweet")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	tw, err := cli.CreateTweet(body.Text, replyTo, mediaIDs, body.QuoteTweetID)
	s.pool.Observe(acct.ID, "CreateTweet", cli.RateLimit())
	if err != nil {
		s.failWriteV2(w, r, acct.ID, "CreateTweet", err)
		return
	}
	sel := apiv2.ParseSelection(r.URL.Query())
	writeJSON(w, http.StatusCreated, apiv2.Envelope{Data: apiv2.TweetObject(*tw, sel)})
}

// v2DeleteTweet serves DELETE /2/tweets/{id}.
func (s *Server) v2DeleteTweet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.v2WriteResult(w, r, "DeleteTweet",
		func(c *xapi.XClient) error { return c.DeleteTweet(id) },
		map[string]any{"deleted": true})
}
