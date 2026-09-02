package server

import (
	"log"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"x-rest-api/internal/openapi"
	"x-rest-api/internal/store"
	"x-rest-api/internal/xapi"
)

// Server serves the /v1 REST API. The /admin panel is mounted as an external
// handler so both share one port.
type Server struct {
	store      *store.Store
	sess       *xapi.Session
	pool       *Pool
	spec       []byte // cached OpenAPI 3 document, built once in Routes
	refresh    func() (int, error)
	refreshing atomic.Bool
}

// NewServer builds the REST API server.
func NewServer(st *store.Store, sess *xapi.Session) *Server {
	return &Server{store: st, sess: sess, pool: NewPool(st)}
}

// SetRefresh wires the queryId refresh callback, triggered when x.com reports the
// GraphQL features/queryId are stale (error code 336).
func (s *Server) SetRefresh(fn func() (int, error)) { s.refresh = fn }

// triggerRefresh runs the queryId refresh once, skipping if one is in flight, so
// a burst of 336s does not stampede x.com.
func (s *Server) triggerRefresh() {
	if s.refresh == nil || !s.refreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.refreshing.Store(false)
		if n, err := s.refresh(); err != nil {
			log.Printf("queryid refresh (triggered by code 336): %v", err)
		} else {
			log.Printf("queryid refresh (triggered by code 336): %d ids", n)
		}
	}()
}

// apiRoute is one /v1 operation: its OpenAPI metadata plus the chi handler. The
// same slice registers the routes and generates the OpenAPI document, so adding
// a route updates both the API and the docs from one place.
type apiRoute struct {
	openapi.Route
	Handler http.HandlerFunc
}

// countParams documents the shared ?count=/?cursor=/?raw= queries on list endpoints.
var countParams = []openapi.Param{
	{Name: "count", In: "query", Type: "integer", Desc: "Items to return (default 40, max 200)."},
	{Name: "cursor", In: "query", Type: "string", Desc: "Pagination cursor; when set, returns a single page. The response carries next_cursor."},
	{Name: "raw", In: "query", Type: "boolean", Desc: "Return the unparsed GraphQL response instead of the flat model."},
	{Name: "format", In: "query", Type: "string", Desc: "csv to return CSV instead of JSON (list results only; next cursor in X-Next-Cursor header)."},
}

// rawOnlyParams documents ?raw= on single-item (non-paginated) read endpoints.
var rawOnlyParams = []openapi.Param{
	{Name: "raw", In: "query", Type: "boolean", Desc: "Return the unparsed GraphQL response instead of the flat model."},
}

// rssParams documents the ?count= query on RSS feed endpoints (raw/csv/cursor do
// not apply; the feed is always the first page as RSS 2.0 XML).
var rssParams = []openapi.Param{
	{Name: "count", In: "query", Type: "integer", Desc: "Items in the feed (default 40, max 200)."},
}

// sortParams documents the ?sort= reply ranking plus ?raw= on thread/replies.
var sortParams = []openapi.Param{
	{Name: "sort", In: "query", Type: "string", Desc: "Reply ranking: relevance | recency | likes (default relevance)."},
	{Name: "raw", In: "query", Type: "boolean", Desc: "Return the unparsed GraphQL response instead of the flat model."},
}

// idsParams documents the ?ids= query on batch-lookup endpoints.
var idsParams = []openapi.Param{
	{Name: "ids", In: "query", Type: "string", Required: true, Desc: "Comma-separated numeric ids (max 100)."},
	{Name: "raw", In: "query", Type: "boolean", Desc: "Return the unparsed GraphQL response instead of the flat model."},
}

// searchParams documents the structured-filter search query. q or any filter is
// enough; the filters render into x.com's rawQuery operator string. withProduct
// adds the tweets-only product selector.
func searchParams(withProduct bool) []openapi.Param {
	p := []openapi.Param{
		{Name: "q", In: "query", Type: "string", Desc: "Raw query, kept verbatim; combined with the filters below. q or any one filter is required."},
	}
	if withProduct {
		p = append(p, openapi.Param{Name: "product", In: "query", Type: "string", Desc: "Latest | Top | Media | Lists (default Latest)."})
	}
	p = append(p,
		openapi.Param{Name: "all_words", In: "query", Type: "string", Desc: "Comma-separated; all must appear (AND)."},
		openapi.Param{Name: "any_words", In: "query", Type: "string", Desc: "Comma-separated; any may appear (OR)."},
		openapi.Param{Name: "exact_phrases", In: "query", Type: "string", Desc: "Comma-separated exact phrases (quoted)."},
		openapi.Param{Name: "exclude_words", In: "query", Type: "string", Desc: "Comma-separated words to exclude (-term)."},
		openapi.Param{Name: "hashtags", In: "query", Type: "string", Desc: "Comma-separated hashtags (OR); # optional."},
		openapi.Param{Name: "exclude_hashtags", In: "query", Type: "string", Desc: "Comma-separated hashtags to exclude."},
		openapi.Param{Name: "from", In: "query", Type: "string", Desc: "Comma-separated authors (from:)."},
		openapi.Param{Name: "to", In: "query", Type: "string", Desc: "Comma-separated reply targets (to:)."},
		openapi.Param{Name: "mention", In: "query", Type: "string", Desc: "Comma-separated mentioned users (@)."},
		openapi.Param{Name: "lang", In: "query", Type: "string", Desc: "Language code (lang:)."},
		openapi.Param{Name: "tweet_type", In: "query", Type: "string", Desc: "all|originals_only|replies_only|retweets_only|exclude_replies|exclude_retweets."},
		openapi.Param{Name: "verified", In: "query", Type: "boolean", Desc: "filter:verified."},
		openapi.Param{Name: "blue_verified", In: "query", Type: "boolean", Desc: "filter:blue_verified."},
		openapi.Param{Name: "has_images", In: "query", Type: "boolean", Desc: "filter:images."},
		openapi.Param{Name: "has_videos", In: "query", Type: "boolean", Desc: "filter:videos."},
		openapi.Param{Name: "has_links", In: "query", Type: "boolean", Desc: "filter:links."},
		openapi.Param{Name: "has_mentions", In: "query", Type: "boolean", Desc: "filter:mentions."},
		openapi.Param{Name: "has_hashtags", In: "query", Type: "boolean", Desc: "filter:hashtags."},
		openapi.Param{Name: "min_faves", In: "query", Type: "integer", Desc: "min_faves: (alias min_likes)."},
		openapi.Param{Name: "min_replies", In: "query", Type: "integer", Desc: "min_replies:."},
		openapi.Param{Name: "min_retweets", In: "query", Type: "integer", Desc: "min_retweets:."},
		openapi.Param{Name: "since", In: "query", Type: "string", Desc: "since:YYYY-MM-DD."},
		openapi.Param{Name: "until", In: "query", Type: "string", Desc: "until:YYYY-MM-DD."},
		openapi.Param{Name: "place", In: "query", Type: "string", Desc: "place: id."},
		openapi.Param{Name: "geocode", In: "query", Type: "string", Desc: "geocode: lat,long,radius."},
		openapi.Param{Name: "near", In: "query", Type: "string", Desc: "near: location (with within:)."},
		openapi.Param{Name: "within", In: "query", Type: "string", Desc: "within: radius."},
		openapi.Param{Name: "list", In: "query", Type: "string", Desc: "list: id (restrict to a list's members)."},
		openapi.Param{Name: "quoted_tweet_id", In: "query", Type: "string", Desc: "quoted_tweet_id: (tweets quoting this tweet)."},
		openapi.Param{Name: "since_id", In: "query", Type: "string", Desc: "since_id: (tweets after this tweet id)."},
		openapi.Param{Name: "max_id", In: "query", Type: "string", Desc: "max_id: (tweets up to this tweet id)."},
		openapi.Param{Name: "count", In: "query", Type: "integer", Desc: "Items to return (default 40, max 200)."},
		openapi.Param{Name: "cursor", In: "query", Type: "string", Desc: "Pagination cursor; when set, returns a single page."},
		openapi.Param{Name: "raw", In: "query", Type: "boolean", Desc: "Return the unparsed GraphQL response."},
		openapi.Param{Name: "format", In: "query", Type: "string", Desc: "csv to return CSV instead of JSON (next cursor in X-Next-Cursor header)."},
	)
	return p
}

// v1Routes is the single source of truth for the /v1 surface.
func (s *Server) v1Routes() []apiRoute {
	tweets := []xapi.Tweet{}
	writeOK := map[string]string{}
	route := func(m, p, sum string, resp any, params []openapi.Param) apiRoute {
		return apiRoute{Route: openapi.Route{
			Method: m, Path: p, Summary: sum, Tag: "x", Response: resp,
			Params: params, Secured: true,
		}}
	}
	list := func(p, sum string, h http.HandlerFunc) apiRoute {
		r := route("GET", p, sum, tweets, countParams)
		r.Handler = h
		return r
	}

	rs := []apiRoute{
		with(route("GET", "/v1/users/{handle}", "Profile by handle or numeric id", xapi.XUser{}, rawOnlyParams), s.getUser),
		list("/v1/users/{handle}/tweets", "A user's posts", s.readTweets("handle", "UserTweets", (*xapi.XClient).UserTweets)),
		list("/v1/users/{handle}/replies", "A user's posts and replies", s.readTweets("handle", "UserTweetsAndReplies", (*xapi.XClient).UserReplies)),
		list("/v1/users/{handle}/media", "A user's media posts", s.readTweets("handle", "UserMedia", (*xapi.XClient).UserMedia)),
		list("/v1/users/{handle}/highlights", "A user's highlights", s.readTweets("handle", "UserHighlightsTweets", (*xapi.XClient).UserHighlights)),
		list("/v1/users/{handle}/likes", "A user's liked posts", s.readTweets("handle", "Likes", (*xapi.XClient).Likes)),
		usersList("/v1/users/{handle}/followers", "A user's followers", s.readUsers("handle", "Followers", (*xapi.XClient).Followers)),
		usersList("/v1/users/{handle}/following", "Who a user follows", s.readUsers("handle", "Following", (*xapi.XClient).Following)),
		usersList("/v1/users/{handle}/verified-followers", "A user's verified followers", s.readUsers("handle", "BlueVerifiedFollowers", (*xapi.XClient).VerifiedFollowers)),
		usersList("/v1/users/{handle}/subscriptions", "Creators a user subscribes to", s.readUsers("handle", "UserCreatorSubscriptions", (*xapi.XClient).Subscriptions)),
		usersList("/v1/users/{handle}/affiliates", "A user's business-profile affiliates", s.readUsers("handle", "UserBusinessProfileTeamTimeline", (*xapi.XClient).Affiliates)),
		with(route("GET", "/v1/users/{handle}/about", "Account origin, username history, identity verification", xapi.AccountAbout{}, rawOnlyParams), s.userAbout),
		with(route("GET", "/v1/users/{handle}/rss", "A user's posts as an RSS 2.0 feed", tweets, rssParams), s.rssTweets("handle", "UserTweets", (*xapi.XClient).UserTweets)),

		with(route("GET", "/v1/users/by", "Look up many profiles by numeric id", []xapi.XUser{}, idsParams), s.usersByIDs),

		with(route("GET", "/v1/tweets/{id}", "Focal tweet with its reply thread", xapi.TweetThread{}, rawOnlyParams), s.getTweet),
		with(route("GET", "/v1/tweets/{id}/result", "A single tweet without its thread", xapi.Tweet{}, rawOnlyParams), s.getTweetResult),
		with(route("GET", "/v1/tweets/{id}/thread", "Tweets in the conversation (self-thread)", tweets, sortParams), s.tweetThread),
		with(route("GET", "/v1/tweets/{id}/replies", "Direct replies to a tweet", tweets, sortParams), s.tweetReplies),
		with(route("GET", "/v1/tweets/{id}/history", "A tweet's edit history (raw)", map[string]any{}, rawOnlyParams), s.tweetHistory),
		usersList("/v1/tweets/{id}/retweeters", "Users who reposted a tweet", s.getRetweeters),
		usersList("/v1/tweets/{id}/likers", "Users who liked a tweet", s.getLikers),
		with(route("GET", "/v1/tweets/by", "Look up many tweets by numeric id", tweets, idsParams), s.tweetsByIDs),

		with(route("GET", "/v1/search", "Keyword/filter search (tweets)", tweets, searchParams(true)), s.search),
		with(route("GET", "/v1/search/people", "Keyword/filter search (users)", []xapi.XUser{}, searchParams(false)), s.searchPeople),
		with(route("GET", "/v1/search/rss", "Search results as an RSS 2.0 feed", tweets, searchParams(false)), s.searchRSS),
		with(route("GET", "/v1/lists/by-slug", "List metadata by owner handle + slug", xapi.List{}, []openapi.Param{
			{Name: "owner", In: "query", Type: "string", Desc: "List owner's @handle."},
			{Name: "slug", In: "query", Type: "string", Desc: "List slug."},
		}), s.listBySlug),
		with(route("GET", "/v1/lists/{id}", "List metadata", xapi.List{}, rawOnlyParams), s.listInfo),
		list("/v1/lists/{id}/tweets", "List timeline", s.listTweets),
		with(route("GET", "/v1/lists/{id}/rss", "List timeline as an RSS 2.0 feed", tweets, rssParams), s.rssTweets("id", "ListLatestTweetsTimeline", (*xapi.XClient).ListTweets)),
		usersList("/v1/lists/{id}/members", "List members", s.readUsersID("id", "ListMembers", "listId", (*xapi.XClient).ListMembers)),
		writeRoute(route("POST", "/v1/lists", "Create a list", map[string]any{}, nil), createListBody{}, 201, s.createList),
		writeRoute(route("PATCH", "/v1/lists/{id}", "Update a list's name/description/visibility", writeOK, nil), updateListBody{}, 200, s.updateList),
		writeRoute(route("DELETE", "/v1/lists/{id}", "Delete a list", writeOK, nil), nil, 200, s.deleteList),
		writeRoute(route("POST", "/v1/lists/{id}/members", "Add a member to a list", writeOK, nil), listMemberBody{}, 200, s.listAddMember),
		writeRoute(route("DELETE", "/v1/lists/{id}/members/{userId}", "Remove a member from a list", writeOK, nil), nil, 200, s.listRemoveMember),
		writeRoute(route("POST", "/v1/lists/{id}/mute", "Mute a list", writeOK, nil), nil, 200, s.muteList),
		writeRoute(route("DELETE", "/v1/lists/{id}/mute", "Unmute a list", writeOK, nil), nil, 200, s.unmuteList),
		list("/v1/communities/{id}/tweets", "Community timeline", s.communityTweets),
		usersList("/v1/communities/{id}/members", "Community members", s.readUsersID("id", "membersSliceTimeline_Query", "communityId", (*xapi.XClient).CommunityMembers)),
		usersList("/v1/communities/{id}/moderators", "Community moderators", s.readUsersID("id", "moderatorsSliceTimeline_Query", "communityId", (*xapi.XClient).CommunityModerators)),
		with(route("GET", "/v1/communities/{id}", "Community info (raw)", map[string]any{}, rawOnlyParams), s.communityInfo),
		with(route("GET", "/v1/trends", "Trends by category (raw)", map[string]any{}, []openapi.Param{
			{Name: "category", In: "query", Type: "string", Desc: "trending | news | sport | entertainment, or a literal timeline id (default trending)."},
			{Name: "count", In: "query", Type: "integer", Desc: "Items to return (default 40, max 200)."},
			{Name: "cursor", In: "query", Type: "string", Desc: "Pagination cursor."},
		}), s.trends),
		with(route("GET", "/v1/notifications", "Notifications timeline (raw, account-scoped)", map[string]any{}, []openapi.Param{
			{Name: "count", In: "query", Type: "integer", Desc: "Items to return (default 40, max 200)."},
			{Name: "cursor", In: "query", Type: "string", Desc: "Pagination cursor."},
		}), s.notifications),
		with(route("GET", "/v1/spaces/{id}", "Space info by id (raw)", map[string]any{}, rawOnlyParams), s.spaceInfo),
		with(route("GET", "/v1/jobs/search", "Search X Jobs", []xapi.Job{}, []openapi.Param{
			{Name: "keyword", In: "query", Type: "string", Desc: "Search keyword."},
			{Name: "location", In: "query", Type: "string", Desc: "Location name."},
			{Name: "location_id", In: "query", Type: "string", Desc: "Location id (from /v1/jobs/locations)."},
			{Name: "location_type", In: "query", Type: "string", Desc: "e.g. REMOTE."},
			{Name: "seniority", In: "query", Type: "string", Desc: "Seniority level."},
			{Name: "company", In: "query", Type: "string", Desc: "Company name."},
			{Name: "employment_type", In: "query", Type: "string", Desc: "e.g. FULL_TIME."},
			{Name: "industry", In: "query", Type: "string", Desc: "Industry."},
			{Name: "count", In: "query", Type: "integer", Desc: "Items to return (default 25)."},
			{Name: "cursor", In: "query", Type: "string", Desc: "Pagination cursor."},
		}), s.jobSearch),
		with(route("GET", "/v1/jobs/locations", "Job location suggestions", []xapi.JobLocation{}, []openapi.Param{
			{Name: "query", In: "query", Type: "string", Desc: "Location query."},
		}), s.jobLocations),
		with(route("GET", "/v1/jobs/{id}", "Job details by id", xapi.Job{}, nil), s.jobDetails),
		with(route("GET", "/v1/dm/inbox", "Direct message inbox (account-scoped)", xapi.Inbox{}, []openapi.Param{
			{Name: "cursor", In: "query", Type: "string", Desc: "Inbox pagination cursor (min_entry_id from a previous page)."},
		}), s.dmInbox),
		with(route("GET", "/v1/dm/conversations/{id}", "A DM conversation (account-scoped)", xapi.Conversation{}, []openapi.Param{
			{Name: "cursor", In: "query", Type: "string", Desc: "Load older history (oldest message id from a previous page)."},
		}), s.dmConversation),
		writeRoute(route("DELETE", "/v1/dm/conversations/{id}", "Delete (leave) a DM conversation", writeOK, nil), nil, 200, s.deleteConversation),
		writeRoute(route("POST", "/v1/dm/conversations/{id}/messages", "Send a direct message", xapi.DirectMessage{}, nil), nil, 201, s.sendDM),
		with(route("GET", "/v1/bookmarks/folders", "Bookmark folders (raw, account-scoped)", map[string]any{}, []openapi.Param{
			{Name: "cursor", In: "query", Type: "string", Desc: "Pagination cursor."},
		}), s.bookmarkFolders),
		list("/v1/bookmarks/folders/{id}", "Tweets in a bookmark folder (account-scoped)", s.bookmarkFolderTweets),
		list("/v1/bookmarks", "Bookmarks (account-scoped)", s.bookmarks),
		with(route("GET", "/v1/home", "Home feed (account-scoped)", tweets, []openapi.Param{
			{Name: "count", In: "query", Type: "integer", Desc: "Items to return (default 40, max 200)."},
			{Name: "chronological", In: "query", Type: "boolean", Desc: "true for the Following (latest) feed."},
			{Name: "latest", In: "query", Type: "boolean", Desc: "Alias of chronological."},
			{Name: "cursor", In: "query", Type: "string", Desc: "Pagination cursor; when set, returns a single page."},
			{Name: "raw", In: "query", Type: "boolean", Desc: "Return the unparsed GraphQL response."},
		}), s.home),
		with(route("GET", "/v1/suggestions", "Who-to-follow recommendations (account-scoped)", []xapi.XUser{}, []openapi.Param{
			{Name: "creator_only", In: "query", Type: "boolean", Desc: "Limit to creators."},
			{Name: "count", In: "query", Type: "integer", Desc: "Items to return (default 40, max 200)."},
			{Name: "cursor", In: "query", Type: "string", Desc: "Pagination cursor."},
		}), s.suggestions),
		with(route("GET", "/v1/lists", "Your own lists (account-scoped)", []xapi.List{}, []openapi.Param{
			{Name: "count", In: "query", Type: "integer", Desc: "Items to return (default 40, max 200)."},
			{Name: "cursor", In: "query", Type: "string", Desc: "Pagination cursor."},
		}), s.ownLists),
		with(route("GET", "/v1/analytics", "Account analytics overview (account-scoped)", map[string]any{}, []openapi.Param{
			{Name: "from_time", In: "query", Type: "string", Desc: "Window start (ISO or epoch)."},
			{Name: "to_time", In: "query", Type: "string", Desc: "Window end (ISO or epoch)."},
			{Name: "granularity", In: "query", Type: "string", Desc: "Day | Hour | Total (default Day)."},
			{Name: "metrics", In: "query", Type: "string", Desc: "Comma-separated metric names."},
			{Name: "verified_followers", In: "query", Type: "boolean", Desc: "Include verified-follower breakdown."},
		}), s.analytics),

		writeRoute(route("POST", "/v1/media", "Upload media (multipart field 'file'); returns media_id", map[string]any{}, nil), nil, 201, s.uploadMedia),
		writeRoute(route("POST", "/v1/tweets", "Post a tweet, reply, or quote (optional media_ids)", xapi.Tweet{}, nil), createTweetBody{}, 201, s.createTweet),
		writeRoute(route("POST", "/v1/notes", "Post a long-form (note) tweet or reply (requires X Premium)", xapi.Tweet{}, nil), createTweetBody{}, 201, s.createNote),
		writeRoute(route("DELETE", "/v1/tweets/{id}", "Delete a tweet", writeOK, nil), nil, 200, s.deleteTweet),
		writeRoute(route("POST", "/v1/tweets/{id}/like", "Like a tweet", writeOK, nil), nil, 200, s.likeTweet),
		writeRoute(route("POST", "/v1/tweets/{id}/unlike", "Remove a like", writeOK, nil), nil, 200, s.unlikeTweet),
		writeRoute(route("POST", "/v1/tweets/{id}/retweet", "Repost a tweet", writeOK, nil), nil, 201, s.retweet),
		writeRoute(route("DELETE", "/v1/tweets/{id}/retweet", "Remove a repost", writeOK, nil), nil, 200, s.unretweet),
		writeRoute(route("POST", "/v1/tweets/{id}/bookmark", "Bookmark a tweet", writeOK, nil), nil, 201, s.createBookmark),
		writeRoute(route("DELETE", "/v1/tweets/{id}/bookmark", "Remove a bookmark", writeOK, nil), nil, 200, s.deleteBookmark),
		writeRoute(route("POST", "/v1/users/{handle}/follow", "Follow a user", writeOK, nil), nil, 200, s.followUser),
		writeRoute(route("DELETE", "/v1/users/{handle}/follow", "Unfollow a user", writeOK, nil), nil, 200, s.unfollowUser),
		writeRoute(route("DELETE", "/v1/users/{handle}/follower", "Remove a follower", writeOK, nil), nil, 200, s.removeFollower),
		writeRoute(route("POST", "/v1/users/{handle}/mute", "Mute a user", writeOK, nil), nil, 200, s.muteUser),
		writeRoute(route("DELETE", "/v1/users/{handle}/mute", "Unmute a user", writeOK, nil), nil, 200, s.unmuteUser),
		writeRoute(route("POST", "/v1/account/username", "Change your @username", writeOK, nil), usernameBody{}, 200, s.changeUsername),
		writeRoute(route("PATCH", "/v1/account/profile", "Update your profile (name/url/location/description)", writeOK, nil), updateProfileBody{}, 200, s.updateProfile),
		writeRoute(route("PUT", "/v1/account/profile/image", "Set your avatar from base64", writeOK, nil), imageBody{}, 200, s.updateProfileImage),
		writeRoute(route("PUT", "/v1/account/profile/banner", "Set your banner from base64", writeOK, nil), bannerBody{}, 200, s.updateProfileBanner),
		writeRoute(route("POST", "/v1/account/password", "Change your password (may rotate the session)", writeOK, nil), passwordBody{}, 200, s.changePassword),
		with(route("GET", "/v1/scheduled", "Your scheduled (unsent) tweets (raw, account-scoped)", map[string]any{}, nil), s.getScheduled),
		writeRoute(route("POST", "/v1/scheduled", "Schedule a tweet for a future time", map[string]any{}, nil), scheduleTweetBody{}, 201, s.scheduleTweet),
		writeRoute(route("DELETE", "/v1/scheduled/{id}", "Cancel a scheduled tweet", writeOK, nil), nil, 200, s.deleteScheduled),
	}
	return rs
}

// with attaches a handler to a route.
func with(r apiRoute, h http.HandlerFunc) apiRoute {
	r.Handler = h
	return r
}

// usersList builds a GET route whose response is a user list with ?count=.
func usersList(p, sum string, h http.HandlerFunc) apiRoute {
	return apiRoute{Route: openapi.Route{
		Method: "GET", Path: p, Summary: sum, Tag: "x", Response: []xapi.XUser{},
		Params: countParams, Secured: true,
	}, Handler: h}
}

// writeRoute sets a write route's request body and success status.
func writeRoute(r apiRoute, body any, status int, h http.HandlerFunc) apiRoute {
	r.RequestBody = body
	r.Status = status
	r.Handler = h
	return r
}

// Routes builds the root handler: /health, /openapi.json and /docs open, /v1
// behind the API key with request logging, and the admin panel mounted at /admin.
func (s *Server) Routes(admin http.Handler) http.Handler {
	routes := s.v1Routes()
	s.buildSpec(routes)

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Logger, middleware.Recoverer)

	r.Get("/health", s.health)
	r.Get("/openapi.json", s.openapiJSON)
	r.Get("/docs", s.docsUI)
	r.Handle("/docs-static/*", http.StripPrefix("/docs-static/", docsStatic()))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusFound)
	})

	r.Mount("/admin", admin)

	r.Route("/v1", func(v chi.Router) {
		v.Use(s.requestLog)
		v.Use(s.apiKeyAuth)
		for _, rt := range routes {
			v.Method(rt.Method, strings.TrimPrefix(rt.Path, "/v1"), rt.Handler)
		}
	})
	return r
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
