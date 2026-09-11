package server

// On-demand account health check. Account validation is otherwise passive: a
// dead cookie only surfaces when a real read fails. This probe lets the operator
// ask the question directly, before a user request hits the bad account.

import (
	"errors"

	"x-rest-api/internal/xapi"
)

// probeOp is the op the probe runs. Viewer is account-scoped, so a success
// proves the cookies identify a logged-in account rather than only that a public
// read works.
const probeOp = "Viewer"

// errNoHandle reports cookies that authenticate but name no handle. The label
// is the handle, so an account cannot be stored without one.
var errNoHandle = errors.New("the cookies authenticate but report no handle")

// ProbeAccount checks whether one account's cookies still authenticate. It
// returns the handle the cookies identify, and whether the probe disabled the
// account. A real auth failure disables it through the same path a failed live
// read uses; a transient error or a rate limit leaves it enabled.
func (s *Server) ProbeAccount(id int64) (string, bool, error) {
	acct, err := s.store.GetAccount(id)
	if err != nil {
		return "", false, err
	}
	cli := xapi.NewClientFor(s.sess, toXAPI(acct))
	u, err := cli.Me()
	rl := cli.RateLimit()
	s.pool.Observe(acct.ID, probeOp, rl)
	if err == nil {
		_ = s.store.MarkAccountUsed(acct.ID)
		return u.ScreenName, false, nil
	}
	if up := asUpstream(err); up != nil && isBanned(up, rl) {
		_ = s.store.DisableAccount(acct.ID, banReason(up))
		return "", true, err
	}
	return "", false, err
}

// VerifyCookies resolves the handle a cookie pair belongs to, without touching
// the store. The account panel calls it before the insert, so a new account is
// labelled with the handle x.com reports rather than a name the operator typed.
func (s *Server) VerifyCookies(authToken, ct0 string) (string, error) {
	cli := xapi.NewClientFor(s.sess, xapi.Account{AuthToken: authToken, CT0: ct0})
	u, err := cli.Me()
	if err != nil {
		return "", err
	}
	if u.ScreenName == "" {
		return "", errNoHandle
	}
	return u.ScreenName, nil
}
