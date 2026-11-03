package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
)

// One-time login codes and the sessions they buy. Both live in memory only
// (a restart signs everyone out) and are keyed by the sha256 of the secret.
const (
	loginCodePrefix = "mysl_"
	sessionPrefix   = "myss_"

	// LoginCodeTTL is how long a login code can be redeemed.
	LoginCodeTTL = 2 * time.Minute
	// SessionTTL is how long a session token works.
	SessionTTL = 12 * time.Hour

	// maxPending bounds the codes and sessions held, so the endpoints cannot
	// grow the server's memory without limit.
	maxPending = 1000
)

type grant struct {
	id      Identity
	expires time.Time
}

// logins holds the pending codes and the live sessions.
type logins struct {
	mu       sync.Mutex
	codes    map[string]grant // sha256 of the code
	sessions map[string]grant // sha256 of the session token
}

func newLogins() *logins {
	return &logins{codes: map[string]grant{}, sessions: map[string]grant{}}
}

func randomSecret(prefix string) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw[:]), nil
}

// prune drops what has expired. The caller holds l.mu.
func (l *logins) prune(now time.Time) {
	for h, g := range l.codes {
		if !now.Before(g.expires) {
			delete(l.codes, h)
		}
	}
	for h, g := range l.sessions {
		if !now.Before(g.expires) {
			delete(l.sessions, h)
		}
	}
}

// lookup finds hash in m by comparing every entry in constant time.
func lookup(m map[string]grant, hash string) (string, bool) {
	found := ""
	for h := range m {
		if subtle.ConstantTimeCompare([]byte(h), []byte(hash)) == 1 {
			found = h
		}
	}
	return found, found != ""
}

// NewLoginCode makes a single-use code that signs in as id for LoginCodeTTL.
func (s *Server) NewLoginCode(id Identity) (code string, expires time.Time, err error) {
	code, err = randomSecret(loginCodePrefix)
	if err != nil {
		return "", time.Time{}, brain.StoreErr(err)
	}
	now := s.now()
	l := s.logins
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	if len(l.codes) >= maxPending {
		return "", time.Time{}, errTooManyLogins
	}
	expires = now.Add(LoginCodeTTL)
	l.codes[HashToken(code)] = grant{id, expires}
	return code, expires, nil
}

var errTooManyLogins = &authError{http.StatusTooManyRequests, "too many pending login codes; wait a couple of minutes"}

// redeem trades a code for a session token. The code is consumed whether or
// not it was still valid.
func (s *Server) redeem(code string) (token string, g grant, ok bool, err error) {
	now := s.now()
	l := s.logins
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	h, found := lookup(l.codes, HashToken(code))
	if !found {
		return "", grant{}, false, nil
	}
	g = l.codes[h]
	delete(l.codes, h)
	if len(l.sessions) >= maxPending {
		return "", grant{}, false, errTooManyLogins
	}
	token, err = randomSecret(sessionPrefix)
	if err != nil {
		return "", grant{}, false, brain.StoreErr(err)
	}
	g.expires = now.Add(SessionTTL)
	l.sessions[HashToken(token)] = g
	return token, g, true, nil
}

// session returns the identity of a live session token.
func (s *Server) session(token string) (Identity, bool) {
	now := s.now()
	l := s.logins
	l.mu.Lock()
	defer l.mu.Unlock()
	h, found := lookup(l.sessions, HashToken(token))
	if !found {
		return Identity{}, false
	}
	g := l.sessions[h]
	if !now.Before(g.expires) {
		delete(l.sessions, h)
		return Identity{}, false
	}
	return g.id, true
}

type loginCodeReply struct {
	Code    string `json:"code"`
	Expires string `json:"expires"`
}

// createLoginCode: any valid token gets a code that signs in as itself.
func (s *Server) createLoginCode(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	code, expires, err := s.NewLoginCode(id)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, loginCodeReply{code, model.FormatTime(expires)}, nil)
}

type redeemRequest struct {
	Code string `json:"code"`
}

type sessionReply struct {
	Token   string `json:"token"`
	Expires string `json:"expires"`
	Identity
}

// redeemLoginCode is the one unauthenticated API call: the code is the credential.
func (s *Server) redeemLoginCode(w http.ResponseWriter, r *http.Request) {
	if s.adminHash == "" {
		WriteError(w, errDisabled)
		return
	}
	var req redeemRequest
	if err := decode(r, w, &req); err != nil {
		WriteError(w, err)
		return
	}
	token, g, ok, err := s.redeem(req.Code)
	if err == nil && !ok {
		err = &authError{http.StatusUnauthorized, "invalid, used or expired login code"}
	}
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, sessionReply{token, model.FormatTime(g.expires), g.id}, nil)
}
