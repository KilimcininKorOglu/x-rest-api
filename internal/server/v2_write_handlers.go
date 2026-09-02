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
	v.Post("/users/{id}/likes", s.v2Like)
	v.Delete("/users/{id}/likes/{tweet_id}", s.v2Unlike)
	v.Post("/users/{id}/retweets", s.v2Retweet)
	v.Delete("/users/{id}/retweets/{source_tweet_id}", s.v2Unretweet)
	v.Post("/users/{id}/following", s.v2Follow)
	v.Delete("/users/{id}/following/{target_id}", s.v2Unfollow)
	v.Post("/users/{id}/bookmarks", s.v2Bookmark)
	v.Delete("/users/{id}/bookmarks/{tweet_id}", s.v2Unbookmark)
	v.Post("/users/{id}/muting", s.v2Mute)
	v.Delete("/users/{id}/muting/{target_id}", s.v2Unmute)
	v.Post("/lists", s.v2CreateList)
	v.Put("/lists/{id}", s.v2UpdateList)
	v.Delete("/lists/{id}", s.v2DeleteList)
	v.Post("/lists/{id}/members", s.v2ListAddMember)
	v.Delete("/lists/{id}/members/{user_id}", s.v2ListRemoveMember)
	v.Post("/dm_conversations/{id}/messages", s.v2SendDM)
	v.Post("/users/{id}/blocking", s.v2Block)
	v.Delete("/users/{id}/blocking/{target_id}", s.v2Unblock)
}

// v2Block serves POST /2/users/{id}/blocking.
func (s *Server) v2Block(w http.ResponseWriter, r *http.Request) {
	var body v2FollowBody
	if !decodeV2Body(w, r, &body) {
		return
	}
	if body.TargetUserID == "" {
		writeJSON(w, http.StatusBadRequest, apiv2.Invalid("target_user_id", "The `target_user_id` field is required."))
		return
	}
	s.v2WriteResult(w, r, "BlockUser",
		func(c *xapi.XClient) error { return c.Block(body.TargetUserID) },
		map[string]any{"blocking": true})
}

// v2Unblock serves DELETE /2/users/{id}/blocking/{target_id}.
func (s *Server) v2Unblock(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "target_id")
	s.v2WriteResult(w, r, "UnblockUser",
		func(c *xapi.XClient) error { return c.Unblock(target) },
		map[string]any{"blocking": false})
}

// v2SendDMBody is the JSON body for POST /2/dm_conversations/{id}/messages.
type v2SendDMBody struct {
	Text string `json:"text"`
}

// v2SendDM serves POST /2/dm_conversations/{id}/messages.
func (s *Server) v2SendDM(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "id")
	var body v2SendDMBody
	if !decodeV2Body(w, r, &body) {
		return
	}
	if body.Text == "" {
		writeJSON(w, http.StatusBadRequest, apiv2.Invalid("text", "The `text` field is required."))
		return
	}
	acct, ok := s.writeGuardV2(w, r, "DMNew")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	dm, err := cli.SendDirectMessage(convID, body.Text)
	s.pool.Observe(acct.ID, "DMNew", cli.RateLimit())
	if err != nil {
		s.failWriteV2(w, r, acct.ID, "DMNew", err)
		return
	}
	writeJSON(w, http.StatusCreated, apiv2.Envelope{Data: map[string]any{
		"dm_conversation_id": dm.ConversationID,
		"dm_event_id":        dm.ID,
	}})
}

// v2TweetIDBody is the JSON body for the like/retweet POST endpoints.
type v2TweetIDBody struct {
	TweetID string `json:"tweet_id"`
}

// tweetIDFromBody decodes the body and returns a required tweet_id, writing a v2
// invalid-request when it is missing. It returns ok=false when a response was
// written.
func tweetIDFromBody(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body v2TweetIDBody
	if !decodeV2Body(w, r, &body) {
		return "", false
	}
	if body.TweetID == "" {
		writeJSON(w, http.StatusBadRequest, apiv2.Invalid("tweet_id", "The `tweet_id` field is required."))
		return "", false
	}
	return body.TweetID, true
}

// v2Like serves POST /2/users/{id}/likes.
func (s *Server) v2Like(w http.ResponseWriter, r *http.Request) {
	tid, ok := tweetIDFromBody(w, r)
	if !ok {
		return
	}
	s.v2WriteResult(w, r, "FavoriteTweet",
		func(c *xapi.XClient) error { return c.FavoriteTweet(tid) },
		map[string]any{"liked": true})
}

// v2Unlike serves DELETE /2/users/{id}/likes/{tweet_id}.
func (s *Server) v2Unlike(w http.ResponseWriter, r *http.Request) {
	tid := chi.URLParam(r, "tweet_id")
	s.v2WriteResult(w, r, "UnfavoriteTweet",
		func(c *xapi.XClient) error { return c.UnfavoriteTweet(tid) },
		map[string]any{"liked": false})
}

// v2Retweet serves POST /2/users/{id}/retweets.
func (s *Server) v2Retweet(w http.ResponseWriter, r *http.Request) {
	tid, ok := tweetIDFromBody(w, r)
	if !ok {
		return
	}
	s.v2WriteResult(w, r, "CreateRetweet",
		func(c *xapi.XClient) error { _, err := c.CreateRetweet(tid); return err },
		map[string]any{"retweeted": true})
}

// v2Unretweet serves DELETE /2/users/{id}/retweets/{source_tweet_id}.
func (s *Server) v2Unretweet(w http.ResponseWriter, r *http.Request) {
	tid := chi.URLParam(r, "source_tweet_id")
	s.v2WriteResult(w, r, "DeleteRetweet",
		func(c *xapi.XClient) error { return c.DeleteRetweet(tid) },
		map[string]any{"retweeted": false})
}

// v2FollowBody is the JSON body for POST /2/users/{id}/following.
type v2FollowBody struct {
	TargetUserID string `json:"target_user_id"`
}

// v2Follow serves POST /2/users/{id}/following.
func (s *Server) v2Follow(w http.ResponseWriter, r *http.Request) {
	var body v2FollowBody
	if !decodeV2Body(w, r, &body) {
		return
	}
	if body.TargetUserID == "" {
		writeJSON(w, http.StatusBadRequest, apiv2.Invalid("target_user_id", "The `target_user_id` field is required."))
		return
	}
	s.v2WriteResult(w, r, "FollowUser",
		func(c *xapi.XClient) error { return c.Follow(body.TargetUserID) },
		map[string]any{"following": true, "pending_follow": false})
}

// v2Unfollow serves DELETE /2/users/{id}/following/{target_id}.
func (s *Server) v2Unfollow(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "target_id")
	s.v2WriteResult(w, r, "UnfollowUser",
		func(c *xapi.XClient) error { return c.Unfollow(target) },
		map[string]any{"following": false})
}

// v2Bookmark serves POST /2/users/{id}/bookmarks.
func (s *Server) v2Bookmark(w http.ResponseWriter, r *http.Request) {
	tid, ok := tweetIDFromBody(w, r)
	if !ok {
		return
	}
	s.v2WriteResult(w, r, "CreateBookmark",
		func(c *xapi.XClient) error { return c.CreateBookmark(tid) },
		map[string]any{"bookmarked": true})
}

// v2Unbookmark serves DELETE /2/users/{id}/bookmarks/{tweet_id}.
func (s *Server) v2Unbookmark(w http.ResponseWriter, r *http.Request) {
	tid := chi.URLParam(r, "tweet_id")
	s.v2WriteResult(w, r, "DeleteBookmark",
		func(c *xapi.XClient) error { return c.DeleteBookmark(tid) },
		map[string]any{"bookmarked": false})
}

// v2Mute serves POST /2/users/{id}/muting.
func (s *Server) v2Mute(w http.ResponseWriter, r *http.Request) {
	var body v2FollowBody
	if !decodeV2Body(w, r, &body) {
		return
	}
	if body.TargetUserID == "" {
		writeJSON(w, http.StatusBadRequest, apiv2.Invalid("target_user_id", "The `target_user_id` field is required."))
		return
	}
	s.v2WriteResult(w, r, "MuteUser",
		func(c *xapi.XClient) error { return c.Mute(body.TargetUserID) },
		map[string]any{"muting": true})
}

// v2Unmute serves DELETE /2/users/{id}/muting/{target_id}.
func (s *Server) v2Unmute(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "target_id")
	s.v2WriteResult(w, r, "UnmuteUser",
		func(c *xapi.XClient) error { return c.Unmute(target) },
		map[string]any{"muting": false})
}

// v2CreateListBody is the JSON body for POST /2/lists.
type v2CreateListBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
}

// v2CreateList serves POST /2/lists.
func (s *Server) v2CreateList(w http.ResponseWriter, r *http.Request) {
	var body v2CreateListBody
	if !decodeV2Body(w, r, &body) {
		return
	}
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, apiv2.Invalid("name", "The `name` field is required."))
		return
	}
	acct, ok := s.writeGuardV2(w, r, "CreateList")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	id, err := cli.CreateList(body.Name, body.Description, body.Private)
	s.pool.Observe(acct.ID, "CreateList", cli.RateLimit())
	if err != nil {
		s.failWriteV2(w, r, acct.ID, "CreateList", err)
		return
	}
	writeJSON(w, http.StatusOK, apiv2.Envelope{Data: map[string]any{"id": id, "name": body.Name}})
}

// v2UpdateListBody is the JSON body for PUT /2/lists/{id}; nil fields are left
// unchanged.
type v2UpdateListBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Private     *bool   `json:"private"`
}

// v2UpdateList serves PUT /2/lists/{id}.
func (s *Server) v2UpdateList(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body v2UpdateListBody
	if !decodeV2Body(w, r, &body) {
		return
	}
	s.v2WriteResult(w, r, "UpdateList",
		func(c *xapi.XClient) error { return c.UpdateList(id, body.Name, body.Description, body.Private) },
		map[string]any{"updated": true})
}

// v2DeleteList serves DELETE /2/lists/{id}.
func (s *Server) v2DeleteList(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.v2WriteResult(w, r, "DeleteList",
		func(c *xapi.XClient) error { return c.DeleteList(id) },
		map[string]any{"deleted": true})
}

// v2UserIDBody is the JSON body for the list add-member endpoint.
type v2UserIDBody struct {
	UserID string `json:"user_id"`
}

// v2ListAddMember serves POST /2/lists/{id}/members.
func (s *Server) v2ListAddMember(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body v2UserIDBody
	if !decodeV2Body(w, r, &body) {
		return
	}
	if body.UserID == "" {
		writeJSON(w, http.StatusBadRequest, apiv2.Invalid("user_id", "The `user_id` field is required."))
		return
	}
	s.v2WriteResult(w, r, "ListAddMember",
		func(c *xapi.XClient) error { return c.ListAddMember(id, body.UserID) },
		map[string]any{"is_member": true})
}

// v2ListRemoveMember serves DELETE /2/lists/{id}/members/{user_id}.
func (s *Server) v2ListRemoveMember(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	uid := chi.URLParam(r, "user_id")
	s.v2WriteResult(w, r, "ListRemoveMember",
		func(c *xapi.XClient) error { return c.ListRemoveMember(id, uid) },
		map[string]any{"is_member": false})
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
