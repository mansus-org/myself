package server

import (
	"net/http"
	"time"

	"github.com/mansus-org/myself/internal/brain"
	"github.com/mansus-org/myself/internal/model"
	"github.com/mansus-org/myself/internal/querylog"
)

// This file is the query log: what this server was asked, and what it answered
// (SPEC section 10). Three verbs are recorded — search, recall and describe —
// because those are the questions; everything else that comes in is a write or a
// read of a whole page. It is the server's own record, not the brain's: it says
// who asked, from which token, and what came back. That is why `serve` can be
// started without it.

// logQuery records one query. The log is a convenience and never a reason for a
// query to fail: a write error goes to the request log and the answer still goes
// out to whoever asked.
func (s *Server) logQuery(r *http.Request, verb, text string, res any, err error, took time.Duration) {
	if s.queries.Path() == "" {
		return
	}
	e := querylog.Entry{Verb: verb, Text: text, Took: took.Milliseconds(), At: r.URL.Query().Get("at")}
	if id, ok := IdentityFrom(r.Context()); ok {
		e.Who = id.Name
	}
	if err != nil {
		e.Error = err.Error()
	} else {
		e.Hits, e.Facts, e.Model = queryAnswer(res)
	}
	if werr := s.queries.Add(e); werr != nil {
		s.log.Printf("query log %s: %v", s.queries.Path(), werr)
	}
}

// queryAnswer reports how many facts a result brought, the first few of them by
// name, and the model that read the question when one did. A query verb without a
// case here is logged as an answer with nothing in it, so a new verb needs one.
func queryAnswer(res any) (hits int, facts []string, modelName string) {
	switch v := res.(type) {
	case brain.SearchResult:
		return len(v.Matches), factNames(v.Matches), ""
	case brain.RecallResult:
		recs := make([]model.Record, 0, len(v.Matches))
		for _, m := range v.Matches {
			recs = append(recs, m.Record)
		}
		return len(v.Matches), factNames(recs), v.Model
	case brain.DescribeResult:
		// What describe answers with is the entity's own facts; the neighbours a
		// depth brings are its answer too, but naming the entity's facts says
		// what was actually learned.
		if len(v.Entities) == 0 {
			return 0, nil, ""
		}
		return len(v.Entities[0].Out), factNames(v.Entities[0].Out), ""
	}
	return 0, nil, ""
}

// factNames names up to querylog.MaxFacts facts as `subject relation object`.
func factNames(recs []model.Record) []string {
	if len(recs) == 0 {
		return nil
	}
	out := make([]string, 0, min(len(recs), querylog.MaxFacts))
	for _, r := range recs {
		if len(out) == querylog.MaxFacts {
			break
		}
		out = append(out, querylog.Fact(r.Subject, r.Relation, r.Object))
	}
	return out
}

// listQueries is GET /api/v1/queries: the log, newest first. It is a read, not an
// admin, endpoint: everything it shows is a fact the same token could already
// read, and what it adds is only that someone asked.
func (s *Server) listQueries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	verb := q.Get("verb")
	if err := querylog.CheckVerb(verb); err != nil {
		WriteError(w, brain.Usagef("%v", err))
		return
	}
	n, err := intOr(q, "n", querylog.DefaultN)
	if err != nil {
		WriteError(w, err)
		return
	}
	if n < 1 {
		WriteError(w, brain.Usagef("-n must be at least 1"))
		return
	}
	if n > querylog.MaxN {
		n = querylog.MaxN
	}
	res, err := s.queries.Read(verb, n)
	if err != nil {
		WriteError(w, brain.StoreErr(err))
		return
	}
	WriteJSON(w, http.StatusOK, res, nil)
}
