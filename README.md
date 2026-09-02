# x-rest-api

Read/write HTTP REST API over x.com (Twitter)'s private GraphQL surface, in Go,
with an htmx dark-mode admin panel. Multi-account cookie rotation, API-key
management, and full request logging. The REST API and the admin panel share one
port: `/v1` is the API, `/admin` is the panel.

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
| `/v1/users/{handle}/tweets`                  | a user's posts                                                                    |
| `/v1/users/{handle}/replies`                 | posts + replies                                                                   |
| `/v1/users/{handle}/media`                   | media posts                                                                       |
| `/v1/users/{handle}/highlights`              | highlights                                                                        |
| `/v1/users/{handle}/likes`                   | a user's liked tweets                                                             |
| `/v1/users/{handle}/followers`               | followers                                                                         |
| `/v1/users/{handle}/following`               | following                                                                         |
| `/v1/users/{handle}/verified-followers`      | verified (blue) followers                                                         |
| `/v1/users/{handle}/subscriptions`           | creators the user subscribes to                                                   |
| `/v1/users/{handle}/about`                   | account origin, username history, identity verification                           |
| `/v1/users/{handle}/rss`                     | a user's posts as an RSS 2.0 feed                                                 |
| `/v1/users/by?ids=`                          | batch profile lookup (comma-separated numeric ids, max 100)                       |
| `/v1/tweets/{id}`                            | focal tweet + reply thread                                                        |
| `/v1/tweets/{id}/result`                     | single tweet, no thread                                                           |
| `/v1/tweets/{id}/thread`                     | tweets in the conversation (self-thread); `?sort=relevance\|recency\|likes`       |
| `/v1/tweets/{id}/replies`                    | direct replies; `?sort=relevance\|recency\|likes`                                 |
| `/v1/tweets/{id}/history`                    | edit history (raw GQL)                                                            |
| `/v1/tweets/{id}/retweeters`                 | who reposted                                                                      |
| `/v1/tweets/{id}/likers`                     | who liked                                                                         |
| `/v1/tweets/by?ids=`                         | batch tweet lookup (comma-separated numeric ids, max 100)                         |
| `/v1/search?q=&product=Latest`               | keyword/filter search (tweets)                                                    |
| `/v1/search/people?q=`                       | keyword/filter search (users)                                                     |
| `/v1/search/rss?q=`                          | search results as an RSS 2.0 feed                                                 |
| `/v1/lists/{id}`                             | list metadata (parsed; `?raw=true` for GQL)                                       |
| `/v1/lists/by-slug?owner=&slug=`             | list metadata by owner handle + slug                                              |
| `/v1/lists/{id}/tweets`                      | list timeline                                                                     |
| `/v1/lists/{id}/rss`                         | list timeline as an RSS 2.0 feed                                                  |
| `/v1/lists/{id}/members`                     | list members                                                                      |
| `/v1/spaces/{id}`                            | Space info by id (raw GQL)                                                        |
| `/v1/spaces/{id}/stream`                     | a Space's live stream status (playback source, share url)                         |
| `/v1/notifications`                          | notifications timeline (raw GQL, account-scoped)                                  |
| `/v1/bookmarks/folders`                      | bookmark folders (raw GQL, account-scoped)                                        |
| `/v1/bookmarks/folders/{id}`                 | tweets in a bookmark folder (account-scoped)                                      |
| `/v1/communities/{id}`                       | community info (raw GQL)                                                          |
| `/v1/communities/{id}/tweets`                | community timeline                                                                |
| `/v1/communities/{id}/members`               | community members                                                                 |
| `/v1/communities/{id}/moderators`            | community moderators                                                              |
| `/v1/trends?category=trending`               | trends (raw GQL); `trending\|news\|sport\|entertainment`                          |
| `/v1/bookmarks`                              | bookmarks (account-scoped)                                                        |
| `/v1/home`                                   | home feed; `?chronological=true` for the Following (latest) feed (account-scoped) |
| `/v1/users/{handle}/affiliates`              | a user's business-profile affiliates                                              |
| `/v1/suggestions?creator_only=`              | who-to-follow recommendations (account-scoped)                                    |
| `/v1/lists`                                  | your own lists (account-scoped)                                                   |
| `/v1/analytics?from_time=&to_time=&metrics=` | account analytics overview (raw, account-scoped)                                  |
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
| `POST /v1/users/{handle}/follow`          | none                                                                             | follow a user                                                     |
| `DELETE /v1/users/{handle}/follow`        | none                                                                             | unfollow a user                                                   |
| `POST /v1/users/{handle}/mute`            | none                                                                             | mute a user (hides their posts)                                   |
| `DELETE /v1/users/{handle}/mute`          | none                                                                             | unmute a user                                                     |
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
route and it appears in the schema. `/docs` serves Swagger UI (vendored, no CDN)
against that schema. Both are open (no API key), like `/health`.

```bash
curl http://localhost:8430/openapi.json
open http://localhost:8430/docs
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
  per-account daily request cap, and transport (proxy / User-Agent /
  `x-client-transaction-id` override). Transport changes apply on restart; writes,
  fallback, daily cap, accounts and keys apply live.

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
internal/server          /v1 router, API-key + logging middleware, rotation pool,
                         OpenAPI /openapi.json + vendored Swagger UI /docs
internal/server/admin    htmx dark-mode panel (templates + static, go:embed)
```

## License

MIT.
