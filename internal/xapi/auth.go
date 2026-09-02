package xapi

import (
	"fmt"
	"maps"
	"sync"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// bearer is the public web-client token hardcoded in x.com's main.js. It is not
// a secret: every browser and every client sends the same one. It identifies the
// app, not the user.
const bearer = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs=" +
	"1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"

// defaultUA matches the pinned browser fingerprint below.
const defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// clientProfile pins a concrete Chrome TLS profile so the TLS/HTTP2 fingerprint
// stays stable and matches a browser, replacing curl_cffi's impersonate="chrome124".
func clientProfile() profiles.ClientProfile {
	if p, ok := profiles.MappedTLSClients["chrome_124"]; ok {
		return p
	}
	return profiles.DefaultClientProfile
}

// Session is the shared upstream transport: one impersonating HTTP client and one
// x-client-transaction-id generator, reused across all accounts. Per-account
// cookies are supplied at request time.
type Session struct {
	client    tls_client.HttpClient
	userAgent string
	txID      string // optional global override for x-client-transaction-id
	gen       *txGen

	mu         sync.RWMutex
	queryIDs   map[string]string   // op -> queryId overrides (auto-refreshed from the bundle)
	featSwitch map[string][]string // op -> feature-flag names the live bundle lists for it
}

// SetQueryIDs replaces the queryId override map (from the DB or a bundle refresh).
func (s *Session) SetQueryIDs(ids map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queryIDs = ids
}

// SetFeatureSwitches replaces the per-op feature-flag name map (from a bundle
// refresh). featuresFor uses it to add flags x.com introduced that ops.json lacks.
func (s *Session) SetFeatureSwitches(m map[string][]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.featSwitch = m
}

// queryID returns the override for op, or the embedded fallback.
func (s *Session) queryID(op, fallback string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if qid, ok := s.queryIDs[op]; ok && qid != "" {
		return qid
	}
	return fallback
}

// featuresFor returns the effective features for op: the embedded base plus any
// flag the live bundle lists for op but the base omits, defaulted to false. This
// keeps a stale ops.json from missing a newly introduced flag (x.com code 336),
// because x.com rejects on a flag's absence, not its value. Discovered flags are
// added as false.
func (s *Session) featuresFor(op string, base map[string]any) map[string]any {
	s.mu.RLock()
	flags := s.featSwitch[op]
	s.mu.RUnlock()
	if len(flags) == 0 {
		return base
	}
	out := make(map[string]any, len(base)+len(flags))
	maps.Copy(out, base)
	for _, f := range flags {
		if _, ok := out[f]; !ok {
			out[f] = false
		}
	}
	return out
}

// NewSession builds the shared transport. proxy, userAgent and txID may be empty
// (they come from the settings table).
func NewSession(userAgent, proxy, txID string) (*Session, error) {
	if userAgent == "" {
		userAgent = defaultUA
	}
	opts := []tls_client.HttpClientOption{
		// 60s (not 30s) so Grok's streamed reasoning replies finish; ordinary ops
		// return in under two seconds, so the higher ceiling never delays them.
		tls_client.WithTimeoutSeconds(60),
		tls_client.WithClientProfile(clientProfile()),
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
	}
	if proxy != "" {
		opts = append(opts, tls_client.WithProxyUrl(proxy))
	}
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
	if err != nil {
		return nil, fmt.Errorf("build tls client: %w", err)
	}
	s := &Session{client: client, userAgent: userAgent, txID: txID}
	s.gen = newTxGen(client, userAgent, "")
	return s, nil
}

// Account carries the per-request session cookies.
type Account struct {
	ID        int64
	Label     string
	AuthToken string
	CT0       string
}

// cookie renders the Cookie header for an account.
func (s *Session) cookie(a Account) string {
	return fmt.Sprintf("auth_token=%s; ct0=%s", a.AuthToken, a.CT0)
}

// headers builds request headers for an account in a fixed, browser-like order.
// A non-empty txValue adds x-client-transaction-id (hardened ops need it).
func (s *Session) headers(a Account, lang, txValue string) http.Header {
	order := []string{
		"authorization", "x-csrf-token", "x-twitter-active-user",
		"x-twitter-auth-type", "x-twitter-client-language", "content-type",
		"accept", "referer", "origin", "cookie", "user-agent",
	}
	h := http.Header{
		"authorization":             {"Bearer " + bearer},
		"x-csrf-token":              {a.CT0}, // double-submit: must equal the ct0 cookie
		"x-twitter-active-user":     {"yes"},
		"x-twitter-auth-type":       {"OAuth2Session"},
		"x-twitter-client-language": {lang},
		"content-type":              {"application/json"},
		"accept":                    {"*/*"},
		"referer":                   {"https://x.com/"},
		"origin":                    {"https://x.com"},
		"cookie":                    {s.cookie(a)},
		"user-agent":                {s.userAgent},
	}
	if txValue != "" {
		h["x-client-transaction-id"] = []string{txValue}
		order = append(order, "x-client-transaction-id")
	}
	h[http.HeaderOrderKey] = order
	return h
}

// transactionID returns the x-client-transaction-id for a request. A configured
// global override wins; otherwise it is generated at runtime from method+path.
func (s *Session) transactionID(method, path string) (string, error) {
	if s.txID != "" {
		return s.txID, nil
	}
	return s.gen.TransactionID(method, path)
}
