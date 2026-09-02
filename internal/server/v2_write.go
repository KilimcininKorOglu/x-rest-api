package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"x-rest-api/internal/apiv2"
	"x-rest-api/internal/store"
	"x-rest-api/internal/xapi"
)

// writeGuardV2 is writeAllowed with the v2 problem-details response on refusal.
func (s *Server) writeGuardV2(w http.ResponseWriter, r *http.Request, op string) (store.Account, bool) {
	acct, err := s.writeAllowed(r, op)
	switch {
	case err == nil:
		return acct, true
	case errors.Is(err, errWritesDisabled):
		recordErr(r, errWritesDisabled)
		writeJSON(w, http.StatusForbidden, apiv2.Invalid("", "writes are disabled (enable them in the admin panel)"))
	case errors.Is(err, errWriteForbidden):
		writeJSON(w, http.StatusForbidden, apiv2.Invalid("", "this API key is not allowed to write"))
	default:
		s.failPickV2(w, r, err)
	}
	return store.Account{}, false
}

// failWriteV2 applies the shared write side effects and writes the v2 error body.
func (s *Server) failWriteV2(w http.ResponseWriter, r *http.Request, id int64, op string, err error) {
	s.applyWriteEffect(id, op, err)
	writeV2Fail(w, r, err)
}

// v2WriteResult runs a no-result v2 write (like/retweet/follow/delete) and writes a
// fixed success data object, handling the write gate, per-op observe and error
// mapping uniformly.
func (s *Server) v2WriteResult(w http.ResponseWriter, r *http.Request, op string, act func(*xapi.XClient) error, success map[string]any) {
	acct, ok := s.writeGuardV2(w, r, op)
	if !ok {
		return
	}
	cli := s.clientFor(acct)
	err := act(cli)
	s.pool.Observe(acct.ID, op, cli.RateLimit())
	if err != nil {
		s.failWriteV2(w, r, acct.ID, op, err)
		return
	}
	writeJSON(w, http.StatusOK, apiv2.Envelope{Data: success})
}

// decodeV2Body decodes a JSON request body, writing a v2 invalid-request on error.
// An empty body decodes to the zero value without error, so optional-body writes
// still work.
func decodeV2Body(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, apiv2.Invalid("", "invalid JSON body"))
		return false
	}
	return true
}
