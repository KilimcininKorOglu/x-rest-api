package xapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	http "github.com/bogdanfinn/fhttp"
)

// UpstreamError is returned when x.com replies with a non-2xx status. It carries
// the HTTP status so the layer can mirror rate limits, plus the first x.com error
// Code/Msg and an HTML flag so the pool can tell a ban from a rate limit from a
// Cloudflare block.
type UpstreamError struct {
	Op     string
	Status int
	Body   string
	Code   int    // first x.com error code (0 when none), e.g. 32, 88, 326, 336, -1
	Msg    string // first x.com error message
	HTML   bool   // body was HTML (Cloudflare/anti-bot block, not a GraphQL error)
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("%s: upstream status %d: %s", e.Op, e.Status, e.Body)
}

// TxRequiredError is returned when a hardened op (search) is called without the
// x-client-transaction-id. It is a caller configuration issue, not an upstream
// failure, so the HTTP layer maps it to 400.
type TxRequiredError struct {
	Op string
}

func (e *TxRequiredError) Error() string {
	return fmt.Sprintf("%s needs a valid x-client-transaction-id — set X_TX_ID "+
		"(copy it from a real search request's headers in DevTools); it is reusable "+
		"for a while, refresh it when search starts 404-ing", e.Op)
}

// parseXErrors extracts the first {code,message} from an x.com GraphQL error body.
func parseXErrors(body []byte) (int, string) {
	var r struct {
		Errors []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &r) == nil && len(r.Errors) > 0 {
		return r.Errors[0].Code, r.Errors[0].Message
	}
	return 0, ""
}

// isHTMLBlock reports whether a response body is an HTML anti-bot page rather
// than a GraphQL JSON error. x.com sits behind Cloudflare, so a cf-ray header is
// present on every response and is NOT by itself a block; only an actual HTML
// body counts. A GraphQL error body is JSON and starts with '{'.
func isHTMLBlock(h http.Header, body []byte) bool {
	if strings.Contains(strings.ToLower(h.Get("content-type")), "text/html") {
		return true
	}
	return bytes.HasPrefix(bytes.TrimSpace(body), []byte("<"))
}

// reservedPaths are x.com URL segments that are not usernames, so a profile URL
// that starts with one is not a handle.
var reservedPaths = map[string]bool{
	"i": true, "home": true, "search": true, "explore": true, "notifications": true,
	"messages": true, "settings": true, "compose": true, "hashtag": true, "intent": true,
}

// normalizeHandle accepts a bare handle, @handle, a numeric id, or a profile URL
// (x.com/handle, x.com/i/user/<id>, twitter.com/handle) and returns the handle or
// numeric id. A URL to /i/user/<id> yields the numeric id, which resolveUID then
// treats as an id rather than a handle.
func normalizeHandle(h string) string {
	h = strings.TrimSpace(h)
	// A real handle is only [A-Za-z0-9_]; a '/' or '.' means a URL or domain, so
	// resolve it as one and never fall back to the raw string (an invalid URL is
	// not a handle).
	if strings.ContainsAny(h, "/.") {
		return userRefFromURL(h)
	}
	return strings.TrimPrefix(h, "@")
}

// userRefFromURL extracts a handle or numeric id from an x.com/twitter.com profile
// URL, or "" when the string is not such a URL. A missing scheme is tolerated.
func userRefFromURL(s string) string {
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	if host != "x.com" && host != "twitter.com" && host != "mobile.twitter.com" {
		return ""
	}
	var segs []string
	for seg := range strings.SplitSeq(strings.Trim(u.Path, "/"), "/") {
		if seg != "" {
			segs = append(segs, seg)
		}
	}
	if len(segs) == 0 {
		return ""
	}
	// /i/user/<id> -> the numeric id.
	if segs[0] == "i" {
		if len(segs) >= 3 && segs[1] == "user" && isDigits(segs[2]) {
			return segs[2]
		}
		return ""
	}
	if reservedPaths[strings.ToLower(segs[0])] {
		return ""
	}
	return segs[0]
}

// isDigits reports whether s is a non-empty run of ASCII digits (a numeric id).
func isDigits(s string) bool {
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// truncate returns at most n bytes of b as a string.
func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n])
	}
	return string(b)
}
