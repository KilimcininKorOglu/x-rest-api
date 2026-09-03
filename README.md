# x-rest-api

Read/write HTTP REST API over x.com (Twitter)'s private GraphQL surface, in Go,
with an htmx dark-mode admin panel. Multi-account cookie rotation, API-key
management, and full request logging. Everything shares one port: `/v1` is the
native API, `/2` mirrors the official X API v2 request/response shape, and
`/admin` is the panel.

`/2` renders the official `{data, includes, meta, errors}` envelope with full
`tweet.fields`/`user.fields`/`expansions` selection, so a client written for
`api.x.com` reaches this server with only a base-URL change. See
[X API v2 compatible layer](#x-api-v2-compatible-layer-2).

Only the listen port comes from the environment. Every other setting (accounts,
API keys, proxy, writes, retention) lives in SQLite and is managed from `/admin`.

Upstream requests are replayed with [`tls-client`](https://github.com/bogdanfinn/tls-client)
impersonating Chrome, so the TLS/HTTP2 fingerprint matches a browser. The
`x-client-transaction-id` header is generated at runtime, so `search` works
without a captured token.

## Quick start

```bash
make build
PORT=8430 ./bin/x-rest-api
```

Then open `http://localhost:8430/admin`:

1. First visit shows a setup wizard: create the admin account.
2. Add one or more X.com accounts (the `auth_token` and `ct0` cookies from
   DevTools -> Application -> Cookies -> x.com).
3. Create an API key (optionally with write permission and a bound account).
4. Call the API with `Authorization: Bearer <key>`.

## Environment

| Key       | Default              | Purpose                                                 |
|-----------|----------------------|---------------------------------------------------------|
| `PORT`    | `8430`               | listen port (binds `0.0.0.0`)                           |
| `DB_PATH` | `data/x-rest-api.db` | SQLite file (holds plaintext cookies/keys; kept `0600`) |

## Account selection

- **Reads rotate.** A read hits the next enabled account round-robin; an account
  that returns 429/404, or whose `x-rate-limit-remaining` hits 0, is locked
  **for that operation only** until its `x-rate-limit-reset` and skipped
  (failover). x.com rate-limits each GraphQL op separately, so an account cooling
  down on `search` still serves timelines. Active per-op locks show in `/admin`.
- **`X-Account: <label>`** pins any read to one account instead of rotating.
- **`/v1/home` and `/v1/bookmarks`** are account-scoped (they read the logged-in
  account's own feed), so they need a specific account: send `X-Account` or bind
  the API key to an account.
- **Bans auto-disable.** When x.com reports bad/expired cookies or access denied
  (error code 32/326, or 88 with budget left, or 403 "OK"), the account is turned
  off (`enabled=0`) with the reason shown in `/admin`; re-enable it after fixing
  the cookies. A stale-features error (code 336) triggers a queryId refresh.
- **Daily cap.** A per-account request cap (`daily_request_limit` in Settings,
  0 = unlimited) benches an account for the rest of the UTC day once it serves
  that many reads, spreading load and lowering the flag risk.
- **Writes never rotate.** A write acts as one identity, so it needs a specific
  account (`X-Account` or the key's bound account) and returns 400 without one.

## Endpoints

All reads are `GET`. `count` defaults to 40 (max 200). Success is `{"data": ...}`;
failure is `{"error": {"message": ...}}`. Every `/v1` route needs the Bearer key;
`/health` is open.

| Endpoint                                     | Description                                                                       |
|----------------------------------------------|-----------------------------------------------------------------------------------|
| `/health`                                    | liveness (no auth)                                                                |
| `/openapi.json`                              | OpenAPI 3.0.3 schema, auto-generated from the route table (no auth)               |
| `/docs`                                      | Swagger UI over `/openapi.json`, vendored (no auth)                               |
| `/v1/users/{handle}`                         | profile (numeric `{handle}` uses UserByRestId)                                    |
| `/v1/users/{handle}/id`                      | resolve one handle or numeric id to `{id, username}`                              |
| `/v1/users/{handle}/tweets`                  | a user's posts                                                                    |
| `/v1/users/{handle}/replies`                 | posts + replies                                                                   |
| `/v1/users/{handle}/replies-only`            | replies only (no standalone posts)                                                |
| `/v1/users/{handle}/reposts`                 | a user's reposts                                                                  |
| `/v1/users/{handle}/media`                   | media posts                                                                       |
| `/v1/users/{handle}/highlights`              | highlights                                                                        |
| `/v1/users/{handle}/likes`                   | a user's liked tweets                                                             |
| `/v1/users/{handle}/mentions`                | tweets mentioning a user                                                          |
| `/v1/users/{handle}/articles`                | your own long-form Articles; `?lifecycle=Published\|Draft` (account-scoped)       |
| `/v1/users/{handle}/followers`               | followers                                                                         |
| `/v1/users/{handle}/following`               | following                                                                         |
| `/v1/users/{handle}/verified-followers`      | verified (blue) followers                                                         |
| `/v1/users/{handle}/subscriptions`           | creators the user subscribes to                                                   |
| `/v1/users/{handle}/about`                   | account origin, username history, identity verification                           |
| `/v1/users/{handle}/rss`                     | a user's posts as an RSS 2.0 feed                                                 |
| `/v1/users/by?ids=` / `?handles=`            | batch profile lookup by numeric id or handle (max 100)                            |
| `/v1/users/resolve?ids=` / `?handles=`       | bulk `{id, username}`: `ids` maps to usernames, `handles` maps to ids (max 100)   |
| `/v1/users/latest?ids=` / `?handles=`        | most recent tweet of each user (one per user, max 100)                            |
| `/v1/tweets/{id}`                            | focal tweet + reply thread                                                        |
| `/v1/tweets/{id}/result`                     | single tweet, no thread                                                           |
| `/v1/tweets/{id}/thread`                     | tweets in the conversation (self-thread); `?sort=relevance\|recency\|likes`       |
| `/v1/tweets/{id}/replies`                    | direct replies; `?sort=relevance\|recency\|likes`                                 |
| `/v1/tweets/{id}/history`                    | edit history (raw GQL)                                                            |
| `/v1/tweets/{id}/retweeters`                 | who reposted                                                                      |
| `/v1/tweets/{id}/likers`                     | who liked                                                                         |
| `/v1/tweets/{id}/quotes`                     | tweets quoting a tweet                                                            |
| `/v1/tweets/{id}/hidden`                     | hidden replies under a tweet                                                      |
| `/v1/tweets/by?ids=`                         | batch tweet lookup (comma-separated numeric ids, max 100)                         |
| `/v1/search?q=&product=Latest`               | keyword/filter search (tweets)                                                    |
| `/v1/search/people?q=`                       | keyword/filter search (users)                                                     |
| `/v1/search/rss?q=`                          | search results as an RSS 2.0 feed                                                 |
| `/v1/lists/{id}`                             | list metadata (parsed; `?raw=true` for GQL)                                       |
| `/v1/lists/by-slug?owner=&slug=`             | list metadata by owner handle + slug                                              |
| `/v1/lists/{id}/tweets`                      | list timeline                                                                     |
| `/v1/lists/{id}/rss`                         | list timeline as an RSS 2.0 feed                                                  |
| `/v1/lists/{id}/members`                     | list members                                                                      |
| `/v1/spaces/live`                            | live Spaces from your network (account-scoped)                                    |
| `/v1/spaces/{id}`                            | Space info by id (raw GQL)                                                        |
| `/v1/spaces/{id}/stream`                     | a Space's live stream status (playback source, share url)                         |
| `/v1/hashflags`                              | active hashflag emojis (hashmojis)                                                |
| `/v1/notifications`                          | notifications timeline (raw GQL, account-scoped)                                  |
| `/v1/bookmarks/folders`                      | bookmark folders (raw GQL, account-scoped)                                        |
| `/v1/bookmarks/folders/{id}`                 | tweets in a bookmark folder (account-scoped)                                      |
| `/v1/communities/{id}`                       | community info (raw GQL)                                                          |
| `/v1/communities/{id}/tweets`                | community timeline                                                                |
| `/v1/communities/{id}/members`               | community members                                                                 |
| `/v1/communities/{id}/moderators`            | community moderators                                                              |
| `/v1/trends?category=trending`               | trends (raw GQL); `trending\|news\|sport\|entertainment`                          |
| `/v1/trends/sidebar`                         | personalized "What's happening" trends (ExploreSidebar)                           |
| `/v1/explore`                                | Explore "For You" trends, incl. AI news stories (ExplorePage)                     |
| `/v1/bookmarks`                              | bookmarks (account-scoped)                                                        |
| `/v1/home`                                   | home feed; `?chronological=true` for the Following (latest) feed (account-scoped) |
| `/v1/users/{handle}/affiliates`              | a user's business-profile affiliates                                              |
| `/v1/suggestions?creator_only=`              | who-to-follow recommendations (account-scoped)                                    |
| `/v1/blocks`                                 | blocked accounts (account-scoped)                                                 |
| `/v1/account/me`                             | your own profile (account-scoped)                                                 |
| `/v1/lists`                                  | your own lists (account-scoped)                                                   |
| `/v1/analytics?from_time=&to_time=&metrics=` | account analytics overview (raw, account-scoped)                                  |
| `/v1/analytics/overview`                     | typed account analytics (followers, engagements, follows; account-scoped)        |
| `/v1/jobs/search?keyword=&location=`         | search X Jobs                                                                     |
| `/v1/jobs/{id}`                              | job details                                                                       |
| `/v1/jobs/locations?query=`                  | job location suggestions                                                          |
| `/v1/dm/inbox`                               | direct message inbox (account-scoped)                                             |
| `/v1/dm/conversations/{id}`                  | a DM conversation (account-scoped)                                                |

Every list read accepts `?cursor=<c>` for manual paging (the response then carries
a top-level `next_cursor`), `?raw=true` to return the unparsed GraphQL response
instead of the flat model, and `?format=csv` to return CSV instead of JSON (the
next cursor then comes back in the `X-Next-Cursor` header). Parsed tweets/users
include media (with best-bitrate video variants), entities
(hashtags/cashtags/mentions/links), nested quote/retweet, cards/polls,
conversation/reply ids, and richer profile fields. Endpoints marked "raw GQL" have
no flat model and always return the raw response.

Any endpoint that takes a `{handle}` also accepts a numeric id, an `@handle`, or a
profile URL (`x.com/<handle>`, `x.com/i/user/<id>`).

### Search filters

`/v1/search` and `/v1/search/people` build x.com's `rawQuery` from structured
params, so you do not have to write operators by hand. Pass `q` (kept verbatim)
and/or any of: `from`, `to`, `mention`, `all_words`, `any_words`, `exact_phrases`,
`exclude_words`, `hashtags`, `exclude_hashtags` (comma-separated); `lang`,
`tweet_type` (`originals_only`/`replies_only`/…); the booleans `verified`,
`blue_verified`, `has_images`, `has_videos`, `has_links`, `has_mentions`,
`has_hashtags`; the integers `min_faves` (alias `min_likes`), `min_replies`,
`min_retweets`; `since`/`until` (YYYY-MM-DD); geo `place`/`geocode`/`near`+`within`;
`list` (list id), `quoted_tweet_id`, `since_id`, `max_id`. Operators you already put
in `q` are never duplicated.

```bash
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8430/v1/search?from=naval&min_faves=500&has_links=true&since=2024-01-01"
```

Writes (need `enable_writes` on in Settings AND a key with write permission AND a
specific account):

| Endpoint                                  | Body                                                                             | Description                                                       |
|-------------------------------------------|----------------------------------------------------------------------------------|-------------------------------------------------------------------|
| `POST /v1/tweets`                         | `{"text": "...", "reply_to": "<id>", "quote_of": "<id>", "media_ids": ["<id>"]}` | post a tweet, reply, or quote                                     |
| `POST /v1/notes`                          | `{"text": "...", "reply_to": "<id>"}`                                            | post a long-form (note) tweet or reply (requires X Premium)       |
| `POST /v1/media`                          | multipart field `file`                                                           | upload media, returns `media_id` for `media_ids`                  |
| `DELETE /v1/tweets/{id}`                  | none                                                                             | delete a tweet                                                    |
| `POST /v1/tweets/{id}/like`               | none                                                                             | like a tweet                                                      |
| `POST /v1/tweets/{id}/unlike`             | none                                                                             | remove a like                                                     |
| `POST /v1/tweets/{id}/retweet`            | none                                                                             | repost a tweet                                                    |
| `DELETE /v1/tweets/{id}/retweet`          | none                                                                             | remove a repost                                                   |
| `POST /v1/tweets/{id}/bookmark`           | none                                                                             | bookmark a tweet                                                  |
| `DELETE /v1/tweets/{id}/bookmark`         | none                                                                             | remove a bookmark                                                 |
| `POST /v1/tweets/{id}/hide`               | none                                                                             | hide a reply (you must be the conversation author)               |
| `DELETE /v1/tweets/{id}/hide`             | none                                                                             | unhide a reply                                                   |
| `POST /v1/users/{handle}/follow`          | none                                                                             | follow a user                                                     |
| `DELETE /v1/users/{handle}/follow`        | none                                                                             | unfollow a user                                                   |
| `POST /v1/users/{handle}/mute`            | none                                                                             | mute a user (hides their posts)                                   |
| `DELETE /v1/users/{handle}/mute`          | none                                                                             | unmute a user                                                     |
| `POST /v1/users/{handle}/block`           | none                                                                             | block a user                                                      |
| `DELETE /v1/users/{handle}/block`         | none                                                                             | unblock a user                                                    |
| `GET /v1/scheduled`                       | none                                                                             | your scheduled (unsent) tweets (account-scoped)                   |
| `POST /v1/scheduled`                      | `{"text": "...", "execute_at": <unix>}`                                          | schedule a tweet for a future time                                |
| `DELETE /v1/scheduled/{id}`               | none                                                                             | cancel a scheduled tweet                                          |
| `POST /v1/grok/chat`                      | `{"messages": [{"role": "user", "content": "..."}], "conversation_id": "..."}`   | chat with Grok; omit `conversation_id` to start a new chat        |
| `POST /v1/lists`                          | `{"name": "...", "description": "...", "is_private": false}`                     | create a list, returns `{"id": "..."}`                            |
| `PATCH /v1/lists/{id}`                    | `{"name": "...", "description": "...", "is_private": true}`                      | update a list (omit fields to leave unchanged)                    |
| `DELETE /v1/lists/{id}`                   | none                                                                             | delete a list                                                     |
| `POST /v1/lists/{id}/members`             | `{"user_id": "<id>"}`                                                            | add a member to a list                                            |
| `DELETE /v1/lists/{id}/members/{userId}`  | none                                                                             | remove a member from a list                                       |
| `POST /v1/lists/{id}/mute`                | none                                                                             | mute a list                                                       |
| `DELETE /v1/lists/{id}/mute`              | none                                                                             | unmute a list                                                     |
| `POST /v1/dm/conversations/{id}/messages` | `{"text": "..."}`                                                                | send a direct message into a conversation                         |
| `DELETE /v1/dm/conversations/{id}`        | none                                                                             | delete (leave) a DM conversation                                  |
| `DELETE /v1/users/{handle}/follower`      | none                                                                             | remove a follower                                                 |
| `POST /v1/account/username`               | `{"username": "..."}`                                                            | change your @username                                             |
| `PATCH /v1/account/profile`               | `{"name": "...", "url": "...", "location": "...", "description": "..."}`         | update your profile (omit fields to leave unchanged)              |
| `PUT /v1/account/profile/image`           | `{"image_base64": "..."}`                                                        | set your avatar from base64                                       |
| `PUT /v1/account/profile/banner`          | `{"banner_base64": "..."}`                                                       | set your banner from base64                                       |
| `POST /v1/account/password`               | `{"current_password": "...", "new_password": "..."}`                             | change your password (may rotate the session; re-capture cookies) |

```bash
curl -H "Authorization: Bearer $KEY" http://localhost:8430/v1/users/naval
curl -H "Authorization: Bearer $KEY" -H "X-Account: main" http://localhost:8430/v1/home
```

## API docs (OpenAPI)

`/openapi.json` is generated at startup from the `/v1` route table and the real
Go response types (via reflection), so it never drifts from the code: add a
route and it appears in the schema. `/openapi-v2.json` is a separate hand-written
document for the `/2` surface. `/docs` serves Swagger UI (vendored, no CDN)
against `/openapi.json`. All three are open (no API key), like `/health`.

```bash
curl http://localhost:8430/openapi.json
curl http://localhost:8430/openapi-v2.json
open http://localhost:8430/docs
```

## X API v2 compatible layer (`/2`)

`/2` mirrors the official X API v2 surface: the same paths, the same
`{data, includes, meta, errors}` envelope, and the same `Authorization: Bearer`
auth as `/v1`. Point an existing `api.x.com` client at this server, keep its
Bearer key, and only change the base URL. Custom features stay on `/v1`; they are
not mirrored to `/2`.

**Field selection** works exactly like X v2: pass `tweet.fields`, `user.fields`,
`media.fields`, `poll.fields`, `place.fields`, `list.fields`, `space.fields`
(comma-separated), and `expansions` to pull related objects into `includes`. Each
`*.fields` set is added to the v2 default set. Fields with no source in the
upstream payload are omitted (the parameter is still accepted). Timelines use the
v2 paging params `max_results` and `pagination_token`, and return
`meta.result_count` / `meta.next_token`.

Reads:

| Endpoint                                              | Description                                  |
|------------------------------------------------------|----------------------------------------------|
| `/2/users/by/username/{username}`                    | one user by handle                           |
| `/2/users/by?usernames=a,b`                          | many users by handle                         |
| `/2/users/me`                                         | the authenticated user (account-scoped)      |
| `/2/users/{id}`                                       | one user by numeric id                       |
| `/2/users?ids=1,2`                                    | many users by numeric id                     |
| `/2/users/{id}/tweets`                               | a user's posts                               |
| `/2/users/{id}/mentions`                             | tweets mentioning a user                     |
| `/2/users/{id}/timelines/reverse_chronological`      | the user's reverse-chronological home        |
| `/2/users/{id}/liked_tweets`                         | a user's liked tweets                        |
| `/2/users/{id}/followers`                            | followers                                    |
| `/2/users/{id}/following`                            | following                                    |
| `/2/users/{id}/blocking`                             | blocked accounts (account-scoped)            |
| `/2/users/{id}/bookmarks`                            | bookmarks (account-scoped)                   |
| `/2/tweets/{id}`                                      | one tweet                                    |
| `/2/tweets?ids=1,2`                                   | many tweets                                  |
| `/2/tweets/search/recent?query=`                     | recent tweet search                          |
| `/2/tweets/{id}/retweeted_by`                        | users who reposted                           |
| `/2/tweets/{id}/liking_users`                        | users who liked                              |
| `/2/tweets/{id}/quote_tweets`                        | tweets quoting a tweet                       |
| `/2/lists/{id}`                                       | list metadata                               |
| `/2/lists/{id}/tweets`                               | list timeline                                |
| `/2/lists/{id}/members`                              | list members                                 |
| `/2/spaces/{id}`                                      | Space by id                                  |
| `/2/dm_events`                                        | direct message events (account-scoped)       |

Writes (same gate as `/v1`: `enable_writes` on, a write key, and a specific
account). The v2 write bodies and paths match X v2, and a refusal returns the v2
problem-details error body:

| Endpoint                                          | Description                        |
|---------------------------------------------------|------------------------------------|
| `POST /2/tweets`                                  | post a tweet                       |
| `DELETE /2/tweets/{id}`                           | delete a tweet                     |
| `PUT /2/tweets/{id}/hidden`                       | hide/unhide a reply                |
| `POST /2/users/{id}/likes`                        | like a tweet                       |
| `DELETE /2/users/{id}/likes/{tweet_id}`           | remove a like                      |
| `POST /2/users/{id}/retweets`                     | repost a tweet                     |
| `DELETE /2/users/{id}/retweets/{source_tweet_id}` | remove a repost                    |
| `POST /2/users/{id}/following`                    | follow a user                      |
| `DELETE /2/users/{id}/following/{target_id}`      | unfollow a user                    |
| `POST /2/users/{id}/muting`                       | mute a user                        |
| `DELETE /2/users/{id}/muting/{target_id}`         | unmute a user                      |
| `POST /2/users/{id}/blocking`                     | block a user                       |
| `DELETE /2/users/{id}/blocking/{target_id}`       | unblock a user                     |
| `POST /2/users/{id}/bookmarks`                    | bookmark a tweet                   |
| `DELETE /2/users/{id}/bookmarks/{tweet_id}`       | remove a bookmark                  |
| `POST /2/lists`                                   | create a list                      |
| `PUT /2/lists/{id}`                               | update a list                      |
| `DELETE /2/lists/{id}`                            | delete a list                      |
| `POST /2/lists/{id}/members`                      | add a list member                  |
| `DELETE /2/lists/{id}/members/{user_id}`          | remove a list member               |
| `POST /2/dm_conversations/{id}/messages`          | send a direct message             |

```bash
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8430/2/users/by/username/jack?user.fields=created_at,public_metrics,verified"
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8430/2/tweets/20?tweet.fields=created_at,public_metrics&expansions=author_id&user.fields=username"
```

## Admin panel

- **Dashboard** — account/key/request counts and recent requests.
- **Accounts** — add/enable/disable/delete accounts; shows status and active
  per-op locks (which operations each account is cooling down on).
- **API Keys** — create (shown once, viewable later), toggle, delete; set write
  permission and bound account.
- **Logs** — every `/v1` request (method, path, status, upstream status, latency,
  account, key, IP, error) with path filter and paging.
- **Query IDs** — refresh each operation's `queryId` live from the x.com client
  bundle (they rotate on web releases); overrides persist and win over the
  embedded `ops.json`, while `variables` stay from `ops.json`. The same refresh
  reads each op's `featureSwitches` from the bundle and adds any flag x.com
  introduced that `ops.json` lacks (defaulted off), which clears stale-features
  (code 336) errors caused by a new flag.
- **Settings** — toggle writes, the public no-auth fallback, log retention, the
  per-account daily request cap, and transport (proxy / User-Agent / TLS client
  profile / `x-client-transaction-id` override). The TLS client profile (e.g.
  `chrome_146`) sets the browser fingerprint; bump it if x.com starts returning
  Cloudflare 403 blocks. Transport changes apply on restart; writes, fallback,
  daily cap, accounts and keys apply live.

## Public no-auth fallback (optional)

Off by default. Enable it in Settings (`enable_public_fallback`). When every
account is exhausted/cooling down, or an authed read is rejected (401/403),
`GET /v1/users/{handle}` (non-numeric) and `GET /v1/tweets/{id}` /
`/v1/tweets/{id}/result` fall back to `api.fxtwitter.com` (no cookie). The thread
form returns `{tweet, replies: []}` because FxTwitter serves a single tweet.
Trade-off: this leaks the queried id/handle to a third party, so it is opt-in.

## Docker

```bash
docker build -t x-rest-api .
docker run -p 8430:8430 -v x-rest-api-data:/app/data x-rest-api
```

The SQLite database lives in the `/app/data` volume.

## Testing

```bash
make test   # offline parser + tx golden + store tests, no network
```

## Security notes

- Account cookies and API keys are stored in SQLite as plaintext, by design; keep
  the DB file and its volume private (it is created `0600`).
- The panel serves on the same port as the API; put the whole service behind TLS
  and network controls if it is exposed beyond localhost.

## Responsible use

Only against your own account or authorized targets. Rate-limit, keep counts
modest. Datacenter IPs (a VPS) are the top flag signal; run from a residential IP
or set a residential proxy in Settings.

## Layout

```
cmd/x-rest-api/main.go   entrypoint: port, DB, session, server, graceful shutdown
internal/config          PORT / DB_PATH only
internal/store           SQLite: settings, admins, sessions, accounts, keys,
                         logs, query_ids, account_locks (per-op rotation locks)
internal/xapi            shared transport (Session) + per-account client, parsers,
                         ops.json, x-client-transaction-id generator
internal/openapi         OpenAPI 3 generator (reflection-based schema + spec)
internal/apiv2           X API v2 envelope + field-selection/expansion engine
internal/server          /v1 and /2 routers, API-key + logging middleware,
                         rotation pool, /openapi.json + /openapi-v2.json,
                         vendored Swagger UI /docs
internal/server/admin    htmx dark-mode panel (templates + static, go:embed)
```

## License

MIT. See [LICENSE](LICENSE).
