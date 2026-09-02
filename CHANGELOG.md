# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
