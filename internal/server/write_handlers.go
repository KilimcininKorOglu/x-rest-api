package server

import (
	"encoding/json"
	"errors"
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

// errWriteForbidden signals a key without can_write -> 403.
var errWriteForbidden = errors.New("write forbidden")

// writeAllowed enforces the write gate and resolves the target account without
// writing a response. Writes need the global enable_writes setting AND a can_write
// key, and a specific account (no rotation), because a write acts as one identity.
// It returns errWritesDisabled, errWriteForbidden, or a pickAccount error.
func (s *Server) writeAllowed(r *http.Request, op string) (store.Account, error) {
	if !s.store.GetSettingBool(store.SettingEnableWrites, false) {
		return store.Account{}, errWritesDisabled
	}
	ri := getReqInfo(r)
	if ri == nil || ri.apiKey == nil || !ri.apiKey.CanWrite {
		return store.Account{}, errWriteForbidden
	}
	acct, _, err := s.pickAccount(r, true, op)
	if err != nil {
		return store.Account{}, err
	}
	setAccount(r, acct.ID)
	return acct, nil
}

// writeGuard is writeAllowed with the v1 error response written on refusal.
func (s *Server) writeGuard(w http.ResponseWriter, r *http.Request, op string) (store.Account, bool) {
	acct, err := s.writeAllowed(r, op)
	switch {
	case err == nil:
		return acct, true
	case errors.Is(err, errWritesDisabled):
		recordErr(r, errWritesDisabled)
		writeError(w, http.StatusForbidden, "writes are disabled (enable them in the admin panel)")
	case errors.Is(err, errWriteForbidden):
		writeError(w, http.StatusForbidden, "this API key is not allowed to write")
	default:
		s.failPick(w, r, err)
	}
	return store.Account{}, false
}

func (s *Server) clientFor(acct store.Account) *xapi.XClient {
	return xapi.NewClientFor(s.sess, toXAPI(acct))
}

// applyWriteEffect reacts to a write's upstream error (writes are pinned, so never
// rotate): a ban disables the account, a rate limit cools it for the op, stale
// features trigger a refresh. It writes no response, so v1 and v2 write layers
// share the effects.
func (s *Server) applyWriteEffect(id int64, op string, err error) {
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
}

// failWrite applies the write side effects and writes the v1 error response.
func (s *Server) failWrite(w http.ResponseWriter, r *http.Request, id int64, op string, err error) {
	s.applyWriteEffect(id, op, err)
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

func (s *Server) muteUser(w http.ResponseWriter, r *http.Request) {
	s.actWriteHandle(w, r, "MuteUser", "muted", (*xapi.XClient).Mute)
}

func (s *Server) unmuteUser(w http.ResponseWriter, r *http.Request) {
	s.actWriteHandle(w, r, "UnmuteUser", "unmuted", (*xapi.XClient).Unmute)
}

func (s *Server) blockUser(w http.ResponseWriter, r *http.Request) {
	s.actWriteHandle(w, r, "BlockUser", "blocked", (*xapi.XClient).Block)
}

func (s *Server) unblockUser(w http.ResponseWriter, r *http.Request) {
	s.actWriteHandle(w, r, "UnblockUser", "unblocked", (*xapi.XClient).Unblock)
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

// createListBody is the JSON body for POST /v1/lists.
type createListBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"is_private"`
}

// updateListBody is the JSON body for PATCH /v1/lists/{id}; nil fields are left
// unchanged.
type updateListBody struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsPrivate   *bool   `json:"is_private"`
}

// listMemberBody is the JSON body for POST /v1/lists/{id}/members.
type listMemberBody struct {
	UserID string `json:"user_id"`
}

func (s *Server) createList(w http.ResponseWriter, r *http.Request) {
	var body createListBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "provide name")
		return
	}
	acct, ok := s.writeGuard(w, r, "CreateList")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	id, err := cli.CreateList(body.Name, body.Description, body.IsPrivate)
	s.pool.Observe(acct.ID, "CreateList", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "CreateList", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"id": id}})
}

func (s *Server) deleteList(w http.ResponseWriter, r *http.Request) {
	s.actWrite(w, r, "DeleteList", "deleted", (*xapi.XClient).DeleteList)
}

func (s *Server) updateList(w http.ResponseWriter, r *http.Request) {
	var body updateListBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	acct, ok := s.writeGuard(w, r, "UpdateList")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := cli.UpdateList(chi.URLParam(r, "id"), body.Name, body.Description, body.IsPrivate)
	s.pool.Observe(acct.ID, "UpdateList", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "UpdateList", err)
		return
	}
	writeData(w, map[string]string{"status": "updated"})
}

func (s *Server) listAddMember(w http.ResponseWriter, r *http.Request) {
	var body listMemberBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.UserID == "" {
		writeError(w, http.StatusBadRequest, "provide user_id")
		return
	}
	acct, ok := s.writeGuard(w, r, "ListAddMember")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := cli.ListAddMember(chi.URLParam(r, "id"), body.UserID)
	s.pool.Observe(acct.ID, "ListAddMember", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "ListAddMember", err)
		return
	}
	writeData(w, map[string]string{"status": "added"})
}

func (s *Server) listRemoveMember(w http.ResponseWriter, r *http.Request) {
	acct, ok := s.writeGuard(w, r, "ListRemoveMember")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := cli.ListRemoveMember(chi.URLParam(r, "id"), chi.URLParam(r, "userId"))
	s.pool.Observe(acct.ID, "ListRemoveMember", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "ListRemoveMember", err)
		return
	}
	writeData(w, map[string]string{"status": "removed"})
}

func (s *Server) muteList(w http.ResponseWriter, r *http.Request) {
	s.actWrite(w, r, "MuteList", "muted", (*xapi.XClient).MuteList)
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	s.actWrite(w, r, "DMDeleteConversation", "deleted", (*xapi.XClient).DeleteConversation)
}

// grokChatBody is the JSON body for POST /v1/grok/chat. conversation_id continues
// an existing chat; omit it to start a new one. The flags default to true.
type grokChatBody struct {
	Messages            []xapi.GrokMessage `json:"messages"`
	ConversationID      string             `json:"conversation_id"`
	ReturnSearchResults *bool              `json:"return_search_results"`
	ReturnCitations     *bool              `json:"return_citations"`
}

func (s *Server) grokChat(w http.ResponseWriter, r *http.Request) {
	var body grokChatBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "provide messages")
		return
	}
	acct, ok := s.writeGuard(w, r, "GrokAddResponse")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	res, err := cli.GrokChat(body.Messages, body.ConversationID, boolOr(body.ReturnSearchResults, true), boolOr(body.ReturnCitations, true))
	s.pool.Observe(acct.ID, "GrokAddResponse", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "GrokAddResponse", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

// boolOr returns *p when set, else def.
func boolOr(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}

// sendDMBody is the JSON body for POST /v1/dm/conversations/{id}/messages.
type sendDMBody struct {
	Text string `json:"text"`
}

func (s *Server) sendDM(w http.ResponseWriter, r *http.Request) {
	var body sendDMBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Text == "" {
		writeError(w, http.StatusBadRequest, "provide text")
		return
	}
	acct, ok := s.writeGuard(w, r, "DMNew")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	dm, err := cli.SendDirectMessage(chi.URLParam(r, "id"), body.Text)
	s.pool.Observe(acct.ID, "DMNew", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "DMNew", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": dm})
}

// usernameBody is the JSON body for POST /v1/account/username.
type usernameBody struct {
	Username string `json:"username"`
}

// updateProfileBody is the JSON body for PATCH /v1/account/profile; nil fields
// are left unchanged.
type updateProfileBody struct {
	Name        *string `json:"name"`
	URL         *string `json:"url"`
	Location    *string `json:"location"`
	Description *string `json:"description"`
}

// imageBody / bannerBody carry a base64-encoded image.
type imageBody struct {
	ImageBase64 string `json:"image_base64"`
}
type bannerBody struct {
	BannerBase64 string `json:"banner_base64"`
}

// passwordBody is the JSON body for POST /v1/account/password.
type passwordBody struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) removeFollower(w http.ResponseWriter, r *http.Request) {
	s.actWriteHandle(w, r, "RemoveFollower", "removed", (*xapi.XClient).RemoveFollower)
}

func (s *Server) changeUsername(w http.ResponseWriter, r *http.Request) {
	var body usernameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Username == "" {
		writeError(w, http.StatusBadRequest, "provide username")
		return
	}
	acct, ok := s.writeGuard(w, r, "ChangeUsername")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := cli.ChangeUsername(body.Username)
	s.pool.Observe(acct.ID, "ChangeUsername", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "ChangeUsername", err)
		return
	}
	writeData(w, map[string]string{"status": "username changed"})
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	var body updateProfileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	acct, ok := s.writeGuard(w, r, "UpdateProfile")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := cli.UpdateProfile(body.Name, body.URL, body.Location, body.Description)
	s.pool.Observe(acct.ID, "UpdateProfile", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "UpdateProfile", err)
		return
	}
	writeData(w, map[string]string{"status": "profile updated"})
}

func (s *Server) updateProfileImage(w http.ResponseWriter, r *http.Request) {
	var body imageBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.ImageBase64 == "" {
		writeError(w, http.StatusBadRequest, "provide image_base64")
		return
	}
	acct, ok := s.writeGuard(w, r, "UpdateProfileImage")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := cli.UpdateProfileImage(body.ImageBase64)
	s.pool.Observe(acct.ID, "UpdateProfileImage", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "UpdateProfileImage", err)
		return
	}
	writeData(w, map[string]string{"status": "image updated"})
}

func (s *Server) updateProfileBanner(w http.ResponseWriter, r *http.Request) {
	var body bannerBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.BannerBase64 == "" {
		writeError(w, http.StatusBadRequest, "provide banner_base64")
		return
	}
	acct, ok := s.writeGuard(w, r, "UpdateProfileBanner")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := cli.UpdateProfileBanner(body.BannerBase64)
	s.pool.Observe(acct.ID, "UpdateProfileBanner", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "UpdateProfileBanner", err)
		return
	}
	writeData(w, map[string]string{"status": "banner updated"})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var body passwordBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.CurrentPassword == "" || body.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "provide current_password and new_password")
		return
	}
	acct, ok := s.writeGuard(w, r, "ChangePassword")
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := cli.ChangePassword(body.CurrentPassword, body.NewPassword)
	s.pool.Observe(acct.ID, "ChangePassword", cli.RateLimit())
	if err != nil {
		s.failWrite(w, r, acct.ID, "ChangePassword", err)
		return
	}
	writeData(w, map[string]string{"status": "password changed; re-capture ct0/auth_token if the session rotated"})
}

func (s *Server) unmuteList(w http.ResponseWriter, r *http.Request) {
	s.actWrite(w, r, "UnmuteList", "unmuted", (*xapi.XClient).UnmuteList)
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
