package server

import (
	"context"
	"net/http"

	"x-rest-api/internal/store"
)

type ctxKey int

const reqInfoKey ctxKey = iota

// reqInfo carries per-request data that the logging middleware records after the
// handler runs. Middleware creates it; the API-key middleware and handlers fill it.
type reqInfo struct {
	apiKey         *store.APIKey
	accountID      *int64
	upstreamStatus *int
	errMsg         string
}

// withReqInfo attaches a fresh reqInfo to the request context.
func withReqInfo(r *http.Request) (*http.Request, *reqInfo) {
	ri := &reqInfo{}
	return r.WithContext(context.WithValue(r.Context(), reqInfoKey, ri)), ri
}

// getReqInfo returns the request's reqInfo, or nil.
func getReqInfo(r *http.Request) *reqInfo {
	ri, _ := r.Context().Value(reqInfoKey).(*reqInfo)
	return ri
}

// setAccount records which account served the request (for logging).
func setAccount(r *http.Request, id int64) {
	if ri := getReqInfo(r); ri != nil {
		ri.accountID = &id
	}
}
