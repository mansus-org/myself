package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mansus-org/myself/internal/brain"
)

// registerMCP serves the MCP endpoint at /mcp (SPEC section 10, "MCP").
// Auth(Read) lets any valid token in; the write tools check the scope
// themselves, so a read token gets a tool error that says what it lacks.
//
// The handler is stateless: no session survives between requests, so a pod
// restart loses nothing. Each request builds its own mcp.Server, which is
// how a tool knows the caller (the schema cache keeps that cheap).
func (s *Server) registerMCP() {
	cache := mcp.NewSchemaCache()
	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		id, _ := IdentityFrom(r.Context())
		return s.mcpServer(id, cache)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	s.mux.Handle("/mcp", s.Auth(Read, h))
}

// mcpInstructions is sent in the initialize result.
const mcpInstructions = `myself is your long-term memory: a graph of facts ("subject relation object") about people, projects, tools and yourself, shared across sessions and agents. Use it, not a notes file, whenever the user says "remember...", corrects how you work, or you learn something durable.

- Recall before guessing or saying "I don't know" about a person, project or tool: describe (known name), search (partial name or topic), path (how X and Y relate), log (what changed lately). Read the "myself://prime" resource, or call prime, at session start.
- Write with remember: one fact per fact, several facts in one call. Facts are superseded, never edited; forget retracts.
- Relations are a closed list (call relations). Type every new entity with "is" and a short type name (relay is project). Link entities by name as the object, use a sentence otherwise. Put versions and details in "has".
- "i", "me" and "myself" all mean the entity self. Update your self-model with describe i, then remember i does '<one paragraph>' replace=true.
- Never store secrets, tokens or passwords, or the user's work email.`

const (
	hintWrite     = "Needs a write token. "
	relationsHint = "The relation must be one of the closed list (see the relations tool): "
)

type rememberInput struct {
	Facts   []brain.Fact `json:"facts" jsonschema:"facts to write, all or nothing; each is subject, relation, object"`
	Replace bool         `json:"replace,omitempty" jsonschema:"first forget every live record with the same subject and relation as a fact (supersede)"`
}

type forgetInput struct {
	UUID     string  `json:"uuid,omitempty" jsonschema:"uuid of one record to retract (use this or subject and relation)"`
	Subject  string  `json:"subject,omitempty" jsonschema:"subject of the live records to retract"`
	Relation string  `json:"relation,omitempty" jsonschema:"relation of the live records to retract"`
	Object   *string `json:"object,omitempty" jsonschema:"only retract records whose object matches this"`
	Reason   string  `json:"reason,omitempty" jsonschema:"why, stored with the forget record"`
}

type describeInput struct {
	Entity string `json:"entity" jsonschema:"entity name; i, me and myself mean self"`
	Depth  int    `json:"depth,omitempty" jsonschema:"also show entities reachable within this many hops (default 0)"`
	At     string `json:"at,omitempty" jsonschema:"show the state as of this time (RFC 3339 or YYYY-MM-DD)"`
}

type lsInput struct {
	Type string `json:"type,omitempty" jsonschema:"list the entities that are (transitively) of this type; omit to list all types"`
	At   string `json:"at,omitempty" jsonschema:"as of this time (RFC 3339 or YYYY-MM-DD)"`
}

type searchInput struct {
	Q  string `json:"q" jsonschema:"text to find, case-insensitive, in subject, relation or object of live facts"`
	At string `json:"at,omitempty" jsonschema:"as of this time (RFC 3339 or YYYY-MM-DD)"`
	N  *int   `json:"n,omitempty" jsonschema:"keep only the first N matches"`
}

type pathInput struct {
	From     string `json:"from" jsonschema:"first entity"`
	To       string `json:"to" jsonschema:"second entity"`
	MaxDepth int    `json:"max_depth,omitempty" jsonschema:"longest chain to look for (default 6)"`
}

type logInput struct {
	Since string `json:"since,omitempty" jsonschema:"only records at or after this time (RFC 3339 or YYYY-MM-DD)"`
	N     *int   `json:"n,omitempty" jsonschema:"keep only the last N records"`
}

type historyInput struct {
	Subject  string `json:"subject" jsonschema:"entity whose records to list, forgotten ones too"`
	Relation string `json:"relation,omitempty" jsonschema:"only this relation"`
}

type primeInput struct {
	Budget int `json:"budget,omitempty" jsonschema:"maximum characters of output (default 2000)"`
}

type none struct{}

// mcpOut is what a tool returns: the section 6 result and its CLI text.
type mcpOut struct {
	v    any
	text string
}

func linesOut(v interface{ Lines() []string }) mcpOut {
	return mcpOut{v, strings.Join(v.Lines(), "\n")}
}

// opt maps an empty optional string to "not given".
func opt(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// mcpServer builds the MCP server for one caller.
func (s *Server) mcpServer(id Identity, cache *mcp.SchemaCache) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "myself", Version: s.version},
		&mcp.ServerOptions{Instructions: mcpInstructions, SchemaCache: cache})

	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	yes := true
	add := func(name, title, desc string, annotations *mcp.ToolAnnotations) *mcp.Tool {
		annotations.Title = title
		return &mcp.Tool{Name: name, Title: title, Description: desc, Annotations: annotations}
	}

	addTool(s, srv, id, add("remember", "Remember facts", hintWrite+
		"Write facts to the brain, all or nothing. Each fact is subject, relation, object. "+relationsHint+"is, has, uses, does, prefers, related. "+
		"Type every new entity with is and a short type name (relay is project, never a sentence). "+
		"The object is an entity name when you want the link (relay uses bun), a sentence otherwise (relay uses 'bun for the backend'). "+
		"Versions and details go in has (relay has 'postgres 17'), keeping the uses link. "+
		"i, me and myself mean the entity self. To update your self-model call describe i first, then remember i does '<one paragraph, under 600 chars>' with replace=true. "+
		"replace=true supersedes every live fact with the same subject and relation. Pass several facts in one call. "+
		"Never store secrets, tokens, passwords or the user's work email.",
		&mcp.ToolAnnotations{DestructiveHint: new(bool)}), true,
		func(st *RequestStore, in rememberInput) (mcpOut, error) {
			if len(in.Facts) == 0 {
				return mcpOut{}, brain.Usagef("remember needs at least one fact")
			}
			defer s.LockWrites()()
			res, err := brain.Remember(st, in.Facts, in.Replace, s.now())
			return linesOut(res), err
		})

	addTool(s, srv, id, add("forget", "Forget facts", hintWrite+
		"Retract facts. Give the uuid of one record (from describe, history or log), or subject and relation, optionally narrowed by object, to retract every matching live record. "+
		"To correct a fact prefer remember with replace=true. Retracted facts stay in history.",
		&mcp.ToolAnnotations{DestructiveHint: &yes}), true,
		func(st *RequestStore, in forgetInput) (mcpOut, error) {
			if in.UUID == "" && in.Subject == "" && in.Relation == "" {
				return mcpOut{}, brain.Usagef("%s", brain.ForgetUsage)
			}
			defer s.LockWrites()()
			res, err := brain.Forget(st, brain.ForgetArgs(in), s.now())
			return linesOut(res), err
		})

	addTool(s, srv, id, add("describe", "Describe an entity",
		"Show the live facts out of an entity (subject relation object) and into it (<- subject relation), with uuids in the structured result. "+
			"Use it before answering about a person, project or tool, and before writing, to check names and spelling. "+
			"depth=1 adds its neighbours. Not found means unknown: fall back to search. describe i shows your self-model.",
		readOnly), false,
		func(st *RequestStore, in describeInput) (mcpOut, error) {
			if in.Entity == "" {
				return mcpOut{}, brain.Usagef("describe needs exactly one <entity>")
			}
			res, err := brain.Describe(st, in.Entity, in.Depth, opt(in.At))
			return linesOut(res), err
		})

	addTool(s, srv, id, add("ls", "List types or entities",
		"Without type: list every type (the objects of is facts). With type: list the entities that are of that type, including through subtypes (ls project).",
		readOnly), false,
		func(st *RequestStore, in lsInput) (mcpOut, error) {
			res, err := brain.Ls(st, opt(in.Type), opt(in.At))
			return linesOut(res), err
		})

	addTool(s, srv, id, add("relations", "List the relations",
		"List the closed set of relations a fact can use. Anything else is rejected.",
		readOnly), false,
		func(st *RequestStore, in none) (mcpOut, error) {
			return linesOut(brain.Relations()), nil
		})

	addTool(s, srv, id, add("search", "Search the brain",
		"Find live facts whose subject, relation or object contains the text (case-insensitive substring). Use it for partial names and topics, and before saying you don't know.",
		readOnly), false,
		func(st *RequestStore, in searchInput) (mcpOut, error) {
			if in.Q == "" {
				return mcpOut{}, brain.Usagef("search needs <text>")
			}
			res, err := brain.Search(st, in.Q, opt(in.At), in.N)
			return linesOut(res), err
		})

	addTool(s, srv, id, add("path", "Find a path between entities",
		"Show the shortest chain of live facts linking two entities: how X relates to Y.",
		readOnly), false,
		func(st *RequestStore, in pathInput) (mcpOut, error) {
			if in.From == "" || in.To == "" {
				return mcpOut{}, brain.Usagef("path needs <a> and <b>")
			}
			depth := in.MaxDepth
			if depth == 0 {
				depth = brain.DefaultMaxDepth
			}
			res, err := brain.Path(st, in.From, in.To, depth)
			return linesOut(res), err
		})

	addTool(s, srv, id, add("log", "Show recent records",
		"List every record in order, forgets included, one line each: uuid time subject relation object. Use since or n for what changed lately.",
		readOnly), false,
		func(st *RequestStore, in logInput) (mcpOut, error) {
			res, err := brain.Log(st, opt(in.Since), in.N)
			return linesOut(res), err
		})

	addTool(s, srv, id, add("history", "Show the history of a subject",
		"List every record ever written for a subject, forgotten ones and their forgets included. Use it to see what a fact used to be, or to find a uuid.",
		readOnly), false,
		func(st *RequestStore, in historyInput) (mcpOut, error) {
			res, err := brain.History(st, in.Subject, opt(in.Relation))
			return linesOut(res), err
		})

	addTool(s, srv, id, add("prime", "Summarise the brain",
		"A compact summary of the brain for your context: self first, then the entities linked to it, then the rest by how much is known. Call it at session start.",
		readOnly), false,
		func(st *RequestStore, in primeInput) (mcpOut, error) {
			budget := in.Budget
			if budget == 0 {
				budget = brain.DefaultBudget
			}
			res, err := brain.Prime(st, budget)
			return mcpOut{res, res.Text}, err
		})

	addTool(s, srv, id, add("dream", "Find near-duplicate entities",
		"Report groups of near-duplicate subjects (relay and relays) to merge. Read-only; merge them with remember and forget.",
		readOnly), false,
		func(st *RequestStore, in none) (mcpOut, error) {
			res, err := brain.Dream(st)
			return linesOut(res), err
		})

	srv.AddResource(&mcp.Resource{
		URI: primeURI, Name: "prime", Title: "Brain summary", MIMEType: "text/plain",
		Description: "The same text as the prime tool, with the default budget.",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		res, err := brain.Prime(s.Store(), brain.DefaultBudget)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: primeURI, MIMEType: "text/plain", Text: res.Text}}}, nil
	})
	return srv
}

const primeURI = "myself://prime"

// addTool registers t on srv. fn runs against a fresh store; a write tool
// first checks the caller's scope. Errors become tool errors (isError) with
// the CLI's message; warnings are appended to the text and to the
// structured result.
func addTool[In any](s *Server, srv *mcp.Server, id Identity, t *mcp.Tool, write bool, fn func(st *RequestStore, in In) (mcpOut, error)) {
	mcp.AddTool(srv, t, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		if write {
			if err := authorize(id, Write); err != nil {
				return nil, nil, errors.New(err.Error())
			}
		}
		st := s.Store()
		out, err := fn(st, in)
		if err != nil {
			return nil, nil, errors.New(err.Error())
		}
		warnings := st.Warnings()
		b, err := encodeJSON(out.v, warnings)
		if err != nil {
			return nil, nil, errors.New("encoding the result failed")
		}
		text := out.text
		for _, w := range warnings {
			text += "\nwarning: " + w
		}
		text = strings.TrimPrefix(text, "\n")
		if text == "" {
			text = "(none)"
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, json.RawMessage(b), nil
	})
}
