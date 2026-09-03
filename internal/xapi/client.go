package xapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/url"
	"strconv"

	http "github.com/bogdanfinn/fhttp"
)

// parseRateLimit extracts x-rate-limit-* headers, or nil when absent.
func parseRateLimit(h http.Header) *RateLimit {
	rem := h.Get("x-rate-limit-remaining")
	reset := h.Get("x-rate-limit-reset")
	if rem == "" && reset == "" {
		return nil
	}
	toInt := func(s string) int { n, _ := strconv.Atoi(s); return n }
	return &RateLimit{
		Limit:     toInt(h.Get("x-rate-limit-limit")),
		Remaining: toInt(rem),
		Reset:     int64(toInt(reset)),
	}
}

const (
	baseURL  = "https://x.com/i/api/graphql"
	pageSize = 100 // x.com caps most timelines around 100/page
)

// txRequired lists ops x.com refuses (404) without a valid x-client-transaction-id.
// Most ops ignore it; search is the hardened one. Keep this tight, because a
// needless tx header only adds a failure mode.
var txRequired = map[string]bool{"SearchTimeline": true}

// XClient is a client over x.com's private GraphQL API, bound to one account and
// the shared transport. Build a fresh one per request with NewClientFor.
type XClient struct {
	sess      *Session
	acct      Account
	rateLimit *RateLimit
}

// RateLimit holds the x-rate-limit-* headers from the last upstream response.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     int64 // unix seconds
}

// NewClientFor binds the shared session to an account.
func NewClientFor(sess *Session, acct Account) *XClient {
	return &XClient{sess: sess, acct: acct}
}

// RateLimit returns the rate-limit info from the last call, or nil.
func (c *XClient) RateLimit() *RateLimit { return c.rateLimit }

// TweetThread is the focal tweet of a TweetDetail plus its reply thread.
type TweetThread struct {
	Tweet   *Tweet  `json:"tweet"`
	Replies []Tweet `json:"replies"`
}

// call fills an op template with the dynamic variables and replays the request.
func (c *XClient) call(op string, variables map[string]any) (map[string]any, error) {
	s, err := spec(op)
	if err != nil {
		return nil, err
	}
	// Apply any runtime queryId override (bundle auto-refresh) over the embedded one.
	s.QueryID = c.sess.queryID(op, s.QueryID)
	// Add any feature flag the live bundle lists for op but ops.json omits.
	s.Features = c.sess.featuresFor(op, s.Features)

	v := map[string]any{}
	maps.Copy(v, s.Variables)
	maps.Copy(v, variables)

	req, err := c.buildRequest(s, op, v)
	if err != nil {
		return nil, err
	}

	// x.com attaches x-client-transaction-id to every request. Some ops (search,
	// bookmark mutations) hard-reject its absence with 404, while others tolerate a
	// missing one. Generate it for every call; a generation failure only aborts the
	// ops that strictly require it, so tolerant reads keep working.
	path := fmt.Sprintf("/i/api/graphql/%s/%s", s.QueryID, op)
	txValue, txErr := c.sess.transactionID(s.Method, path)
	if (txErr != nil || txValue == "") && txRequired[op] {
		if txErr != nil {
			return nil, fmt.Errorf("%s: %w", op, txErr)
		}
		return nil, &TxRequiredError{Op: op}
	}
	req.Header = c.sess.headers(c.acct, "en", txValue)

	resp, err := c.sess.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()
	c.rateLimit = parseRateLimit(resp.Header)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read body: %w", op, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, msg := parseXErrors(body)
		return nil, &UpstreamError{
			Op: op, Status: resp.StatusCode, Body: truncate(body, 300),
			Code: code, Msg: msg, HTML: isHTMLBlock(resp.Header, body),
		}
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("%s: decode json: %w", op, err)
	}
	return out, nil
}

// callForm sends a form-urlencoded POST to a legacy REST 1.1/2.0 endpoint (not
// GraphQL), reusing the session's auth headers but overriding content-type. op is
// a label used only for error/rate-limit bookkeeping.
func (c *XClient) callForm(op, apiURL string, form url.Values) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader([]byte(form.Encode())))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	req.Header = c.sess.headers(c.acct, "en", "")
	// Override in place with the lowercase key headers() uses; Header.Set would add
	// a second canonical "Content-Type" that x.com may read instead.
	req.Header["content-type"] = []string{"application/x-www-form-urlencoded"}
	return c.doForm(op, req)
}

// callFormGet sends a GET to a legacy REST 1.1 endpoint with query params (DM
// reads), reusing the session auth headers. It is the GET counterpart of callForm.
func (c *XClient) callFormGet(op, apiURL string, params url.Values) (map[string]any, error) {
	u := apiURL
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	req.Header = c.sess.headers(c.acct, "en", "")
	return c.doForm(op, req)
}

// doRaw sends a prepared REST request and returns the raw response body, sharing
// rate-limit and upstream-error handling across the form-POST, query-GET, and
// json-POST paths. Callers decode the body themselves.
func (c *XClient) doRaw(op string, req *http.Request) ([]byte, error) {
	resp, err := c.sess.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer resp.Body.Close()
	c.rateLimit = parseRateLimit(resp.Header)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read body: %w", op, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, msg := parseXErrors(body)
		return nil, &UpstreamError{
			Op: op, Status: resp.StatusCode, Body: truncate(body, 300),
			Code: code, Msg: msg, HTML: isHTMLBlock(resp.Header, body),
		}
	}
	return body, nil
}

// doForm sends a prepared REST request and decodes the JSON response. An empty
// body decodes to a nil map (some deletes return no content).
func (c *XClient) doForm(op string, req *http.Request) (map[string]any, error) {
	body, err := c.doRaw(op, req)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("%s: decode json: %w", op, err)
		}
	}
	return out, nil
}

// callJSONRaw POSTs a JSON payload to a REST endpoint and returns the raw body,
// reusing the session auth headers with the given content-type. Used by endpoints
// whose payload is a JSON object (DM send) or whose reply is newline-delimited
// JSON chunks (Grok). A non-empty txPath makes it attach x-client-transaction-id
// derived from that path, which the grok.x.com API rejects a request without.
func (c *XClient) callJSONRaw(op, apiURL, txPath, contentType string, payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal payload: %w", op, err)
	}
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	tx := ""
	if txPath != "" {
		if tx, err = c.sess.transactionID(http.MethodPost, txPath); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
	}
	req.Header = c.sess.headers(c.acct, "en", tx)
	req.Header["content-type"] = []string{contentType}
	return c.doRaw(op, req)
}

// callJSON is callJSONRaw (application/json, no transaction-id) with a single
// JSON-object reply decoded to a map.
func (c *XClient) callJSON(op, apiURL string, payload any) (map[string]any, error) {
	body, err := c.callJSONRaw(op, apiURL, "", "application/json", payload)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("%s: decode json: %w", op, err)
		}
	}
	return out, nil
}

// buildRequest constructs the GET (query string) or POST (json body) request for
// an op, encoding variables and features as compact JSON.
func (c *XClient) buildRequest(s OpSpec, op string, v map[string]any) (*http.Request, error) {
	varsJSON, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal variables: %w", op, err)
	}
	featsJSON, err := json.Marshal(s.Features)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal features: %w", op, err)
	}

	if s.Method == "POST" {
		payload, err := json.Marshal(map[string]any{
			"variables": v, "features": s.Features, "queryId": s.QueryID,
		})
		if err != nil {
			return nil, fmt.Errorf("%s: marshal body: %w", op, err)
		}
		u := fmt.Sprintf("%s/%s/%s", baseURL, s.QueryID, op)
		return http.NewRequest(http.MethodPost, u, bytes.NewReader(payload))
	}

	qs := "variables=" + url.QueryEscape(string(varsJSON)) +
		"&features=" + url.QueryEscape(string(featsJSON))
	u := fmt.Sprintf("%s/%s/%s?%s", baseURL, s.QueryID, op, qs)
	return http.NewRequest(http.MethodGet, u, nil)
}

// paginate follows the bottom cursor until it has count records (or runs dry),
// then dedups by rest_id preserving order (cursors overlap by one). It starts
// from startCursor and returns the trailing cursor. When startCursor is non-empty
// it fetches a SINGLE page, so callers can step pagination manually.
func paginate[T any](
	c *XClient, op string, variables map[string]any,
	parse func(map[string]any) ([]T, string), key func(T) string, count int, startCursor string,
) ([]T, string, error) {
	var out []T
	cur := startCursor
	single := startCursor != ""
	empty := 0
	for len(out) < count && empty < 2 {
		v := map[string]any{"count": minInt(count, pageSize)}
		maps.Copy(v, variables)
		if cur != "" {
			v["cursor"] = cur
		}
		payload, err := c.call(op, v)
		if err != nil {
			return nil, "", err
		}
		var recs []T
		recs, cur = parse(payload)
		if len(recs) == 0 {
			empty++
		} else {
			empty = 0
			out = append(out, recs...)
		}
		if cur == "" || single {
			break
		}
	}
	return dedup(out, key, count), cur, nil
}

// dedup removes repeated records by key, preserving order, and trims to count.
func dedup[T any](in []T, key func(T) string, count int) []T {
	seen := map[string]bool{}
	out := make([]T, 0, len(in))
	for _, x := range in {
		k := key(x)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, x)
	}
	if len(out) > count {
		out = out[:count]
	}
	return out
}

func tweetKey(t Tweet) string { return t.RestID }
func userKey(u XUser) string  { return u.RestID }

// ------------------------------- users ---------------------------------- //

// GetUser fetches a profile by @handle.
func (c *XClient) GetUser(handle string) (*XUser, error) {
	payload, err := c.call("UserByScreenName", map[string]any{"screen_name": normalizeHandle(handle)})
	if err != nil {
		return nil, err
	}
	return parseUserByScreenName(payload), nil
}

// resolveUID returns the numeric id for a handle or id, resolving handles through
// GetUser first.
func (c *XClient) resolveUID(handleOrID string) (string, error) {
	s := normalizeHandle(handleOrID)
	if isDigits(s) {
		return s, nil
	}
	u, err := c.GetUser(s)
	if err != nil {
		return "", err
	}
	if u == nil || u.RestID == "" {
		return "", fmt.Errorf("user %q not found", handleOrID)
	}
	return u.RestID, nil
}

// Follow follows a user by @handle or numeric id (REST 1.1 friendships/create).
func (c *XClient) Follow(handleOrID string) error {
	uid, err := c.resolveUID(handleOrID)
	if err != nil {
		return err
	}
	_, err = c.callForm("FollowUser", "https://x.com/i/api/1.1/friendships/create.json", url.Values{"user_id": {uid}})
	return err
}

// Unfollow unfollows a user by @handle or numeric id (REST 1.1 friendships/destroy).
func (c *XClient) Unfollow(handleOrID string) error {
	uid, err := c.resolveUID(handleOrID)
	if err != nil {
		return err
	}
	_, err = c.callForm("UnfollowUser", "https://x.com/i/api/1.1/friendships/destroy.json", url.Values{"user_id": {uid}})
	return err
}

// Mute mutes a user by @handle or numeric id (REST 1.1 mutes/users/create). A
// mute hides the target's posts from the account without unfollowing them.
func (c *XClient) Mute(handleOrID string) error {
	uid, err := c.resolveUID(handleOrID)
	if err != nil {
		return err
	}
	_, err = c.callForm("MuteUser", "https://x.com/i/api/1.1/mutes/users/create.json", url.Values{"user_id": {uid}})
	return err
}

// Unmute reverses Mute by @handle or numeric id (REST 1.1 mutes/users/destroy).
func (c *XClient) Unmute(handleOrID string) error {
	uid, err := c.resolveUID(handleOrID)
	if err != nil {
		return err
	}
	_, err = c.callForm("UnmuteUser", "https://x.com/i/api/1.1/mutes/users/destroy.json", url.Values{"user_id": {uid}})
	return err
}

// Block blocks a user by @handle or numeric id (REST 1.1 blocks/create). A block
// hides the target from the account and removes any mutual follow.
func (c *XClient) Block(handleOrID string) error {
	uid, err := c.resolveUID(handleOrID)
	if err != nil {
		return err
	}
	_, err = c.callForm("BlockUser", "https://x.com/i/api/1.1/blocks/create.json", url.Values{"user_id": {uid}})
	return err
}

// Unblock reverses Block by @handle or numeric id (REST 1.1 blocks/destroy).
func (c *XClient) Unblock(handleOrID string) error {
	uid, err := c.resolveUID(handleOrID)
	if err != nil {
		return err
	}
	_, err = c.callForm("UnblockUser", "https://x.com/i/api/1.1/blocks/destroy.json", url.Values{"user_id": {uid}})
	return err
}

// BlockedAccounts lists the authenticated account's blocked users
// (BlockedAccountsAll). It is account-scoped, so it takes no target id.
func (c *XClient) BlockedAccounts(count int, cursor string) ([]XUser, string, error) {
	return paginate(c, "BlockedAccountsAll", map[string]any{}, parseTimelineUsers, userKey, count, cursor)
}

// Me returns the authenticated account's own profile (Viewer). It is
// account-scoped.
func (c *XClient) Me() (*XUser, error) {
	payload, err := c.call("Viewer", map[string]any{})
	if err != nil {
		return nil, err
	}
	u := parseUserResult(asMap(dig(payload, "data", "viewer", "user_results", "result")))
	if u == nil {
		return nil, fmt.Errorf("Me: %s", responseErr(payload))
	}
	return u, nil
}

// ---------------------------- tweet timelines --------------------------- //

func (c *XClient) userTimeline(op, handle string, count int, cursor string) ([]Tweet, string, error) {
	uid, err := c.resolveUID(handle)
	if err != nil {
		return nil, "", err
	}
	return paginate(c, op, map[string]any{"userId": uid}, parseTimelineTweets, tweetKey, count, cursor)
}

func (c *XClient) UserTweets(handle string, count int, cursor string) ([]Tweet, string, error) {
	return c.userTimeline("UserTweets", handle, count, cursor)
}

func (c *XClient) UserReplies(handle string, count int, cursor string) ([]Tweet, string, error) {
	return c.userTimeline("UserTweetsAndReplies", handle, count, cursor)
}

// UserRepliesOnly returns only a user's replies (the "with_replies" tab's
// replies-only variant), without their standalone posts.
func (c *XClient) UserRepliesOnly(handle string, count int, cursor string) ([]Tweet, string, error) {
	return c.userTimeline("UserRepliesTimeline", handle, count, cursor)
}

// UserReposts returns the tweets a user has reposted (their "reposts" tab).
func (c *XClient) UserReposts(handle string, count int, cursor string) ([]Tweet, string, error) {
	return c.userTimeline("UserRepostsTimeline", handle, count, cursor)
}

func (c *XClient) UserMedia(handle string, count int, cursor string) ([]Tweet, string, error) {
	return c.userTimeline("UserMedia", handle, count, cursor)
}

func (c *XClient) UserHighlights(handle string, count int, cursor string) ([]Tweet, string, error) {
	return c.userTimeline("UserHighlightsTweets", handle, count, cursor)
}

// Likes returns a user's liked tweets.
func (c *XClient) Likes(handle string, count int, cursor string) ([]Tweet, string, error) {
	return c.userTimeline("Likes", handle, count, cursor)
}

// HomeLatest returns the chronological ("Following") home timeline.
func (c *XClient) HomeLatest(count int, cursor string) ([]Tweet, string, error) {
	return paginate(c, "HomeLatestTimeline", map[string]any{}, parseTimelineTweets, tweetKey, count, cursor)
}

// GetUserByID fetches a profile by numeric id (UserByRestId).
func (c *XClient) GetUserByID(id string) (*XUser, error) {
	payload, err := c.call("UserByRestId", map[string]any{"userId": id})
	if err != nil {
		return nil, err
	}
	return parseUserByScreenName(payload), nil
}

// searchVars builds the SearchTimeline variables for a product.
func searchVars(query, product string) map[string]any {
	if product == "" {
		product = "Latest"
	}
	return map[string]any{"rawQuery": query, "querySource": "typed_query", "product": product}
}

// Search runs SearchTimeline for tweets. product: Latest | Top | Media | Lists.
func (c *XClient) Search(query, product string, count int, cursor string) ([]Tweet, string, error) {
	return paginate(c, "SearchTimeline", searchVars(query, product), parseTimelineTweets, tweetKey, count, cursor)
}

// SearchUsers runs SearchTimeline with product=People, returning users.
func (c *XClient) SearchUsers(query string, count int, cursor string) ([]XUser, string, error) {
	return paginate(c, "SearchTimeline", searchVars(query, "People"), parseTimelineUsers, userKey, count, cursor)
}

func (c *XClient) ListTweets(listID string, count int, cursor string) ([]Tweet, string, error) {
	return paginate(c, "ListLatestTweetsTimeline", map[string]any{"listId": listID},
		parseTimelineTweets, tweetKey, count, cursor)
}

// ModeratedReplies returns the hidden replies under a root tweet, keyed by
// rootTweetId. It is the read companion of HideReply/UnhideReply.
func (c *XClient) ModeratedReplies(rootTweetID string, count int, cursor string) ([]Tweet, string, error) {
	return paginate(c, "ModeratedTimeline", map[string]any{"rootTweetId": rootTweetID},
		parseTimelineTweets, tweetKey, count, cursor)
}

func (c *XClient) CommunityTweets(communityID string, count int, cursor string) ([]Tweet, string, error) {
	return paginate(c, "CommunityTweetsTimeline", map[string]any{"communityId": communityID},
		parseTimelineTweets, tweetKey, count, cursor)
}

func (c *XClient) Bookmarks(count int, cursor string) ([]Tweet, string, error) {
	return paginate(c, "Bookmarks", map[string]any{}, parseTimelineTweets, tweetKey, count, cursor)
}

// BookmarkFolderTweets returns the tweets in one bookmark folder, keyed by
// bookmark_collection_id.
func (c *XClient) BookmarkFolderTweets(folderID string, count int, cursor string) ([]Tweet, string, error) {
	return paginate(c, "BookmarkFolderTimeline", map[string]any{"bookmark_collection_id": folderID},
		parseTimelineTweets, tweetKey, count, cursor)
}

// Home returns the ranked home feed.
func (c *XClient) Home(count int, cursor string) ([]Tweet, string, error) {
	return paginate(c, "HomeTimeline", map[string]any{}, parseTimelineTweets, tweetKey, count, cursor)
}

// -------------------- single tweet + engagement ------------------------- //

// GetTweet returns the focal tweet of a TweetDetail plus its reply thread.
func (c *XClient) GetTweet(tweetID string) (*TweetThread, error) {
	payload, err := c.call("TweetDetail", map[string]any{"focalTweetId": tweetID})
	if err != nil {
		return nil, err
	}
	tweets, _ := parseTimelineTweets(payload)
	thread := &TweetThread{Replies: []Tweet{}}
	for i := range tweets {
		if tweets[i].RestID == tweetID {
			t := tweets[i]
			thread.Tweet = &t
		} else {
			thread.Replies = append(thread.Replies, tweets[i])
		}
	}
	if thread.Tweet == nil && len(tweets) > 0 {
		thread.Tweet = &tweets[0]
	}
	return thread, nil
}

// GetTweetResult returns a single tweet by id (TweetResultByRestId), without its
// reply thread.
func (c *XClient) GetTweetResult(tweetID string) (*Tweet, error) {
	payload, err := c.call("TweetResultByRestId", map[string]any{"tweetId": tweetID})
	if err != nil {
		return nil, err
	}
	return parseTweet(asMap(dig(payload, "data", "tweetResult", "result"))), nil
}

// TweetReplies returns the direct replies to a tweet (in_reply_to == tweetID),
// from one TweetDetail page. mode is the reply ranking (Relevance/Recency/Likes),
// empty for the op default.
func (c *XClient) TweetReplies(tweetID, mode string) ([]Tweet, error) {
	all, err := c.tweetDetailTweets(tweetID, mode)
	if err != nil {
		return nil, err
	}
	var out []Tweet
	for _, t := range all {
		if t.InReplyToTweetID == tweetID {
			out = append(out, t)
		}
	}
	return out, nil
}

// TweetThread returns the tweets in the same conversation as tweetID (its
// self-thread), from one TweetDetail page.
func (c *XClient) TweetThread(tweetID, mode string) ([]Tweet, error) {
	all, err := c.tweetDetailTweets(tweetID, mode)
	if err != nil {
		return nil, err
	}
	var out []Tweet
	for _, t := range all {
		if t.ConversationID == tweetID {
			out = append(out, t)
		}
	}
	return out, nil
}

// tweetDetailTweets fetches and parses one TweetDetail page. A non-empty mode sets
// the reply rankingMode.
func (c *XClient) tweetDetailTweets(tweetID, mode string) ([]Tweet, error) {
	vars := map[string]any{"focalTweetId": tweetID}
	if mode != "" {
		vars["rankingMode"] = mode
	}
	payload, err := c.call("TweetDetail", vars)
	if err != nil {
		return nil, err
	}
	tweets, _ := parseTimelineTweets(payload)
	return tweets, nil
}

// Retweeters returns the users who reposted a tweet.
func (c *XClient) Retweeters(tweetID string, count int, cursor string) ([]XUser, string, error) {
	return paginate(c, "Retweeters", map[string]any{"tweetId": tweetID},
		parseTimelineUsers, userKey, count, cursor)
}

// ----------------------------- social graph ----------------------------- //

// userGraph paginates a user-list op keyed by userId (followers/following/...).
func (c *XClient) userGraph(op, handle string, count int, cursor string) ([]XUser, string, error) {
	uid, err := c.resolveUID(handle)
	if err != nil {
		return nil, "", err
	}
	return paginate(c, op, map[string]any{"userId": uid}, parseTimelineUsers, userKey, count, cursor)
}

func (c *XClient) Followers(handle string, count int, cursor string) ([]XUser, string, error) {
	return c.userGraph("Followers", handle, count, cursor)
}

func (c *XClient) Following(handle string, count int, cursor string) ([]XUser, string, error) {
	return c.userGraph("Following", handle, count, cursor)
}

func (c *XClient) VerifiedFollowers(handle string, count int, cursor string) ([]XUser, string, error) {
	return c.userGraph("BlueVerifiedFollowers", handle, count, cursor)
}

func (c *XClient) Subscriptions(handle string, count int, cursor string) ([]XUser, string, error) {
	return c.userGraph("UserCreatorSubscriptions", handle, count, cursor)
}

// Affiliates returns a user's business-profile team members. The ops.json entry
// carries the fixed teamName/withVoice variables; only userId is dynamic.
func (c *XClient) Affiliates(handle string, count int, cursor string) ([]XUser, string, error) {
	return c.userGraph("UserBusinessProfileTeamTimeline", handle, count, cursor)
}

// Suggestions returns the account's "who to follow" recommendations. creatorOnly
// limits them to creators; the context variable is itself a JSON-encoded string.
func (c *XClient) Suggestions(creatorOnly bool, count int, cursor string) ([]XUser, string, error) {
	ctx := "{}"
	if creatorOnly {
		ctx = `{"isCreatorOnlyConnectTab":true}`
	}
	return paginate(c, "ConnectTabTimeline", map[string]any{"context": ctx}, parseTimelineUsers, userKey, count, cursor)
}

// OwnLists returns the authenticated account's own lists (account-scoped).
func (c *XClient) OwnLists(count int, cursor string) ([]List, string, error) {
	return paginate(c, "ListsManagementPageTimeline", map[string]any{}, parseListsTimeline, listKey, count, cursor)
}

// Analytics returns the account's raw analytics overview for a time window.
// Metrics and granularity are caller-supplied; the shape varies per metric set,
// so this returns the decoded response as-is.
func (c *XClient) Analytics(fromTime, toTime, granularity string, metrics []string, verifiedFollowers bool) (map[string]any, error) {
	return c.call("AccountOverviewQuery", map[string]any{
		"from_time":               fromTime,
		"to_time":                 toTime,
		"granularity":             granularity,
		"requested_metrics":       metrics,
		"show_verified_followers": verifiedFollowers,
	})
}

// ListMembers returns the members of a list (keyed by listId, not a handle).
func (c *XClient) ListMembers(listID string, count int, cursor string) ([]XUser, string, error) {
	return paginate(c, "ListMembers", map[string]any{"listId": listID}, parseTimelineUsers, userKey, count, cursor)
}

// CommunityMembers returns a community's members.
func (c *XClient) CommunityMembers(communityID string, count int, cursor string) ([]XUser, string, error) {
	return paginate(c, "membersSliceTimeline_Query", map[string]any{"communityId": communityID},
		parseTimelineUsers, userKey, count, cursor)
}

// CommunityModerators returns a community's moderators.
func (c *XClient) CommunityModerators(communityID string, count int, cursor string) ([]XUser, string, error) {
	return paginate(c, "moderatorsSliceTimeline_Query", map[string]any{"communityId": communityID},
		parseTimelineUsers, userKey, count, cursor)
}

// ResolveUID exposes handle/id resolution for the raw request path.
func (c *XClient) ResolveUID(handleOrID string) (string, error) {
	return c.resolveUID(handleOrID)
}

// CallRaw runs one op with the given variables (plus count) and returns the raw
// decoded GQL response, for the ?raw=true passthrough path. A non-empty cursor
// pages forward. It does not parse or paginate.
func (c *XClient) CallRaw(op string, vars map[string]any, cursor string, count int) (map[string]any, error) {
	v := map[string]any{}
	maps.Copy(v, vars)
	if count > 0 {
		v["count"] = minInt(count, pageSize)
	}
	if cursor != "" {
		v["cursor"] = cursor
	}
	return c.call(op, v)
}
