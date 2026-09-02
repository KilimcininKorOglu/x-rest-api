package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"x-rest-api/internal/store"
	"x-rest-api/internal/xapi"
)

const (
	defaultCount    = 40
	maxCount        = 200
	maxReadAttempts = 3 // rotation retries on rate-limit walls
)

// errNotFound signals a nil result (e.g. unknown user/tweet) -> HTTP 404.
var errNotFound = errors.New("not found")

// errNeedAccount signals an account-scoped read/write without an account -> 400.
var errNeedAccount = errors.New("account required")

// errWritesDisabled signals the global write gate is off -> 403.
var errWritesDisabled = errors.New("writes disabled")

// cursorParam reads ?cursor= (empty = start from the top / auto-paginate).
func cursorParam(r *http.Request) string { return r.URL.Query().Get("cursor") }

// rawParam reads ?raw=true, requesting the unparsed GQL passthrough.
func rawParam(r *http.Request) bool {
	v := r.URL.Query().Get("raw")
	return v == "true" || v == "1"
}

// asRead adapts a typed (items, cursor, err) list result into the readFn shape.
func asRead[T any](items []T, cursor string, err error) (any, string, error) {
	return items, cursor, err
}

// countParam reads ?count=, clamping to [1, maxCount] with a default.
func countParam(r *http.Request) int {
	raw := r.URL.Query().Get("count")
	if raw == "" {
		return defaultCount
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return defaultCount
	}
	if n > maxCount {
		return maxCount
	}
	return n
}

// pickAccount selects the account for a request. X-Account header pins a specific
// account. Account-scoped requests (home/bookmarks/writes) fall back to the key's
// bound account, else errNeedAccount. Otherwise it rotates the pool for op, so an
// account cooling down on op is skipped while it still serves other ops.
func (s *Server) pickAccount(r *http.Request, scoped bool, op string) (store.Account, bool, error) {
	if label := strings.TrimSpace(r.Header.Get("X-Account")); label != "" {
		a, err := s.store.GetAccountByLabel(label)
		return a, true, err
	}
	if scoped {
		if ri := getReqInfo(r); ri != nil && ri.apiKey != nil && ri.apiKey.BoundAccountID != nil {
			a, err := s.store.GetAccount(*ri.apiKey.BoundAccountID)
			return a, true, err
		}
		return store.Account{}, true, errNeedAccount
	}
	a, err := s.pool.Next(op)
	return a, false, err
}

// readFn runs one read against a client and returns the payload plus the next
// pagination cursor ("" when there is none or the endpoint is not paginated).
type readFn func(*xapi.XClient) (any, string, error)

// serveRead runs a read against a chosen account, rotating on rate-limit walls
// when the account is not pinned, and writing the JSON response. op is the
// endpoint's primary GraphQL op, used for per-op account selection and locking.
func (s *Server) serveRead(w http.ResponseWriter, r *http.Request, scoped bool, op string, do readFn) {
	s.serveReadPub(w, r, scoped, op, do, nil)
}

// serveReadPub is serveRead with an optional public no-auth fallback (pub), used
// when the account pool is exhausted or an authed read is rejected (401/403) and
// the operator enabled it. pub is nil for endpoints FxTwitter cannot serve.
func (s *Server) serveReadPub(w http.ResponseWriter, r *http.Request, scoped bool, op string, do readFn, pub func() (any, error)) {
	var lastErr error
	for range maxReadAttempts {
		acct, pinned, err := s.pickAccount(r, scoped, op)
		if err != nil {
			if s.tryPublic(w, r, pub) {
				return
			}
			s.failPick(w, r, err)
			return
		}
		setAccount(r, acct.ID)
		cli := xapi.NewClientFor(s.sess, toXAPI(acct))
		payload, cur, err := do(cli)
		s.pool.Observe(acct.ID, op, cli.RateLimit())
		if err == nil {
			_ = s.store.MarkAccountUsed(acct.ID)
			writeResult(w, r, payload, cur)
			return
		}
		if errors.Is(err, errNotFound) {
			recordErr(r, err)
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if up := asUpstream(err); up != nil {
			if retry := s.handleUpstream(w, r, up, acct.ID, op, pinned, cli.RateLimit(), pub); retry {
				lastErr = err
				continue
			}
			return
		}
		fail(w, r, err)
		return
	}
	if s.tryPublic(w, r, pub) {
		return
	}
	fail(w, r, lastErr)
}

// rawUID serves the ?raw=true passthrough for a userId-keyed op: it resolves the
// handle to an id, then returns the unparsed GQL page.
func (s *Server) rawUID(c *xapi.XClient, op, handle, cursor string, count int) (any, string, error) {
	uid, err := c.ResolveUID(handle)
	if err != nil {
		return nil, "", err
	}
	m, err := c.CallRaw(op, map[string]any{"userId": uid}, cursor, count)
	return m, "", err
}

// rawByVars serves the ?raw=true passthrough for an op with fixed variables.
func rawByVars(c *xapi.XClient, op string, vars map[string]any, cursor string, count int) (any, string, error) {
	m, err := c.CallRaw(op, vars, cursor, count)
	return m, "", err
}

// lockOp returns the op to lock on failure: the exact failing op from the
// upstream error when present, else the endpoint's primary op. This matters for
// handlers that run more than one op (e.g. a handle lookup before the timeline).
func lockOp(primary string, up *xapi.UpstreamError) string {
	if up != nil && up.Op != "" {
		return up.Op
	}
	return primary
}

// upstreamKind classifies an x.com failure so the pool can react correctly.
type upstreamKind int

const (
	kindOther         upstreamKind = iota
	kindBan                        // bad/expired cookies or access denied -> disable account
	kindRateLimit                  // per-op rate limit -> cool the account for the op
	kindFeaturesStale              // GraphQL features/queryId outdated -> refresh
	kindTransient                  // load shed / transport blip -> retry
	kindHTMLBlock                  // anti-bot/Cloudflare HTML -> abort
)

// classifyUpstream maps an upstream error plus rate-limit headers to a kind.
func classifyUpstream(up *xapi.UpstreamError, rl *xapi.RateLimit) upstreamKind {
	switch {
	case up.HTML:
		return kindHTMLBlock
	case up.Code == 32 || up.Code == 326:
		return kindBan
	case up.Code == 88 && rl != nil && rl.Remaining > 0:
		return kindBan
	case up.Status == 403 && up.Msg == "OK":
		return kindBan
	case up.Code == 336:
		return kindFeaturesStale
	case up.Code == -1:
		return kindTransient
	case up.Status == 429 || up.Code == 88 || (rl != nil && rl.Remaining == 0):
		return kindRateLimit
	case up.Status == 404:
		return kindRateLimit
	}
	return kindOther
}

// rlStatus normalises the status used for a per-op cooldown (429 or 404).
func rlStatus(up *xapi.UpstreamError) int {
	if up.Status == 404 {
		return 404
	}
	return 429
}

// banReason renders a short reason recorded when an account is auto-disabled.
func banReason(up *xapi.UpstreamError) string {
	return fmt.Sprintf("x.com code %d: %s (http %d)", up.Code, up.Msg, up.Status)
}

// handleUpstream classifies an upstream error and either handles it terminally
// (writing the response) or returns retry=true to rotate to another account. A
// ban disables the account; a rate limit cools it for the op; stale features
// trigger a queryId refresh; a transient error retries.
func (s *Server) handleUpstream(w http.ResponseWriter, r *http.Request, up *xapi.UpstreamError, accountID int64, op string, pinned bool, rl *xapi.RateLimit, pub func() (any, error)) bool {
	switch classifyUpstream(up, rl) {
	case kindBan:
		_ = s.store.DisableAccount(accountID, banReason(up))
		if !pinned {
			return true
		}
	case kindRateLimit:
		if !pinned {
			s.pool.Fail(accountID, lockOp(op, up), rlStatus(up))
			return true
		}
	case kindFeaturesStale:
		s.triggerRefresh()
	case kindTransient:
		if !pinned {
			return true
		}
	case kindHTMLBlock:
		recordErr(r, up)
		writeError(w, http.StatusBadGateway, "upstream returned an anti-bot/HTML block; try again later")
		return false
	}
	if (up.Status == 401 || up.Status == 403) && s.tryPublic(w, r, pub) {
		return false
	}
	fail(w, r, up)
	return false
}

// tryPublic runs the public fallback when enabled; returns true if it answered.
func (s *Server) tryPublic(w http.ResponseWriter, r *http.Request, pub func() (any, error)) bool {
	if pub == nil || !s.store.GetSettingBool(store.SettingPublicFallback, false) {
		return false
	}
	payload, err := pub()
	if err != nil {
		recordErr(r, err)
		return false
	}
	writeData(w, payload)
	return true
}

// failPick maps an account-selection error to a status.
func (s *Server) failPick(w http.ResponseWriter, r *http.Request, err error) {
	recordErr(r, err)
	if errors.Is(err, errNeedAccount) {
		writeError(w, http.StatusBadRequest, "this endpoint needs a specific account: send X-Account or bind the API key to an account")
		return
	}
	writeError(w, http.StatusServiceUnavailable, err.Error())
}

// asUpstream extracts an *xapi.UpstreamError if present.
func asUpstream(err error) *xapi.UpstreamError {
	up, _ := errors.AsType[*xapi.UpstreamError](err)
	return up
}

// recordErr stores an error message on the request for logging.
func recordErr(r *http.Request, err error) {
	if ri := getReqInfo(r); ri != nil && ri.errMsg == "" {
		ri.errMsg = err.Error()
	}
}

// ---- read handlers ---------------------------------------------------------- //

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	handle := chi.URLParam(r, "handle")
	// FxTwitter's user endpoint needs a screen_name, so skip the fallback for
	// numeric ids.
	var pub func() (any, error)
	if !allDigits(handle) {
		pub = func() (any, error) { return s.sess.FetchUserPublic(handle) }
	}
	s.serveReadPub(w, r, false, "UserByScreenName", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawUserResult(c, handle)
		}
		u, err := lookupUser(c, handle)
		if err == nil && u == nil {
			return nil, "", errNotFound
		}
		return u, "", err
	}, pub)
}

// rawUserResult serves the ?raw=true profile passthrough, choosing UserByRestId
// for a numeric handle and UserByScreenName otherwise.
func rawUserResult(c *xapi.XClient, handle string) (any, string, error) {
	if allDigits(handle) {
		m, err := c.CallRaw("UserByRestId", map[string]any{"userId": handle}, "", 1)
		return m, "", err
	}
	m, err := c.CallRaw("UserByScreenName", map[string]any{"screen_name": handle}, "", 1)
	return m, "", err
}

// lookupUser resolves a profile by numeric id (UserByRestId) or handle
// (UserByScreenName).
func lookupUser(c *xapi.XClient, handle string) (*xapi.XUser, error) {
	if allDigits(handle) {
		return c.GetUserByID(handle)
	}
	return c.GetUser(handle)
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// readTweets adapts a tweet-list client method into a read handler. op is the
// primary GraphQL op, threaded through for per-op account selection and locking.
// The handle-keyed endpoints support ?cursor= (manual paging) and ?raw=true.
func (s *Server) readTweets(param, op string, fn func(*xapi.XClient, string, int, string) ([]xapi.Tweet, string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		arg, count, cursor := chi.URLParam(r, param), countParam(r), cursorParam(r)
		s.serveRead(w, r, false, op, func(c *xapi.XClient) (any, string, error) {
			if rawParam(r) {
				return s.rawUID(c, op, arg, cursor, count)
			}
			return asRead(fn(c, arg, count, cursor))
		})
	}
}

// rssTweets adapts a keyed tweet-list client method into an RSS feed handler. It
// skips the raw/csv branches: writeResult renders the feed when the path ends in
// /rss. The cursor is empty, so the feed is the first page only.
func (s *Server) rssTweets(param, op string, fn func(*xapi.XClient, string, int, string) ([]xapi.Tweet, string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		arg, count := chi.URLParam(r, param), countParam(r)
		s.serveRead(w, r, false, op, func(c *xapi.XClient) (any, string, error) {
			return asRead(fn(c, arg, count, ""))
		})
	}
}

// searchRSS runs the search query and renders the tweets as an RSS feed.
func (s *Server) searchRSS(w http.ResponseWriter, r *http.Request) {
	built, err := buildSearchQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "provide q or at least one search filter")
		return
	}
	count := countParam(r)
	s.serveRead(w, r, false, "SearchTimeline", func(c *xapi.XClient) (any, string, error) {
		return asRead(c.Search(built, "Latest", count, ""))
	})
}

// readUsers adapts a handle-keyed user-list client method into a read handler.
func (s *Server) readUsers(param, op string, fn func(*xapi.XClient, string, int, string) ([]xapi.XUser, string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		arg, count, cursor := chi.URLParam(r, param), countParam(r), cursorParam(r)
		s.serveRead(w, r, false, op, func(c *xapi.XClient) (any, string, error) {
			if rawParam(r) {
				return s.rawUID(c, op, arg, cursor, count)
			}
			return asRead(fn(c, arg, count, cursor))
		})
	}
}

// readUsersID adapts an id-keyed user-list client method (list members, community
// members/moderators) into a read handler. varKey is the GraphQL variable name for
// the raw path, because these ops key on listId/communityId, not a resolved userId.
func (s *Server) readUsersID(param, op, varKey string, fn func(*xapi.XClient, string, int, string) ([]xapi.XUser, string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		arg, count, cursor := chi.URLParam(r, param), countParam(r), cursorParam(r)
		s.serveRead(w, r, false, op, func(c *xapi.XClient) (any, string, error) {
			if rawParam(r) {
				return rawByVars(c, op, map[string]any{varKey: arg}, cursor, count)
			}
			return asRead(fn(c, arg, count, cursor))
		})
	}
}

// userAbout returns the flat AboutAccountQuery result (account origin, username
// history, identity verification). ?raw=true returns the unparsed GraphQL.
func (s *Server) userAbout(w http.ResponseWriter, r *http.Request) {
	handle := strings.TrimPrefix(chi.URLParam(r, "handle"), "@")
	s.serveRead(w, r, false, "AboutAccountQuery", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "AboutAccountQuery", map[string]any{"screenName": handle}, "", 0)
		}
		a, err := c.UserAbout(handle)
		if err == nil && a == nil {
			return nil, "", errNotFound
		}
		return a, "", err
	})
}

// sortMode maps ?sort=relevance|recency|likes to x.com's rankingMode, "" for the
// op default.
func sortMode(r *http.Request) string {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort"))) {
	case "relevance":
		return "Relevance"
	case "recency":
		return "Recency"
	case "likes":
		return "Likes"
	}
	return ""
}

// listInfo returns the raw ListByRestId result (name, description, member/subscriber
// counts). Served raw because its shape is not the standard timeline.
func (s *Server) listInfo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.serveRead(w, r, false, "ListByRestId", func(c *xapi.XClient) (any, string, error) {
		return rawByVars(c, "ListByRestId", map[string]any{"listId": id}, "", 0)
	})
}

// notifications returns the raw NotificationsTimeline result. It is account-scoped
// (the logged-in account's own notifications), so it needs a specific account.
func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	count, cursor := countParam(r), cursorParam(r)
	s.serveRead(w, r, true, "NotificationsTimeline", func(c *xapi.XClient) (any, string, error) {
		return rawByVars(c, "NotificationsTimeline", map[string]any{}, cursor, count)
	})
}

// communityInfo returns the raw CommunityQuery result (name, description, counts).
func (s *Server) communityInfo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.serveRead(w, r, false, "CommunityQuery", func(c *xapi.XClient) (any, string, error) {
		return rawByVars(c, "CommunityQuery", map[string]any{"communityId": id}, "", 0)
	})
}

// trendTimelineIDs maps a trend category to x.com's base64 timeline id.
var trendTimelineIDs = map[string]string{
	"trending":      "VGltZWxpbmU6DAC2CwABAAAACHRyZW5kaW5nAAA",
	"news":          "VGltZWxpbmU6DAC2CwABAAAABG5ld3MAAA",
	"sport":         "VGltZWxpbmU6DAC2CwABAAAABnNwb3J0cwAA",
	"entertainment": "VGltZWxpbmU6DAC2CwABAAAADWVudGVydGFpbm1lbnQAAA",
}

// trends returns the raw GenericTimelineById result for a category (default
// trending). An unknown category is passed through as a literal timeline id.
func (s *Server) trends(w http.ResponseWriter, r *http.Request) {
	cat := r.URL.Query().Get("category")
	tid, ok := trendTimelineIDs[cat]
	if !ok {
		tid = trendTimelineIDs["trending"]
		if cat != "" {
			tid = cat
		}
	}
	count, cursor := countParam(r), cursorParam(r)
	s.serveRead(w, r, false, "GenericTimelineById", func(c *xapi.XClient) (any, string, error) {
		return rawByVars(c, "GenericTimelineById", map[string]any{"timelineId": tid}, cursor, count)
	})
}

func (s *Server) getTweet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// FxTwitter returns a single tweet, not a reply thread, so the fallback
	// yields {tweet, replies: []}.
	pub := func() (any, error) {
		tw, err := s.sess.FetchTweetPublic(id)
		if err != nil {
			return nil, err
		}
		return &xapi.TweetThread{Tweet: tw, Replies: []xapi.Tweet{}}, nil
	}
	s.serveReadPub(w, r, false, "TweetDetail", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "TweetDetail", map[string]any{"focalTweetId": id}, "", 0)
		}
		th, err := c.GetTweet(id)
		return th, "", err
	}, pub)
}

func (s *Server) getTweetResult(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pub := func() (any, error) { return s.sess.FetchTweetPublic(id) }
	s.serveReadPub(w, r, false, "TweetResultByRestId", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "TweetResultByRestId", map[string]any{"tweetId": id}, "", 0)
		}
		tw, err := c.GetTweetResult(id)
		if err == nil && tw == nil {
			return nil, "", errNotFound
		}
		return tw, "", err
	}, pub)
}

func (s *Server) getRetweeters(w http.ResponseWriter, r *http.Request) {
	id, count, cursor := chi.URLParam(r, "id"), countParam(r), cursorParam(r)
	s.serveRead(w, r, false, "Retweeters", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "Retweeters", map[string]any{"tweetId": id}, cursor, count)
		}
		return asRead(c.Retweeters(id, count, cursor))
	})
}

// idsParam reads a comma-separated ?ids= list into a trimmed, non-empty slice.
func idsParam(r *http.Request) []string {
	raw := r.URL.Query().Get("ids")
	var out []string
	for p := range strings.SplitSeq(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// usersByIDs looks up many profiles in one call (UsersByRestIds).
func (s *Server) usersByIDs(w http.ResponseWriter, r *http.Request) {
	ids := idsParam(r)
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "missing required query parameter ids (comma-separated numeric ids)")
		return
	}
	s.serveRead(w, r, false, "UsersByRestIds", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "UsersByRestIds", map[string]any{"userIds": ids}, "", 0)
		}
		u, err := c.UsersByIDs(ids)
		return u, "", err
	})
}

// tweetsByIDs looks up many tweets in one call (TweetResultsByRestIds).
func (s *Server) tweetsByIDs(w http.ResponseWriter, r *http.Request) {
	ids := idsParam(r)
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "missing required query parameter ids (comma-separated numeric ids)")
		return
	}
	s.serveRead(w, r, false, "TweetResultsByRestIds", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "TweetResultsByRestIds", map[string]any{"tweetIds": ids}, "", 0)
		}
		tw, err := c.TweetsByIDs(ids)
		return tw, "", err
	})
}

// getLikers returns the users who liked a tweet (Favoriters).
func (s *Server) getLikers(w http.ResponseWriter, r *http.Request) {
	id, count, cursor := chi.URLParam(r, "id"), countParam(r), cursorParam(r)
	s.serveRead(w, r, false, "Favoriters", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "Favoriters", map[string]any{"tweetId": id}, cursor, count)
		}
		return asRead(c.Favoriters(id, count, cursor))
	})
}

// tweetHistory returns the raw TweetEditHistory result (edit versions). Served raw
// because the edit-history shape is not the standard timeline.
func (s *Server) tweetHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.serveRead(w, r, false, "TweetEditHistory", func(c *xapi.XClient) (any, string, error) {
		return rawByVars(c, "TweetEditHistory", map[string]any{"tweetId": id}, "", 0)
	})
}

// commaVals reads a comma-separated query param into a trimmed, non-empty slice.
func commaVals(r *http.Request, name string) []string {
	var out []string
	for p := range strings.SplitSeq(r.URL.Query().Get(name), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// boolVal reads a truthy query param (true/1/yes/on).
func boolVal(r *http.Request, name string) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get(name))) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

// intVal reads a non-negative integer query param, 0 when absent/invalid.
func intVal(r *http.Request, name string) int {
	n, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(name)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// buildSearchQuery assembles the rawQuery string from q plus the structured filter
// params. It returns an error when no criterion is present, so a search always has
// at least one operator. min_faves accepts both min_likes and min_faves aliases.
func buildSearchQuery(r *http.Request) (string, error) {
	q := r.URL.Query()
	minLikes := intVal(r, "min_likes")
	if minLikes == 0 {
		minLikes = intVal(r, "min_faves")
	}
	f := xapi.SearchFilters{
		Query:            q.Get("q"),
		AllWords:         commaVals(r, "all_words"),
		AnyWords:         commaVals(r, "any_words"),
		ExactPhrases:     commaVals(r, "exact_phrases"),
		ExcludeWords:     commaVals(r, "exclude_words"),
		HashtagsAny:      commaVals(r, "hashtags"),
		HashtagsExclude:  commaVals(r, "exclude_hashtags"),
		FromUsers:        commaVals(r, "from"),
		ToUsers:          commaVals(r, "to"),
		MentioningUsers:  commaVals(r, "mention"),
		Lang:             q.Get("lang"),
		TweetType:        q.Get("tweet_type"),
		VerifiedOnly:     boolVal(r, "verified"),
		BlueVerifiedOnly: boolVal(r, "blue_verified"),
		HasImages:        boolVal(r, "has_images"),
		HasVideos:        boolVal(r, "has_videos"),
		HasLinks:         boolVal(r, "has_links"),
		HasMentions:      boolVal(r, "has_mentions"),
		HasHashtags:      boolVal(r, "has_hashtags"),
		MinLikes:         minLikes,
		MinReplies:       intVal(r, "min_replies"),
		MinRetweets:      intVal(r, "min_retweets"),
		Since:            q.Get("since"),
		Until:            q.Get("until"),
		Place:            q.Get("place"),
		Geocode:          q.Get("geocode"),
		Near:             q.Get("near"),
		Within:           q.Get("within"),
		List:             q.Get("list"),
		QuotedTweetID:    q.Get("quoted_tweet_id"),
		SinceID:          q.Get("since_id"),
		MaxID:            q.Get("max_id"),
	}
	built := xapi.BuildSearchQuery(f)
	if built == "" {
		return "", errNotFound
	}
	return built, nil
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	built, err := buildSearchQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "provide q or at least one search filter (from, since, min_faves, ...)")
		return
	}
	product, count, cursor := r.URL.Query().Get("product"), countParam(r), cursorParam(r)
	s.serveRead(w, r, false, "SearchTimeline", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "SearchTimeline", searchRawVars(built, product), cursor, count)
		}
		return asRead(c.Search(built, product, count, cursor))
	})
}

// searchRawVars builds the SearchTimeline variables for the raw passthrough.
func searchRawVars(q, product string) map[string]any {
	if product == "" {
		product = "Latest"
	}
	return map[string]any{"rawQuery": q, "querySource": "typed_query", "product": product}
}

// searchPeople runs SearchTimeline with product=People, returning users.
func (s *Server) searchPeople(w http.ResponseWriter, r *http.Request) {
	built, err := buildSearchQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "provide q or at least one search filter")
		return
	}
	count, cursor := countParam(r), cursorParam(r)
	s.serveRead(w, r, false, "SearchTimeline", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "SearchTimeline", searchRawVars(built, "People"), cursor, count)
		}
		return asRead(c.SearchUsers(built, count, cursor))
	})
}

// tweetThread returns the tweets in a tweet's conversation (self-thread).
func (s *Server) tweetThread(w http.ResponseWriter, r *http.Request) {
	id, mode := chi.URLParam(r, "id"), sortMode(r)
	s.serveRead(w, r, false, "TweetDetail", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "TweetDetail", threadVars(id, mode), "", 0)
		}
		ts, err := c.TweetThread(id, mode)
		return ts, "", err
	})
}

// tweetReplies returns the direct replies to a tweet.
func (s *Server) tweetReplies(w http.ResponseWriter, r *http.Request) {
	id, mode := chi.URLParam(r, "id"), sortMode(r)
	s.serveRead(w, r, false, "TweetDetail", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "TweetDetail", threadVars(id, mode), "", 0)
		}
		ts, err := c.TweetReplies(id, mode)
		return ts, "", err
	})
}

// threadVars builds the TweetDetail raw-path variables, adding rankingMode when set.
func threadVars(id, mode string) map[string]any {
	v := map[string]any{"focalTweetId": id}
	if mode != "" {
		v["rankingMode"] = mode
	}
	return v
}

func (s *Server) listTweets(w http.ResponseWriter, r *http.Request) {
	id, count, cursor := chi.URLParam(r, "id"), countParam(r), cursorParam(r)
	s.serveRead(w, r, false, "ListLatestTweetsTimeline", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "ListLatestTweetsTimeline", map[string]any{"listId": id}, cursor, count)
		}
		return asRead(c.ListTweets(id, count, cursor))
	})
}

func (s *Server) communityTweets(w http.ResponseWriter, r *http.Request) {
	id, count, cursor := chi.URLParam(r, "id"), countParam(r), cursorParam(r)
	s.serveRead(w, r, false, "CommunityTweetsTimeline", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "CommunityTweetsTimeline", map[string]any{"communityId": id}, cursor, count)
		}
		return asRead(c.CommunityTweets(id, count, cursor))
	})
}

// spaceInfo returns the raw AudioSpaceById result (a Space's metadata, host,
// listeners). The {id} is the Space's base-encoded id, not a numeric id.
func (s *Server) spaceInfo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.serveRead(w, r, false, "AudioSpaceById", func(c *xapi.XClient) (any, string, error) {
		return rawByVars(c, "AudioSpaceById", map[string]any{"id": id}, "", 0)
	})
}

// bookmarkFolders returns the raw BookmarkFoldersSlice result (the account's
// bookmark folders). Account-scoped, so it needs a specific account.
func (s *Server) bookmarkFolders(w http.ResponseWriter, r *http.Request) {
	cursor := cursorParam(r)
	s.serveRead(w, r, true, "BookmarkFoldersSlice", func(c *xapi.XClient) (any, string, error) {
		return rawByVars(c, "BookmarkFoldersSlice", map[string]any{}, cursor, 0)
	})
}

// bookmarkFolderTweets returns the tweets in one bookmark folder. Account-scoped.
func (s *Server) bookmarkFolderTweets(w http.ResponseWriter, r *http.Request) {
	id, count, cursor := chi.URLParam(r, "id"), countParam(r), cursorParam(r)
	s.serveRead(w, r, true, "BookmarkFolderTimeline", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "BookmarkFolderTimeline", map[string]any{"bookmark_collection_id": id}, cursor, count)
		}
		return asRead(c.BookmarkFolderTweets(id, count, cursor))
	})
}

// bookmarks and home are account-scoped: they read the authenticated account's
// own timeline, so rotation is meaningless and a specific account is required.
func (s *Server) bookmarks(w http.ResponseWriter, r *http.Request) {
	count, cursor := countParam(r), cursorParam(r)
	s.serveRead(w, r, true, "Bookmarks", func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, "Bookmarks", map[string]any{}, cursor, count)
		}
		return asRead(c.Bookmarks(count, cursor))
	})
}

// suggestions serves who-to-follow recommendations (account-scoped).
// ?creator_only=true limits them to creators.
func (s *Server) suggestions(w http.ResponseWriter, r *http.Request) {
	count, cursor := countParam(r), cursorParam(r)
	creatorOnly := r.URL.Query().Get("creator_only") == "true"
	s.serveRead(w, r, true, "ConnectTabTimeline", func(c *xapi.XClient) (any, string, error) {
		return asRead(c.Suggestions(creatorOnly, count, cursor))
	})
}

// ownLists serves the authenticated account's own lists (account-scoped).
func (s *Server) ownLists(w http.ResponseWriter, r *http.Request) {
	count, cursor := countParam(r), cursorParam(r)
	s.serveRead(w, r, true, "ListsManagementPageTimeline", func(c *xapi.XClient) (any, string, error) {
		return asRead(c.OwnLists(count, cursor))
	})
}

// analytics serves the account's analytics overview (account-scoped). The time
// window, granularity and metric set come from query params; the response shape
// varies per metric set, so it is returned raw.
func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	granularity := q.Get("granularity")
	if granularity == "" {
		granularity = "Day"
	}
	var metrics []string
	if m := q.Get("metrics"); m != "" {
		metrics = strings.Split(m, ",")
	}
	s.serveRead(w, r, true, "AccountOverviewQuery", func(c *xapi.XClient) (any, string, error) {
		out, err := c.Analytics(q.Get("from_time"), q.Get("to_time"), granularity, metrics, q.Get("verified_followers") == "true")
		return out, "", err
	})
}

// home serves the ranked home timeline, or the chronological one with
// ?chronological=true (aka ?latest=true).
func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	count, cursor := countParam(r), cursorParam(r)
	q := r.URL.Query()
	latest := q.Get("chronological") == "true" || q.Get("latest") == "true"
	op := "HomeTimeline"
	if latest {
		op = "HomeLatestTimeline"
	}
	s.serveRead(w, r, true, op, func(c *xapi.XClient) (any, string, error) {
		if rawParam(r) {
			return rawByVars(c, op, map[string]any{}, cursor, count)
		}
		if latest {
			return asRead(c.HomeLatest(count, cursor))
		}
		return asRead(c.Home(count, cursor))
	})
}
