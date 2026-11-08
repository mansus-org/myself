package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The local login key: how `myself graph --web` reuses a server that is
// already running instead of telling the user to go set up tokens.
//
// A server writes LocalInfo next to the store it serves, 0600, and removes it
// when it stops. The key in it only ever opens POST /api/v1/login-codes, only
// for a caller on loopback, and only for as long as that server runs. Whoever
// can read the file can already read the store next to it, so it hands out
// nothing the same user did not have. It is never printed and never an argument,
// so it stays out of shell history and `ps` output.

// LocalKeyName is the name a login made with the local key is signed in as.
const LocalKeyName = "local"

// localKeyPrefix starts the local login key of a running server.
const localKeyPrefix = "myslk_"

// NewLocalKey returns a fresh local login key (see LocalInfo).
func NewLocalKey() (string, error) { return randomSecret(localKeyPrefix) }

// LocalInfo is what a running server leaves next to its store, so that another
// process on the same machine can find out what it serves and ask it for a
// login code.
type LocalInfo struct {
	Store   string `json:"store"`   // the store file it serves, absolute
	Addr    string `json:"addr"`    // the address it listens on
	PID     int    `json:"pid"`     // its process id
	Key     string `json:"key"`     // its local login key
	Started string `json:"started"` // when it started
}

// LocalInfoPath is the file of the server on port serving storeFile. One file
// per port, so servers on different stores and ports do not overwrite
// each other, and a lookup for a store that is not served there finds nothing.
func LocalInfoPath(storeFile string, port int) string {
	dir := filepath.Dir(absPath(storeFile))
	return filepath.Join(dir, fmt.Sprintf(".myself-server-%d.json", port))
}

// WriteLocalInfo records that a server has started, overwriting the leftover of
// one that died, and returns the func that removes the record again.
func WriteLocalInfo(info LocalInfo) (remove func(), err error) {
	info.Store = absPath(info.Store)
	if info.Started == "" {
		info.Started = time.Now().UTC().Format(time.RFC3339)
	}
	path := LocalInfoPath(info.Store, portOf(info.Addr))
	b, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	return func() { os.Remove(path) }, nil
}

// ReadLocalInfo returns the record of the server on port serving storeFile.
// It is fs.ErrNotExist when there is none: no server, or one old enough not to
// have written it.
func ReadLocalInfo(storeFile string, port int) (LocalInfo, error) {
	var info LocalInfo
	b, err := os.ReadFile(LocalInfoPath(storeFile, port))
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(b, &info); err != nil {
		return info, fmt.Errorf("cannot read the record of the myself server on port %d: %w", port, err)
	}
	return info, nil
}

// localOrAuth is the auth of an endpoint that the local login key also opens,
// and only from a loopback address: any other caller goes through Auth
// unchanged. The key is the secret in LocalInfo, so it opens no read of the
// store and no write, only the minting of a login code for the user who can
// already read that file.
func (s *Server) localOrAuth(need Access, h http.HandlerFunc) http.Handler {
	auth := s.Auth(need, h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The key needs an admin hash to buy anything: without one the API is
		// disabled and the code could never be redeemed.
		if s.adminHash == "" || s.localKey == "" || !localPeer(r) || !keyMatch(r, s.localKey) {
			auth.ServeHTTP(w, r)
			return
		}
		if rl, ok := r.Context().Value(logKey).(*reqLog); ok {
			rl.name = LocalKeyName
		}
		id := Identity{Name: LocalKeyName, Scope: ScopeWrite, Admin: true}
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey, id)))
	})
}

// keyMatch reports whether the bearer token of r is key, in constant time.
func keyMatch(r *http.Request, key string) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(key)) == 1
}

// localPeer reports whether r came in on a loopback address. Behind a proxy on
// the same host this is true of every caller, which is why the key itself, not
// the address, is what has to be secret.
func localPeer(r *http.Request) bool {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

// portOf is the port of a host:port address, or 0 when it has none.
func portOf(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return 0
	}
	return n
}

// absPath is path made absolute, or path unchanged when that fails.
func absPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
