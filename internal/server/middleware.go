package server

import (
	"net"
	"net/http"
	"strings"
	"time"

	"x-rest-api/internal/store"
)

// statusRecorder captures the response status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// requestLog wraps /v1 handlers, timing them and recording one row per request.
func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, ri := withReqInfo(r)
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		start := time.Now()
		next.ServeHTTP(rec, r)
		s.saveLog(r, ri, rec.status, time.Since(start))
	})
}

// saveLog persists one request row from the collected reqInfo.
func (s *Server) saveLog(r *http.Request, ri *reqInfo, status int, dur time.Duration) {
	l := store.RequestLog{
		Method:     r.Method,
		Path:       r.URL.Path,
		Query:      r.URL.RawQuery,
		Status:     status,
		DurationMS: dur.Milliseconds(),
		RemoteIP:   clientIP(r),
	}
	if ri != nil {
		if ri.apiKey != nil {
			l.APIKeyID = &ri.apiKey.ID
		}
		l.AccountID = ri.accountID
		l.UpstreamStatus = ri.upstreamStatus
		l.Error = ri.errMsg
	}
	if err := s.store.InsertLog(l); err != nil {
		// Logging must not break the request path; report and continue.
		println("requestLog: insert:", err.Error())
	}
}

// apiKeyAuth requires a valid, enabled Bearer key and records it on the request.
func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(r.Header.Get("Authorization"))
		const p = "Bearer "
		if !strings.HasPrefix(raw, p) {
			writeError(w, http.StatusUnauthorized, "missing Authorization Bearer key")
			return
		}
		key, err := s.store.GetAPIKeyByKey(strings.TrimSpace(raw[len(p):]))
		if err != nil || !key.Enabled {
			writeError(w, http.StatusUnauthorized, "invalid or disabled API key")
			return
		}
		if ri := getReqInfo(r); ri != nil {
			ri.apiKey = &key
		}
		_ = s.store.MarkAPIKeyUsed(key.ID)
		next.ServeHTTP(w, r)
	})
}

// clientIP extracts the remote IP without the port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
