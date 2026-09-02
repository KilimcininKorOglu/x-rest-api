package server

import (
	"fmt"
	"sync"
	"time"

	"x-rest-api/internal/store"
	"x-rest-api/internal/xapi"
)

// cooldown durations applied when an account trips an upstream wall on one op.
const (
	cooldownRateLimit = 15 * time.Minute // 429
	cooldownNotFound  = 5 * time.Minute  // 404 under load
)

// Pool hands out enabled accounts round-robin for read ops, skipping any account
// that is locked (cooling down) for the requested op. x.com rate-limits each op
// separately, so locks are per-op, not per-account.
type Pool struct {
	st  *store.Store
	mu  sync.Mutex
	idx int
}

// NewPool builds a rotation pool backed by the store.
func NewPool(st *store.Store) *Pool { return &Pool{st: st} }

// Next returns the next account available for op, round-robin.
func (p *Pool) Next(op string) (store.Account, error) {
	avail, err := p.st.ListAvailableAccountsForOp(op)
	if err != nil {
		return store.Account{}, err
	}
	avail = p.underDailyCap(avail)
	if len(avail) == 0 {
		return store.Account{}, fmt.Errorf("no available accounts for %s (all disabled, cooling down, or over the daily cap)", op)
	}
	p.mu.Lock()
	a := avail[p.idx%len(avail)]
	p.idx++
	p.mu.Unlock()
	return a, nil
}

// underDailyCap drops accounts that already hit the per-day request cap today.
// A limit of 0 disables the cap and returns the list unchanged.
func (p *Pool) underDailyCap(accts []store.Account) []store.Account {
	limit := p.st.GetSettingInt(store.SettingDailyRequestLimit, 0)
	if limit <= 0 {
		return accts
	}
	today := store.DailyStamp(time.Now())
	out := accts[:0:0]
	for _, a := range accts {
		if a.DailyDate == today && a.DailyCount >= limit {
			continue
		}
		out = append(out, a)
	}
	return out
}

// Fail locks an account for one op when it hits a rate-limit-style wall, so
// rotation skips it for that op until it recovers (other ops stay usable).
func (p *Pool) Fail(id int64, op string, status int) {
	var d time.Duration
	switch status {
	case 429:
		d = cooldownRateLimit
	case 404:
		d = cooldownNotFound
	default:
		return
	}
	_ = p.st.LockAccountOp(id, op, time.Now().Add(d))
}

// Observe applies the upstream rate-limit headers: when an account is out of
// budget for op, lock it until the exact reset time so rotation skips it precisely.
func (p *Pool) Observe(id int64, op string, rl *xapi.RateLimit) {
	if rl == nil || rl.Remaining > 0 || rl.Reset == 0 {
		return
	}
	reset := time.Unix(rl.Reset, 0)
	if reset.After(time.Now()) {
		_ = p.st.LockAccountOp(id, op, reset)
	}
}

// toXAPI converts a stored account into the transport account.
func toXAPI(a store.Account) xapi.Account {
	return xapi.Account{ID: a.ID, Label: a.Label, AuthToken: a.AuthToken, CT0: a.CT0}
}
