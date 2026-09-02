package server

import (
	"errors"
	"net/http"
	"strconv"

	"x-rest-api/internal/apiv2"
	"x-rest-api/internal/xapi"
)

// v2 timeline page-size bounds, matching the X API v2 max_results contract.
const (
	v2DefaultMax = 10
	v2MinMax     = 5
	v2MaxMax     = 100
)

// v2ReadFn runs a v2 read against a client and returns the finished envelope. A
// nil error with an errors-only envelope is a normal not-found response; a
// non-nil error triggers rotation/failover.
type v2ReadFn func(*xapi.XClient) (apiv2.Envelope, error)

// serveV2 runs a v2 read against a chosen account with the same rotation and
// failover as serveRead, but writes the X API v2 envelope instead of the v1
// {data} wrapper. It has no public fallback.
func (s *Server) serveV2(w http.ResponseWriter, r *http.Request, scoped bool, op string, do v2ReadFn) {
	var lastErr error
	for range maxReadAttempts {
		acct, pinned, err := s.pickAccount(r, scoped, op)
		if err != nil {
			s.failPickV2(w, r, err)
			return
		}
		setAccount(r, acct.ID)
		cli := xapi.NewClientFor(s.sess, toXAPI(acct))
		env, err := do(cli)
		s.pool.Observe(acct.ID, op, cli.RateLimit())
		if err == nil {
			_ = s.store.MarkAccountUsed(acct.ID)
			writeJSON(w, http.StatusOK, env)
			return
		}
		if up := asUpstream(err); up != nil {
			if s.handleV2Upstream(w, r, up, acct.ID, op, pinned, cli.RateLimit()) {
				lastErr = err
				continue
			}
			return
		}
		writeV2Fail(w, r, err)
		return
	}
	writeV2Fail(w, r, lastErr)
}

// handleV2Upstream applies the shared upstream side effects and writes a v2 error
// body when terminal; it returns true to rotate to another account.
func (s *Server) handleV2Upstream(w http.ResponseWriter, r *http.Request, up *xapi.UpstreamError, accountID int64, op string, pinned bool, rl *xapi.RateLimit) bool {
	switch s.applyUpstreamEffect(up, accountID, op, pinned, rl) {
	case outcomeRetry:
		return true
	case outcomeHTMLBlock:
		recordErr(r, up)
		writeJSON(w, http.StatusBadGateway,
			apiv2.Invalid("", "upstream returned an anti-bot/HTML block; try again later"))
		return false
	}
	writeV2Fail(w, r, up)
	return false
}

// writeV2Fail maps a client/upstream error to an HTTP status and writes a v2
// problem-details body, mirroring an upstream status when present.
func writeV2Fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadGateway
	if up := asUpstream(err); up != nil {
		status = up.Status
		if ri := getReqInfo(r); ri != nil {
			ri.upstreamStatus = &up.Status
		}
	}
	recordErr(r, err)
	writeJSON(w, status, apiv2.Invalid("", err.Error()))
}

// failPickV2 maps an account-selection error to a v2 problem-details body.
func (s *Server) failPickV2(w http.ResponseWriter, r *http.Request, err error) {
	recordErr(r, err)
	if errors.Is(err, errNeedAccount) {
		writeJSON(w, http.StatusBadRequest,
			apiv2.Invalid("", "this endpoint needs a specific account: send X-Account or bind the API key to an account"))
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, apiv2.Invalid("", err.Error()))
}

// v2MaxResults reads the v2 max_results parameter, clamping to [v2MinMax, v2MaxMax]
// with a v2DefaultMax default.
func v2MaxResults(r *http.Request) int {
	raw := r.URL.Query().Get("max_results")
	if raw == "" {
		return v2DefaultMax
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < v2MinMax {
		return v2DefaultMax
	}
	if n > v2MaxMax {
		return v2MaxMax
	}
	return n
}
