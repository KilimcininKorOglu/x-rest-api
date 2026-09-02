// Package store is the SQLite persistence layer. All configuration lives here;
// the process environment carries only the listen port. Queries are parameterized.
package store

import "time"

// Account is an x.com session (cookies stored plaintext, by explicit choice).
type Account struct {
	ID            int64
	Label         string
	AuthToken     string
	CT0           string
	Enabled       bool
	Status        string // ok | rate_limited | error
	ErrorMsg      string // reason an account was auto-disabled (ban/auth failure)
	CooldownUntil *time.Time
	LastUsedAt    *time.Time
	DailyCount    int    // requests served today (UTC), for the daily cap
	DailyDate     string // UTC date (YYYY-MM-DD) the DailyCount belongs to
	CreatedAt     time.Time
}

// APIKey is a bearer key clients present to /v1 (stored plaintext, by choice).
type APIKey struct {
	ID             int64
	Name           string
	Key            string
	Enabled        bool
	CanWrite       bool
	BoundAccountID *int64
	LastUsedAt     *time.Time
	CreatedAt      time.Time
}

// AdminUser is a panel operator; the password is bcrypt-hashed.
type AdminUser struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// RequestLog is one recorded /v1 request.
type RequestLog struct {
	ID             int64
	TS             time.Time
	APIKeyID       *int64
	AccountID      *int64
	Method         string
	Path           string
	Query          string
	Status         int
	DurationMS     int64
	UpstreamStatus *int
	Error          string
	RemoteIP       string
}

// LogFilter narrows a request-log listing.
type LogFilter struct {
	Path   string
	Limit  int
	Offset int
}
