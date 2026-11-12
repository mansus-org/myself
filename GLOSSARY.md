# myself

A single-binary CLI that gives coding agents a long-term memory: facts in one append-only file, read
as a graph, alone or shared through `myself serve`. These are the words the spec, the CLI, the API,
MCP and the web app all use. `SPEC.md` is where each one is defined in full.

## The brain

**Brain**:
One store of facts: the file `myself` reads and writes, on its own or behind a server.
_Avoid_: Database, memory bank, knowledge base, vault

**Fact**:
One remembered statement, a subject, a relation and an object, written at a time.
_Avoid_: Record (in the app), triple, memory, note, statement

**Record**:
One line of the file: a fact, or a forget. The spec and the API say record; the app says fact or
forget, because a reader never needs the word.

**Subject**:
The entity a fact is about, stored as a normalized name (`relay-app`).

**Relation**:
What kind of fact it is, one of `is`, `has`, `uses`, `does`, `prefers`, `related` (`are` is read as
`is`).
_Avoid_: Predicate, verb, property, edge type

**Object**:
The free text of a fact, the how and the why. When it names an entity, the fact is a link.
_Avoid_: Value, target

**Entity**:
A name that is the subject of a live fact, or the object of a live `is` fact.
_Avoid_: Node (outside the graph's code), item, topic, page

**Type**:
An entity that another entity `is`: with `relay is project`, `project` is a type of `relay`. Types
nest: with `project is software`, `relay` is also `software`.
_Avoid_: Category, tag, class, kind, label

**Link**:
A live fact whose object names an entity, drawn as a line in the graph.
_Avoid_: Edge, connection, relationship

**Linked from**:
The facts of other entities that link to this one, as `describe` prints them.
_Avoid_: Backlinks, incoming edges, mentions

**Neighbour**:
An entity one link away, in either direction.

**Live**:
Not forgotten. A fact is live unless a forget points at it.

## Changing the brain

**Remember**:
Writing facts. All of one write is saved, or none of it.
_Avoid_: Save, add (as a verb on its own), insert, store

**Forget**:
Writing a forget for a fact, with an optional reason. The fact stays in the history, struck through:
nothing is erased.
_Avoid_: Delete, remove, retract, erase

**Forgotten**:
A fact that a forget points at.

**Replace**:
Remembering a fact after forgetting the live facts with the same subject and relation
(`remember --replace`).
_Avoid_: Update, overwrite, edit

**Molting**:
Changing what the agent knows about itself (`self`) by forgetting and rewriting, so earlier selves
stay readable.

**Self**:
The entity for the agent or owner identity. `i`, `me` and `myself` all mean `self`.

## Reading the brain

**History**:
Every fact ever written with a subject, forgotten ones included.

**Log**:
Every fact and forget, in the order they were written.
_Avoid_: Feed, activity, timeline, events

**As of**:
The brain read as it was at a past time (`--at`).
_Avoid_: Snapshot, version, point in time

**Graph**:
Entities drawn as squares and links as lines; the web app's home.

**Prime**:
The brief an agent reads at the start of a session, cut to a budget of characters (`myself prime`).
_Avoid_: Summary, context dump, digest

**Budget**:
The most characters prime may print.

**Dream**:
The housekeeping report: near-duplicate names (`myself dream`).

**Near-duplicate**:
Names that are equal once dashes and a trailing `s` are removed: `relay` and `relays`.
_Avoid_: Duplicate (they may be different things), alias

## Access

**Admin token**:
The one token the server is configured with, by its hash. It can do everything, including making and
revoking agent tokens.

**Agent token**:
A token the admin makes for an agent or a person, with a scope. Shown once, stored only as a hash.
_Avoid_: API key, password, secret (except for "shown once")

**Scope**:
What a token may do: `read` (every read), `write` (also remember and forget). The admin token is
above both.
_Avoid_: Role, permission level, access level

**Login link**:
A link with a one-time code (`#login=`) that signs the web app in for 12 hours, as the token that
made it. `myself graph --web` prints one.

**Session**:
What a login link gives the web app: a 12-hour token with the scope of the token that made the code.

## Agents

**Agent**:
A model at work in a harness, using the brain through the CLI, the skill and hooks, or MCP.

**Harness**:
The program that runs an agent: Claude Code, pi, Codex.
_Avoid_: Client (it is ambiguous), IDE, runtime

**Connect**:
Giving a harness what it needs to use this brain: the CLI, the skill, the hooks, the server's URL
and an agent token, or the MCP URL and a token.
_Avoid_: Integrate, install (for the whole thing), pair

**Skill**:
`SKILL.md`, the instructions that teach an agent when and how to use `myself`. Every server serves
its copy at `/skill.md`.

**Hooks**:
The scripts a harness runs at set moments: `session-start.sh` primes a session, `stop-nudge.sh` asks
once whether anything is worth remembering, `prompt-recall.sh` (optional) brings relevant facts to
each prompt.
