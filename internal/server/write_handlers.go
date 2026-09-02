package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"x-rest-api/internal/store"
	"x-rest-api/internal/xapi"
)

// maxMediaBytes caps an uploaded media file (x.com's own video limit is ~512 MB).
const maxMediaBytes = 512 << 20

// createTweetBody is the JSON body for POST /v1/tweets.
type createTweetBody struct {
	Text     string   `json:"text"`
	ReplyTo  string   `json:"reply_to"`
	QuoteOf  string   `json:"quote_of"`  // tweet id to quote
	MediaIDs []string `json:"media_ids"` // ids from POST /v1/media
}

// writeGuard enforces the write gate and resolves the target account. Writes need
// the global enable_writes setting AND a can_write key, and a specific account
// (no rotation), because a write acts as one identity.
func (s *Server) writeGuard(w http.ResponseWriter, r *http.Request, op string) (store.Account, bool) {
	if !s.store.GetSettingBool(store.SettingEnableWrites, false) {
		recordErr(r, errWritesDisabled)
		writeError(w, http.StatusForbidden, "writes are disabled (enable them in the admin panel)")
		return store.Account{}, false
	}
	ri := getReqInfo(r)
	if ri == nil || ri.apiKey == nil || !ri.apiKey.CanWrite {
		writeError(w, http.StatusForbidden, "this API key is not allowed to write")
		return store.Account{}, false
	}
	acct, _, err := s.pickAccount(r, true, op)
	if err != nil {
		s.failPick(w, r, err)
		return store.Account{}, false
	}
	setAccount(r, acct.ID)
	return acct, true
}

func (s *Server) clientFor(acct store.Account) *xapi.XClient {
	return xapi.NewClientFor(s.sess, toXAPI(acct))
}

// failWrite reacts to a write's upstream error (writes are pinned, so never
// rotate): a ban disables the account, a rate limit cools it for the op, stale
// features trigger a refresh; then it writes the error.
func (s *Server) failWrite(w http.ResponseWriter, r *http.Request, id int64, op string, err error) {
	if up := asUpstream(err); up != nil {
		switch classifyUpstream(up, nil) {
		case kindBan:
			_ = s.store.DisableAccount(id, banReason(up))
		case kindRateLimit:
			s.pool.Fail(id, lockOp(op, up), rlStatus(up))
		case kindFeaturesStale:
			s.triggerRefresh()
		}
	}
	fail(w, r, err)
}

func (s *Server) createTweet(w http.ResponseWriter, r *http.Request) {
	var body createTweetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Text == "" && len(body.MediaIDs) == 0 && body.QuoteOf == "" {
		writeError(w, http.StatusBadRequest, "provide text, media_ids, or quote_of")
		return
	}
	acct, ok := s.writeGuard(w, r, "CreateTweet")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	tw, err := cli.CreateTweet(body.Text, body.ReplyTo, body.MediaIDs, body.QuoteOf)
	s.pool.Observe(acct.ID, "CreateTweet", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "CreateTweet", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": tw})
}

func (s *Server) likeTweet(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.writeGuard(w, r, "FavoriteTweet")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := cli.FavoriteTweet(chi.URLParam(r, "id"))
	s.pool.Observe(acct.ID, "FavoriteTweet", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "FavoriteTweet", err)
		return
	}
	writeData(w, map[string]string{"status": "liked"})
}

func (s *Server) retweet(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.writeGuard(w, r, "CreateRetweet")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	id, err := cli.CreateRetweet(chi.URLParam(r, "id"))
	s.pool.Observe(acct.ID, "CreateRetweet", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "CreateRetweet", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"retweet_id": id}})
}

// actWrite runs a no-result write action (delete/unlike/unretweet/bookmark) that
// takes the tweet id from the path, handling the write gate, per-op observe, and
// upstream-error classification uniformly.
func (s *Server) actWrite(w http.ResponseWriter, r *http.Request, op, status string, act func(*xapi.XClient, string) error) {
	acct, ok := s.writeGuard(w, r, op)
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := act(cli, chi.URLParam(r, "id"))
	s.pool.Observe(acct.ID, op, cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, op, err)
		return
	}
	writeData(w, map[string]string{"status": status})
}

// actWriteHandle is actWrite for endpoints keyed on {handle} instead of {id}
// (follow/unfollow), passing the handle through to the client method.
func (s *Server) actWriteHandle(w http.ResponseWriter, r *http.Request, op, status string, act func(*xapi.XClient, string) error) {
	acct, ok := s.writeGuard(w, r, op)
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := act(cli, chi.URLParam(r, "handle"))
	s.pool.Observe(acct.ID, op, cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, op, err)
		return
	}
	writeData(w, map[string]string{"status": status})
}

func (s *Server) followUser(w http.ResponseWriter, r *http.Request) {
	s.actWriteHandle(w, r, "FollowUser", "followed", (*xapi.XClient).Follow)
}

func (s *Server) unfollowUser(w http.ResponseWriter, r *http.Request) {
	s.actWriteHandle(w, r, "UnfollowUser", "unfollowed", (*xapi.XClient).Unfollow)
}

func (s *Server) deleteTweet(w http.ResponseWriter, r *http.Request) {
	s.actWrite(w, r, "DeleteTweet", "deleted", (*xapi.XClient).DeleteTweet)
}

func (s *Server) unlikeTweet(w http.ResponseWriter, r *http.Request) {
	s.actWrite(w, r, "UnfavoriteTweet", "unliked", (*xapi.XClient).UnfavoriteTweet)
}

func (s *Server) unretweet(w http.ResponseWriter, r *http.Request) {
	s.actWrite(w, r, "DeleteRetweet", "unretweeted", (*xapi.XClient).DeleteRetweet)
}

func (s *Server) createBookmark(w http.ResponseWriter, r *http.Request) {
	s.actWrite(w, r, "CreateBookmark", "bookmarked", (*xapi.XClient).CreateBookmark)
}

func (s *Server) deleteBookmark(w http.ResponseWriter, r *http.Request) {
	s.actWrite(w, r, "DeleteBookmark", "unbookmarked", (*xapi.XClient).DeleteBookmark)
}

// scheduleTweetBody is the JSON body for POST /v1/scheduled.
type scheduleTweetBody struct {
	Text      string `json:"text"`
	ExecuteAt int64  `json:"execute_at"` // unix seconds; when the tweet should post
}

// uploadMedia uploads a media file (multipart form field "file") and returns its
// media_id, which POST /v1/tweets accepts in media_ids.
func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.writeGuard(w, r, "MediaUpload")
	if !ok {
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing multipart file field 'file'")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMediaBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read uploaded file")
		return
	}
	mediaType := hdr.Header.Get("Content-Type")
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	cli := s.clientFor(acct)
	id, err := cli.UploadMedia(data, mediaType)
	s.pool.Observe(acct.ID, "MediaUpload", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "MediaUpload", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"media_id": id}})
}

// getScheduled returns the account's scheduled tweets (raw, account-scoped read).
func (s *Server) getScheduled(w http.ResponseWriter, r *http.Request) {
	s.serveRead(w, r, true, "FetchScheduledTweets", func(c *xapi.XClient) (any, string, error) {
		m, err := c.ScheduledTweets()
		return m, "", err
	})
}

// scheduleTweet schedules a tweet for a future unix time.
func (s *Server) scheduleTweet(w http.ResponseWriter, r *http.Request) {
	var body scheduleTweetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Text == "" || body.ExecuteAt == 0 {
		writeError(w, http.StatusBadRequest, "missing required fields text and execute_at (unix seconds)")
		return
	}
	acct, ok := s.writeGuard(w, r, "CreateScheduledTweet")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	m, err := cli.ScheduleTweet(body.Text, body.ExecuteAt)
	s.pool.Observe(acct.ID, "CreateScheduledTweet", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "CreateScheduledTweet", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": m})
}

// deleteScheduled cancels a scheduled tweet by id.
func (s *Server) deleteScheduled(w http.ResponseWriter, r *http.Request) {
	s.actWrite(w, r, "DeleteScheduledTweet", "unscheduled", (*xapi.XClient).DeleteScheduledTweet)
}

// createNote posts a long-form (note) tweet. Body is the same {text, reply_to}
// shape as POST /v1/tweets.
func (s *Server) createNote(w http.ResponseWriter, r *http.Request) {
	var body createTweetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Text == "" {
		writeError(w, http.StatusBadRequest, "missing required field text")
		return
	}
	acct, ok := s.writeGuard(w, r, "CreateNoteTweet")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	tw, err := cli.CreateNoteTweet(body.Text, body.ReplyTo)
	s.pool.Observe(acct.ID, "CreateNoteTweet", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "CreateNoteTweet", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": tw})
}
