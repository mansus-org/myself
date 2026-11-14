// Package neo4j is an optional brain.Store that keeps the event log in Neo4j
// instead of the JSONL file (SPEC section 1.4, 10).
//
// It stores the log, not a derived view of it: every record becomes one
// (:Record) node with the five fields of SPEC section 1 and nothing else,
// retraction records included. Liveness, `--at` and `history` stay in
// internal/brain, computed from the records exactly as they are for the file,
// so switching backends cannot change what a command answers. That is the whole
// point of the seam: brain.Store is three methods, and everything above it is
// unaware of where records came from.
//
// It talks to the HTTP transaction endpoint with net/http only, so the binary
// gains no driver dependency and no Bolt port to reach — the same endpoint the
// query catalog uses, and the one that works through the cluster's ingress.
package neo4j

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
)

// Environment variables selecting the backend. The backend is on when
// EnvURL is non-empty; every other setting has a default.
const (
	EnvURL      = "MYSELF_NEO4J_URL"      // e.g. http://neo4j.neo4j.svc.cluster.local:7474
	EnvUser     = "MYSELF_NEO4J_USER"     // default neo4j
	EnvPassword = "MYSELF_NEO4J_PASSWORD" // required; never a flag, it would show in ps
	EnvDatabase = "MYSELF_NEO4J_DATABASE" // default neo4j
	EnvTimeout  = "MYSELF_NEO4J_TIMEOUT"  // seconds or a Go duration; default 10s

	// DefaultDatabase is the database used when EnvDatabase is unset. Community
	// edition has one database, so this is rarely more than a formality.
	DefaultDatabase = "neo4j"
	DefaultUser     = "neo4j"
	DefaultTimeout  = 10 * time.Second
)

// Config is a resolved backend setting.
type Config struct {
	Base     string // scheme://host[:port], no trailing slash, no /db path
	User     string
	Password string
	Database string
	Timeout  time.Duration
}

// FromEnv reads the backend settings. ok is false when the backend is not
// selected, in which case the caller uses the file store as before.
func FromEnv(getenv func(string) string) (cfg Config, ok bool, err error) {
	base := strings.TrimRight(strings.TrimSpace(getenv(EnvURL)), "/")
	if base == "" {
		return Config{}, false, nil
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return Config{}, false, fmt.Errorf("%s %q: want an http(s) URL", EnvURL, getenv(EnvURL))
	}
	cfg = Config{
		Base:     base,
		User:     strings.TrimSpace(getenv(EnvUser)),
		Password: getenv(EnvPassword),
		Database: strings.TrimSpace(getenv(EnvDatabase)),
		Timeout:  DefaultTimeout,
	}
	if cfg.User == "" {
		cfg.User = DefaultUser
	}
	if cfg.Database == "" {
		cfg.Database = DefaultDatabase
	}
	if cfg.Password == "" {
		return Config{}, false, fmt.Errorf("%s is not set; %s selects the Neo4j backend, which needs a password: %w", EnvPassword, EnvURL, ErrNoPassword)
	}
	if s := strings.TrimSpace(getenv(EnvTimeout)); s != "" {
		if d, perr := time.ParseDuration(s); perr == nil && d > 0 {
			cfg.Timeout = d
		} else if f, perr := strconv.ParseFloat(s, 64); perr == nil && f > 0 {
			cfg.Timeout = time.Duration(f * float64(time.Second))
		} else {
			return Config{}, false, fmt.Errorf("%s %q: want seconds (30) or a duration (500ms, 1m)", EnvTimeout, s)
		}
	}
	return cfg, true, nil
}

// Store is a brain.Store over one Neo4j database. It is safe for concurrent
// use; the HTTP client and the settings are read-only after New.
type Store struct {
	cfg    Config
	client *http.Client
	warn   io.Writer
}

// New returns a Store for cfg. warn receives brain.Store warnings; nil
// discards them. Nothing is read or written until the first call.
func New(cfg Config, warn io.Writer) *Store {
	if warn == nil {
		warn = io.Discard
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Store{cfg: cfg, client: &http.Client{Timeout: timeout}, warn: warn}
}

// String names the backend for logs and for `serve`'s startup line. It never
// includes the password.
func (s *Store) String() string {
	return fmt.Sprintf("neo4j %s@%s", s.cfg.Database, s.cfg.Base)
}

// schema statements the store relies on. Idempotent, so EnsureSchema is safe on
// every start and safe against a database an operator prepared by hand.
var schema = []string{
	"CREATE CONSTRAINT record_uuid IF NOT EXISTS FOR (r:Record) REQUIRE r.uuid IS UNIQUE",
	"CREATE INDEX record_subject IF NOT EXISTS FOR (r:Record) ON (r.subject)",
}

// EnsureSchema creates the uniqueness constraint that makes uuid an identity
// and the subject index. Call it once at startup; it changes nothing when the
// schema is already there.
func (s *Store) EnsureSchema() error {
	return s.commit(toStatements(schema, nil))
}

// loadStatement reads the whole log in uuid order. brain requires uuid-sorted
// records (SPEC section 4), and a UUIDv7 string sorts lexicographically in the
// same order it sorts temporally, so ORDER BY on the property is enough.
const loadStatement = "MATCH (r:Record) RETURN r.uuid AS uuid, r.subject AS subject, " +
	"r.relation AS relation, r.object AS object, r.time AS time ORDER BY r.uuid"

// Load returns every record, forgets included, as a Snapshot.
func (s *Store) Load() (*brain.Snapshot, error) {
	rows, err := s.rows(loadStatement, nil)
	if err != nil {
		return nil, err
	}
	recs := make([]model.Record, 0, len(rows))
	for _, row := range rows {
		recs = append(recs, model.Record{
			UUID:     str(row["uuid"]),
			Subject:  str(row["subject"]),
			Relation: str(row["relation"]),
			Object:   str(row["object"]),
			Time:     str(row["time"]),
		})
	}
	return brain.NewSnapshot(recs), nil
}

// appendStatement writes a batch in one transaction. MERGE on uuid makes Append
// idempotent, which is what lets `import` union by uuid and lets a retried write
// be harmless — the same guarantee the append-only file gives by construction.
const appendStatement = "UNWIND $rows AS row MERGE (r:Record {uuid: row.uuid}) SET r += row"

// Append writes recs as one transaction, so a batch is all-or-nothing like
// `remember -`. An empty batch writes nothing.
func (s *Store) Append(recs ...model.Record) error {
	if len(recs) == 0 {
		return nil
	}
	rows := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, map[string]any{
			"uuid": r.UUID, "subject": r.Subject, "relation": r.Relation,
			"object": r.Object, "time": r.Time,
		})
	}
	return s.commit([]statement{{
		Statement:  appendStatement,
		Parameters: map[string]any{"rows": rows},
	}})
}

// Warn reports a warning the way the file store does. The Neo4j backend has no
// corrupt lines, so this is only used if a caller has something to say.
func (s *Store) Warn(msg string) {
	if s.warn != nil {
		fmt.Fprintf(s.warn, "warning: %s\n", msg)
	}
}

type statement struct {
	Statement  string         `json:"statement"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type commitBody struct {
	Statements []statement `json:"statements"`
}

type commitReply struct {
	Results []struct {
		Columns []string `json:"columns"`
		Data    []struct {
			Row []json.RawMessage `json:"row"`
		} `json:"data"`
	} `json:"results"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

func toStatements(cypher []string, params map[string]any) []statement {
	out := make([]statement, 0, len(cypher))
	for _, c := range cypher {
		out = append(out, statement{Statement: c, Parameters: params})
	}
	return out
}

// commit posts one /db/{database}/tx/commit request, which is one transaction:
// every statement in it succeeds or none does.
func (s *Store) commit(stmts []statement) error {
	body, err := json.Marshal(commitBody{Statements: stmts})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, s.endpoint(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.cfg.User, s.cfg.Password)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", s, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var reply commitReply
	if jerr := json.Unmarshal(raw, &reply); jerr != nil {
		return fmt.Errorf("%s: unreadable reply (HTTP %d): %v", s, res.StatusCode, jerr)
	}
	if len(reply.Errors) > 0 {
		return fmt.Errorf("%s: %s: %s", s, reply.Errors[0].Code, reply.Errors[0].Message)
	}
	// A 401 arrives with no JSON body, and a 5xx may arrive as HTML, so the
	// status is checked after the decoded errors rather than before them.
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d: %s", s, res.StatusCode, firstLine(string(raw)))
	}
	return nil
}

// rows runs one read statement and returns each row as a map keyed by column.
func (s *Store) rows(cypher string, params map[string]any) ([]map[string]any, error) {
	body, err := json.Marshal(commitBody{Statements: []statement{{Statement: cypher, Parameters: params}}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, s.endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(s.cfg.User, s.cfg.Password)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var reply commitReply
	if jerr := json.Unmarshal(raw, &reply); jerr != nil {
		return nil, fmt.Errorf("%s: unreadable reply (HTTP %d): %v", s, res.StatusCode, jerr)
	}
	if len(reply.Errors) > 0 {
		return nil, fmt.Errorf("%s: %s: %s", s, reply.Errors[0].Code, reply.Errors[0].Message)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d: %s", s, res.StatusCode, firstLine(string(raw)))
	}
	if len(reply.Results) == 0 {
		return nil, nil
	}
	cols := reply.Results[0].Columns
	out := make([]map[string]any, 0, len(reply.Results[0].Data))
	for _, d := range reply.Results[0].Data {
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			if i < len(d.Row) {
				row[c] = decodeJSON(d.Row[i])
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *Store) endpoint() string {
	return s.cfg.Base + "/db/" + s.cfg.Database + "/tx/commit"
}

// decodeJSON turns one row value back into a Go value: a quoted string stays a
// string, a number stays a number, null becomes nil.
func decodeJSON(raw json.RawMessage) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return trimmed
	}
	return v
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ErrNoPassword is returned by FromEnv when the URL is set without a password.
// It exists so a caller can tell "backend not selected" from "backend
// misconfigured" without matching on message text.
var ErrNoPassword = errors.New("neo4j: password not set")
