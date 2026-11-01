package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/store"
)

// Request body limits.
const (
	maxBody   = 1 << 20  // remember, forget, tokens
	maxImport = 64 << 20 // import
)

func (s *Server) registerAPI() {
	api := func(pattern string, need Access, h http.HandlerFunc) {
		s.mux.Handle(pattern, s.Auth(need, h))
	}
	api("GET /api/v1/whoami", Read, s.whoami)
	api("POST /api/v1/remember", Write, s.remember)
	api("POST /api/v1/forget", Write, s.forget)
	api("GET /api/v1/describe/{entity...}", Read, s.describe)
	api("GET /api/v1/ls", Read, s.ls)
	api("GET /api/v1/relations", Read, s.relations)
	api("GET /api/v1/search", Read, s.search)
	api("GET /api/v1/path", Read, s.path)
	api("GET /api/v1/log", Read, s.logRecords)
	api("GET /api/v1/history/{subject...}", Read, s.history)
	api("GET /api/v1/prime", Read, s.prime)
	api("GET /api/v1/dream", Read, s.dream)
	api("GET /api/v1/graph", Read, s.graph)
	api("GET /api/v1/export.jsonl", Read, s.exportJSONL)
	api("POST /api/v1/import", Admin, s.importJSONL)
	api("GET /api/v1/tokens", Admin, s.listTokens)
	api("POST /api/v1/tokens", Admin, s.createToken)
	api("DELETE /api/v1/tokens/{id}", Admin, s.deleteToken)
	// Anything else under /api is a JSON 404 (or 503 while disabled).
	s.mux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.adminHash == "" {
			WriteError(w, errDisabled)
			return
		}
		WriteError(w, brain.NotFoundf("no endpoint %s %s", r.Method, r.URL.Path))
	}))
}

// encodeJSON is v as compact JSON without HTML escaping. Non-empty warnings
// are added to an object as a last "warnings" field.
func encodeJSON(v any, warnings []string) ([]byte, error) {
	b, err := model.Marshal(v)
	if err == nil && len(warnings) > 0 && len(b) > 1 && b[0] == '{' {
		ws, _ := model.Marshal(warnings)
		sep := ","
		if string(b) == "{}" {
			sep = ""
		}
		b = append(append(append(b[:len(b)-1:len(b)-1], sep+`"warnings":`...), ws...), '}')
	}
	return b, err
}

// WriteJSON writes v (compact, no HTML escaping) with status. Non-empty
// warnings are added to an object body as a last "warnings" field.
func WriteJSON(w http.ResponseWriter, status int, v any, warnings []string) {
	b, err := encodeJSON(v, warnings)
	if err != nil {
		status, b = http.StatusInternalServerError, []byte(`{"error":"encoding the response failed","exit_code":3}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(append(b, '\n'))
}

// reply writes a brain result or error. st's warnings go into the body.
func reply(w http.ResponseWriter, st *RequestStore, v any, err error) {
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, v, st.Warnings())
}

// decode reads a JSON body into v, rejecting unknown fields and trailing data.
func decode(r *http.Request, w http.ResponseWriter, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return brain.Usagef("bad request body: %v", err)
	}
	if dec.More() {
		return brain.Usagef("bad request body: more than one JSON value")
	}
	return nil
}

func optString(q url.Values, name string) *string {
	if !q.Has(name) {
		return nil
	}
	v := q.Get(name)
	return &v
}

func optInt(q url.Values, name string) (*int, error) {
	if !q.Has(name) {
		return nil, nil
	}
	n, err := strconv.Atoi(q.Get(name))
	if err != nil {
		return nil, brain.Usagef("invalid argument %q for %s: want an integer", q.Get(name), name)
	}
	return &n, nil
}

func intOr(q url.Values, name string, def int) (int, error) {
	n, err := optInt(q, name)
	if err != nil || n == nil {
		return def, err
	}
	return *n, nil
}

func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	id, _ := IdentityFrom(r.Context())
	WriteJSON(w, http.StatusOK, id, nil)
}

type rememberRequest struct {
	Facts   []brain.Fact `json:"facts"`
	Replace bool         `json:"replace"`
}

func (s *Server) remember(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	var req rememberRequest
	if err := decode(r, w, &req); err != nil {
		WriteError(w, err)
		return
	}
	if len(req.Facts) == 0 {
		WriteError(w, brain.Usagef("remember needs at least one fact"))
		return
	}
	defer s.LockWrites()()
	res, err := brain.Remember(st, req.Facts, req.Replace, s.now())
	reply(w, st, res, err)
}

type forgetRequest struct {
	UUID     string  `json:"uuid"`
	Subject  string  `json:"subject"`
	Relation string  `json:"relation"`
	Object   *string `json:"object"`
	Reason   string  `json:"reason"`
}

func (s *Server) forget(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	var req forgetRequest
	if err := decode(r, w, &req); err != nil {
		WriteError(w, err)
		return
	}
	if req.UUID == "" && req.Subject == "" && req.Relation == "" {
		WriteError(w, brain.Usagef("%s", brain.ForgetUsage))
		return
	}
	defer s.LockWrites()()
	res, err := brain.Forget(st, brain.ForgetArgs(req), s.now())
	reply(w, st, res, err)
}

func (s *Server) describe(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	entity := r.PathValue("entity")
	if entity == "" {
		WriteError(w, brain.Usagef("describe needs exactly one <entity>"))
		return
	}
	q := r.URL.Query()
	depth, err := intOr(q, "depth", 0)
	if err != nil {
		WriteError(w, err)
		return
	}
	res, err := brain.Describe(st, entity, depth, optString(q, "at"))
	reply(w, st, res, err)
}

func (s *Server) ls(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	q := r.URL.Query()
	res, err := brain.Ls(st, optString(q, "type"), optString(q, "at"))
	reply(w, st, res, err)
}

func (s *Server) relations(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, brain.Relations(), nil)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	q := r.URL.Query()
	text := q.Get("q")
	if text == "" {
		WriteError(w, brain.Usagef("search needs <text>"))
		return
	}
	n, err := optInt(q, "n")
	if err != nil {
		WriteError(w, err)
		return
	}
	res, err := brain.Search(st, text, optString(q, "at"), n)
	reply(w, st, res, err)
}

func (s *Server) path(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" || to == "" {
		WriteError(w, brain.Usagef("path needs <a> and <b>"))
		return
	}
	maxDepth, err := intOr(q, "max_depth", brain.DefaultMaxDepth)
	if err != nil {
		WriteError(w, err)
		return
	}
	res, err := brain.Path(st, from, to, maxDepth)
	reply(w, st, res, err)
}

func (s *Server) logRecords(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	q := r.URL.Query()
	n, err := optInt(q, "n")
	if err != nil {
		WriteError(w, err)
		return
	}
	res, err := brain.Log(st, optString(q, "since"), n)
	reply(w, st, res, err)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	res, err := brain.History(st, r.PathValue("subject"), optString(r.URL.Query(), "relation"))
	reply(w, st, res, err)
}

func (s *Server) prime(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	budget, err := intOr(r.URL.Query(), "budget", brain.DefaultBudget)
	if err != nil {
		WriteError(w, err)
		return
	}
	res, err := brain.Prime(st, budget)
	reply(w, st, res, err)
}

func (s *Server) dream(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	res, err := brain.Dream(st)
	reply(w, st, res, err)
}

func (s *Server) graph(w http.ResponseWriter, r *http.Request) {
	st := s.Store()
	res, err := brain.GraphView(st, optString(r.URL.Query(), "at"))
	reply(w, st, res, err)
}

func (s *Server) exportJSONL(w http.ResponseWriter, r *http.Request) {
	snap, err := s.Store().Load()
	if err != nil {
		WriteError(w, brain.StoreErr(err))
		return
	}
	var buf bytes.Buffer
	for _, rec := range snap.Records {
		b, err := model.Marshal(rec)
		if err != nil {
			WriteError(w, brain.StoreErr(err))
			return
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", `attachment; filename="brain.jsonl"`)
	w.Write(buf.Bytes())
}

// ImportResult is the body of POST /api/v1/import.
type ImportResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

func (s *Server) importJSONL(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImport))
	if err != nil {
		WriteError(w, brain.Usagef("reading the body: %v", err))
		return
	}
	recs, err := parseImport(string(body))
	if err != nil {
		WriteError(w, err)
		return
	}
	defer s.LockWrites()()
	st := s.Store()
	snap, err := st.Load()
	if err != nil {
		WriteError(w, brain.StoreErr(err))
		return
	}
	seen := make(map[string]bool, len(snap.Records)+len(recs))
	for _, rec := range snap.Records {
		seen[rec.UUID] = true
	}
	var fresh []model.Record
	for _, rec := range recs {
		if !seen[rec.UUID] {
			seen[rec.UUID] = true
			fresh = append(fresh, rec)
		}
	}
	sort.SliceStable(fresh, func(i, j int) bool { return fresh[i].UUID < fresh[j].UUID })
	if len(fresh) > 0 {
		if err := st.Append(fresh...); err != nil {
			WriteError(w, brain.StoreErr(err))
			return
		}
	}
	WriteJSON(w, http.StatusOK, ImportResult{len(fresh), len(recs) - len(fresh)}, st.Warnings())
}

// parseImport validates every non-blank line of a JSONL body (the last line
// may lack its newline). Any bad line rejects the whole import.
func parseImport(body string) ([]model.Record, error) {
	var recs []model.Record
	for i, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		rec, ok := store.DecodeLine(line)
		if !ok {
			return nil, brain.Usagef("line %d: not a record", i+1)
		}
		if why := invalid(rec); why != "" {
			return nil, brain.Usagef("line %d: %s", i+1, why)
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

func invalid(r model.Record) string {
	if !model.IsUUID(r.UUID) {
		return fmt.Sprintf("uuid %q is not a uuid", r.UUID)
	}
	if _, err := time.Parse(time.RFC3339, r.Time); err != nil {
		return fmt.Sprintf("time %q is not RFC 3339", r.Time)
	}
	if r.IsForget() {
		if !model.IsUUID(r.Subject) {
			return fmt.Sprintf("forget subject %q is not a uuid", r.Subject)
		}
		return ""
	}
	if rel, err := model.ParseRelation(r.Relation); err != nil || rel != r.Relation {
		return fmt.Sprintf("relation %q is not one of: %s", r.Relation, strings.Join(model.Relations(), ", "))
	}
	if strings.TrimSpace(r.Subject) == "" {
		return "empty subject"
	}
	if strings.TrimSpace(r.Object) == "" {
		return "empty object"
	}
	return ""
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, s.tokens.List(), nil)
}

type createTokenRequest struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

// CreatedToken is the body of POST /api/v1/tokens: the only time the
// plaintext token is returned.
type CreatedToken struct {
	TokenInfo
	Token string `json:"token"`
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var req createTokenRequest
	if err := decode(r, w, &req); err != nil {
		WriteError(w, err)
		return
	}
	info, plain, err := s.tokens.Create(req.Name, req.Scope, s.now())
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, CreatedToken{info, plain}, nil)
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.tokens.Delete(id); err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, struct {
		Deleted string `json:"deleted"`
	}{id}, nil)
}
