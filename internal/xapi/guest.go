package xapi

// Tier 1 of the credential-free read path: a guest token minted from x.com's own
// public web bearer. It reaches the same GraphQL surface as a session, but x.com
// only serves a narrow set of ops to logged-out clients, so this tier covers a
// single tweet and a profile lookup and nothing deeper.

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

const guestActivateURL = "https://api.x.com/1.1/guest/activate.json"

// guestTokenTTL is how long a minted token is reused. x.com states no lifetime,
// so this stays short enough that a token is re-minted before it is refused.
const guestTokenTTL = 30 * time.Minute

// guestToken returns a cached guest token, minting one when none is valid.
func (s *Session) guestToken() (string, error) {
	s.mu.RLock()
	tok, exp := s.guestTok, s.guestExp
	s.mu.RUnlock()
	if tok != "" && time.Now().Before(exp) {
		return tok, nil
	}

	tok, err := s.mintGuestToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.guestTok, s.guestExp = tok, time.Now().Add(guestTokenTTL)
	s.mu.Unlock()
	return tok, nil
}

// dropGuestToken clears the cache so the next read mints a fresh token.
func (s *Session) dropGuestToken() {
	s.mu.Lock()
	s.guestTok, s.guestExp = "", time.Time{}
	s.mu.Unlock()
}

// mintGuestToken activates a guest token against the public REST 1.1 endpoint.
func (s *Session) mintGuestToken() (string, error) {
	req, err := http.NewRequest(http.MethodPost, guestActivateURL, nil)
	if err != nil {
		return "", err
	}
	req.Header = http.Header{
		"authorization":     {"Bearer " + bearer},
		"accept":            {"application/json"},
		"user-agent":        {s.userAgent},
		http.HeaderOrderKey: {"authorization", "accept", "user-agent"},
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("guest: activate: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("guest: activate: http %d", resp.StatusCode)
	}
	var out struct {
		GuestToken string `json:"guest_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("guest: activate: decode: %w", err)
	}
	if out.GuestToken == "" {
		return "", fmt.Errorf("guest: activate: empty guest_token")
	}
	return out.GuestToken, nil
}

// guestHeaders builds the logged-out header set: the public bearer plus the
// guest token, and no cookie or CSRF pair.
func (s *Session) guestHeaders(tok string) http.Header {
	order := []string{
		"authorization", "x-guest-token", "x-twitter-active-user",
		"x-twitter-client-language", "content-type", "accept",
		"referer", "origin", "user-agent",
	}
	return http.Header{
		"authorization":             {"Bearer " + bearer},
		"x-guest-token":             {tok},
		"x-twitter-active-user":     {"yes"},
		"x-twitter-client-language": {"en"},
		"content-type":              {"application/json"},
		"accept":                    {"*/*"},
		"referer":                   {"https://x.com/"},
		"origin":                    {"https://x.com"},
		"user-agent":                {s.userAgent},
		http.HeaderOrderKey:         order,
	}
}

// callGuest runs one GraphQL read with a guest token. A rejected token is
// dropped and the read is retried once with a freshly minted one.
func (s *Session) callGuest(op string, variables map[string]any) (map[string]any, error) {
	payload, status, err := s.doGuestCall(op, variables)
	if err == nil {
		return payload, nil
	}
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return nil, err
	}
	s.dropGuestToken()
	payload, _, err = s.doGuestCall(op, variables)
	return payload, err
}

// doGuestCall performs a single guest GraphQL read and reports the HTTP status
// so the caller can decide whether re-minting the token is worth a retry.
func (s *Session) doGuestCall(op string, variables map[string]any) (map[string]any, int, error) {
	spec, err := opSpecFor(s, op, variables)
	if err != nil {
		return nil, 0, err
	}
	req, err := buildGraphQLRequest(spec.op, op, spec.vars)
	if err != nil {
		return nil, 0, err
	}
	tok, err := s.guestToken()
	if err != nil {
		return nil, 0, err
	}
	req.Header = s.guestHeaders(tok)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("guest %s: %w", op, err)
	}
	return decodeGraphQL(resp, "guest "+op)
}

// resolvedOp is an op spec with the session's queryId and feature overrides and
// the caller's variables already merged in.
type resolvedOp struct {
	op   OpSpec
	vars map[string]any
}

// opSpecFor resolves an op the same way the session-backed path does, so a
// rotated queryId or a new feature flag reaches the guest tier too.
func opSpecFor(s *Session, op string, variables map[string]any) (resolvedOp, error) {
	sp, err := spec(op)
	if err != nil {
		return resolvedOp{}, err
	}
	sp.QueryID = s.queryID(op, sp.QueryID)
	sp.Features = s.featuresFor(op, sp.Features)

	v := map[string]any{}
	maps.Copy(v, sp.Variables)
	maps.Copy(v, variables)
	return resolvedOp{op: sp, vars: v}, nil
}

// FetchTweetGuest fetches one tweet with a guest token (no cookie needed).
func (s *Session) FetchTweetGuest(id string) (*Tweet, error) {
	payload, err := s.callGuest("TweetResultByRestId", map[string]any{"tweetId": id})
	if err != nil {
		return nil, err
	}
	tw := parseTweet(asMap(dig(payload, "data", "tweetResult", "result")))
	if tw == nil {
		return nil, fmt.Errorf("guest: tweet %s: no result", id)
	}
	return tw, nil
}

// FetchUserGuest fetches a profile by handle with a guest token.
func (s *Session) FetchUserGuest(handle string) (*XUser, error) {
	payload, err := s.callGuest("UserByScreenName",
		map[string]any{"screen_name": normalizeHandle(handle)})
	if err != nil {
		return nil, err
	}
	u := parseUserByScreenName(payload)
	if u == nil || u.RestID == "" {
		return nil, fmt.Errorf("guest: user %s: no result", handle)
	}
	return u, nil
}
