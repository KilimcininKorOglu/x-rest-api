# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2026-09-03

### Added
- X API v2 compatible `/2` surface: field-selection and envelope mapping layer plus read endpoints
- `/2` recent tweet search at `/2/tweets/search/recent`
- `/2` write scaffolding with POST/DELETE `/2/tweets`
- `/2` like and retweet write endpoints
- `/2` follow and unfollow write endpoints
- `/2` engagement and user-graph read endpoints
- `/2` bookmark and mute endpoints
- `/2` list read and write endpoints
- `/2` direct-message endpoints
- `/2/spaces/{id}` with typed Space parsing
- Account analytics overview endpoint
- Replies-only and reposts user timeline endpoints
- Explore For You trends endpoint
- Personalized sidebar trends endpoint
- Long-form Articles read endpoint and the Article embedded in a tweet
- Hashflags and live-spaces read endpoints
- Hide-reply endpoints (ModerateTweet/UnmoderateTweet)
- Quote-tweets and mentions endpoints
- Authenticated-user profile on `/v1` and `/2`
- Blocked-accounts list on `/v1` and `/2`
- Block and unblock over REST 1.1 on `/v1` and `/2`
- Bulk latest-tweet endpoint and handle lookup on `/users/by`
- id<->username resolver endpoints
- `author_id` and `in_reply_to_user_id` exposed on Tweet

### Changed
- Serve a separate OpenAPI document for the `/2` surface
- Report missing ids in v2 batch lookups as partial errors
- Anonymize captured test fixtures with synthetic data
- Feed the CHANGELOG section to GoReleaser as release notes

### Fixed
- Bump the Go toolchain floor to 1.26.6
- Clear gosec and golangci-lint findings
- Read user counts and bio from the newer GraphQL schema

## [1.0.0] - 2026-09-02

### Added
- REST API over x.com's private GraphQL surface with an htmx admin panel, multi-account cookie rotation, API keys, and request logging
- Quote tweets and media attachment on tweet creation
- User discovery reads: suggestions, affiliates, own lists, and analytics
- List management writes and parsed list metadata
- Resolve a list by owner handle and slug (ListBySlug)
- X Jobs reads: search, details, and locations
- Direct message reads, conversation delete, and message send
- Profile and account management writes
- User mute/unmute endpoints
- Grok chat endpoint
- Space live stream status endpoint
- Configurable TLS client profile and User-Agent from Settings
- Central version constant surfaced in OpenAPI and the /health response

### Fixed
- Send x-client-transaction-id on every request, so hardened ops stop returning 404
- Align the CreateRetweet queryId with the live web client
- Bump the TLS profile to chrome_146 to clear a Cloudflare block on by-rest-id lookups

### Changed
- Add GitHub Actions CI and a GoReleaser release workflow
- Expand the live smoke test suite and endpoint documentation
