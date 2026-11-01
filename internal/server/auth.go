package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/mansus-org/myself/internal/brain"
)

// Access is what an endpoint needs from the caller's token.
type Access int

const (
	// Read: any valid token.
	Read Access = iota
	// Write: a write-scope token or the admin token.
	Write
	// Admin: only the admin token.
	Admin
)

// Identity is the caller, as GET /api/v1/whoami returns it.
type Identity struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
	Admin bool   `json:"admin"`
}

// AdminName is the identity name of the admin token.
const AdminName = "admin"

// IdentityFrom returns the caller of a request that went through Auth.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	return id, ok
}

// authError is an HTTP error that is not a brain exit error: 401, 403, 503.
// Its exit_code is 3, as for a store the caller cannot read or write.
type authError struct {
	status int
	msg    string
}

func (e *authError) Error() string { return e.msg }

var errDisabled = &authError{http.StatusServiceUnavailable,
	"the API is disabled: MYSELF_ADMIN_TOKEN_SHA256 is not set on the server"}

// Auth wraps h so it only runs for a bearer token with the given access, and
// puts the caller's Identity in the request context. Without an admin hash
// configured it answers 503 to everything.
func (s *Server) Auth(need Access, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := s.authenticate(r)
		if err == nil {
			err = authorize(id, need)
		}
		if err != nil {
			WriteError(w, err)
			return
		}
		if rl, ok := r.Context().Value(logKey).(*reqLog); ok {
			rl.name = id.Name
		}
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey, id)))
	})
}

func (s *Server) authenticate(r *http.Request) (Identity, error) {
	if s.adminHash == "" {
		return Identity{}, errDisabled
	}
	h := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(h, "Bearer ")
	token = strings.TrimSpace(token)
	if !ok || token == "" {
		return Identity{}, &authError{http.StatusUnauthorized, "missing token: send the header Authorization: Bearer <token>"}
	}
	if subtle.ConstantTimeCompare([]byte(HashToken(token)), []byte(s.adminHash)) == 1 {
		return Identity{Name: AdminName, Scope: ScopeWrite, Admin: true}, nil
	}
	info, ok, err := s.tokens.Check(token, s.now())
	if err != nil {
		s.log.Printf("tokens: saving last_used: %v", err)
	}
	if !ok {
		return Identity{}, &authError{http.StatusUnauthorized, "invalid or revoked token"}
	}
	return Identity{Name: info.Name, Scope: info.Scope}, nil
}

func authorize(id Identity, need Access) error {
	switch {
	case need == Admin && !id.Admin:
		return &authError{http.StatusForbidden, "this endpoint needs the admin token"}
	case need == Write && id.Scope != ScopeWrite:
		return &authError{http.StatusForbidden, "token " + id.Name + " is read-only; this endpoint needs a write token"}
	}
	return nil
}

// errorBody is the body of every API error.
type errorBody struct {
	Error    string `json:"error"`
	ExitCode int    `json:"exit_code"`
}

// WriteError writes err as {"error","exit_code"} with the HTTP status of
// its exit code: 1 -> 400, 2 -> 404, 3 -> 500; auth errors keep their own
// status (401, 403, 503) with exit code 3.
func WriteError(w http.ResponseWriter, err error) {
	var ae *authError
	if errors.As(err, &ae) {
		if ae.status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="myself"`)
		}
		WriteJSON(w, ae.status, errorBody{ae.msg, brain.ExitStore}, nil)
		return
	}
	code := brain.Code(err)
	status := http.StatusBadRequest
	switch code {
	case brain.ExitNotFound:
		status = http.StatusNotFound
	case brain.ExitStore:
		status = http.StatusInternalServerError
	}
	WriteJSON(w, status, errorBody{err.Error(), code}, nil)
}
