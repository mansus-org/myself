package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
)

// Token scopes. Read covers the GET endpoints, write adds remember and forget.
const (
	ScopeRead  = "read"
	ScopeWrite = "write"
)

// TokenPrefix starts every agent token.
const TokenPrefix = "mys_"

// lastUsedEvery limits how often last_used is written.
const lastUsedEvery = time.Minute

// TokenInfo is a token as listed by GET /api/v1/tokens. The token itself is
// never stored or listed, only its sha256.
type TokenInfo struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Scope    string  `json:"scope"`
	Created  string  `json:"created"`
	LastUsed *string `json:"last_used"`
}

type storedToken struct {
	TokenInfo
	SHA256 string `json:"sha256"`
}

type tokenFile struct {
	Tokens []storedToken `json:"tokens"`
}

// Tokens is the set of agent tokens, kept in memory and persisted to
// tokens.json (0600, atomic rename, under a flock on tokens.json.lock).
// One server process owns the file.
type Tokens struct {
	path string

	mu   sync.Mutex
	list []storedToken
}

// OpenTokens reads path; a missing file is an empty set.
func OpenTokens(path string) (*Tokens, error) {
	t := &Tokens{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	var f tokenFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, errors.New(path + ": " + err.Error())
	}
	t.list = f.Tokens
	return t, nil
}

// HashToken returns the lowercase hex sha256 of token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// List returns every token, oldest first.
func (t *Tokens) List() []TokenInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TokenInfo, len(t.list))
	for i, s := range t.list {
		out[i] = s.TokenInfo
	}
	return out
}

// Create adds a token and returns it with its plaintext, which is not kept.
func (t *Tokens) Create(name, scope string, now time.Time) (TokenInfo, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return TokenInfo{}, "", brain.Usagef("empty token name. give a name, such as the agent's")
	}
	if len(name) > 100 {
		return TokenInfo{}, "", brain.Usagef("token name is longer than 100 characters")
	}
	if scope != ScopeRead && scope != ScopeWrite {
		return TokenInfo{}, "", brain.Usagef("scope must be %q or %q", ScopeRead, ScopeWrite)
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return TokenInfo{}, "", brain.StoreErr(err)
	}
	plain := TokenPrefix + hex.EncodeToString(raw[:])
	st := storedToken{
		TokenInfo: TokenInfo{ID: model.NewUUID(), Name: name, Scope: scope, Created: model.FormatTime(now)},
		SHA256:    HashToken(plain),
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	next := append(append([]storedToken(nil), t.list...), st)
	if err := t.save(next); err != nil {
		return TokenInfo{}, "", brain.StoreErr(err)
	}
	t.list = next
	return st.TokenInfo, plain, nil
}

// Delete revokes the token with id.
func (t *Tokens) Delete(id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	next := make([]storedToken, 0, len(t.list))
	for _, s := range t.list {
		if s.ID != id {
			next = append(next, s)
		}
	}
	if len(next) == len(t.list) {
		return brain.NotFoundf("unknown token %q", id)
	}
	if err := t.save(next); err != nil {
		return brain.StoreErr(err)
	}
	t.list = next
	return nil
}

// Check returns the token whose hash matches token, updating last_used at
// most once a minute. A failure to persist last_used is returned as err
// with ok still true, for the caller to log.
func (t *Tokens) Check(token string, now time.Time) (info TokenInfo, ok bool, err error) {
	h := []byte(HashToken(token))
	t.mu.Lock()
	defer t.mu.Unlock()
	idx := -1
	for i, s := range t.list {
		if subtle.ConstantTimeCompare(h, []byte(s.SHA256)) == 1 {
			idx = i
		}
	}
	if idx < 0 {
		return TokenInfo{}, false, nil
	}
	s := &t.list[idx]
	if s.LastUsed == nil || lastUsedStale(*s.LastUsed, now) {
		ts := model.FormatTime(now)
		s.LastUsed = &ts
		err = t.save(t.list)
	}
	return s.TokenInfo, true, err
}

func lastUsedStale(last string, now time.Time) bool {
	t, err := time.Parse(time.RFC3339, last)
	return err != nil || now.Sub(t) >= lastUsedEvery
}

// save writes list to a temp file and renames it over tokens.json, under
// the flock. The caller holds t.mu.
func (t *Tokens) save(list []storedToken) error {
	if list == nil {
		list = []storedToken{}
	}
	b, err := json.MarshalIndent(tokenFile{list}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(t.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	unlock, err := store.Lock(t.path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	f, err := os.CreateTemp(dir, ".tokens-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after the rename
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, t.path)
}
