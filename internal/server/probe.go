package server

// On-demand account health check. Account validation is otherwise passive: a
// dead cookie only surfaces when a real read fails. This probe lets the operator
// ask the question directly, before a user request hits the bad account.

import "x-rest-api/internal/xapi"

// probeOp is the op the probe runs. Viewer is account-scoped, so a success
// proves the cookies identify a logged-in account rather than only that a public
// read works.
const probeOp = "Viewer"

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
