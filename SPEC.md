# myself: specification

The contract for the `myself` CLI: a Go, agent-native graph memory stored as an append-only JSONL event log. Every implementation ticket builds against this file. Where the original proposal had a problem, the deviation is marked **Decision** with the reason.

## 1. Data model

Every record is exactly five fields. Nothing else is stored.

| field      | meaning |
|------------|---------|
| `uuid`     | UUIDv7 (time-sortable). The record's identity. |
| `subject`  | A name (`relay`, `bun`, `self`). For a `forget` record, the uuid of the forgotten record. |
| `relation` | One value of the closed enum in section 2. Says roughly what kind of link this is. |
| `object`   | Free text of any length, ideally a sentence. The *how* and *why* live here. |
| `time`     | RFC 3339 UTC timestamp (`2026-10-29T16:00:00Z`) of when the record was written. |

### Entities and the graph

- **Entities are implicit.** A name is an entity if it is the normalized subject of a live record, or the normalized object of a live `is` record (so `project` is an entity once `relay is project` exists, even if nothing has `project` as a subject yet).
- **The graph is made of transitive connections between nodes.** An edge `subject -> object` exists when a live record's normalized object equals a known entity name. There is no entity/literal flag and no per-relation edge type. The graph is the transitive closure of those edges.
- Relation kind matters less than the connection itself and the free-text object that explains it. A record whose object is a sentence (`bun for the backend, biome for linting`) is still a fact, it just creates no edge.
- Consequences, all derived from the rule above:
  - `describe <entity> --depth N` walks N hops, following edges in both directions.
  - `path <a> <b>` finds a chain of records linking `a` to `b` through intermediate entities.
  - `ls <type>` follows `is` transitively: with `relay is project` and `project is software`, `ls software` lists `project` and `relay`.
- **Special names:** `i`, `me` and `myself`, as a subject or as a whole object, resolve to the entity `self` (the agent or owner identity used for molting).
- Not stored: `object_kind`, `by`, `src`, `invalid_at`, a schema version, an `alias` op. Add them only when a need appears.

## 2. Relations (closed enum)

`is`, `are`, `has`, `uses`, `does`, `prefers`, `related`.

- `are` is accepted as a synonym of `is` and stored as `is`, so `ls` and type queries see one relation.
- `forget` is reserved for retractions (section 4). `remember` cannot use it.
- Matching is case-insensitive. Anything else is rejected with exit code 1 and a one-line stderr message written for an LLM to retry from:
  `error: unknown relation "built-with". use one of: is, are, has, uses, does, prefers, related`
  For the reserved word: `error: relation "forget" is reserved, use the forget command. use one of: is, are, has, uses, does, prefers, related`
- The list lives in one place in code and in this file. Adding a relation means changing both, on purpose.

## 3. Normalization

- Applied to subjects, and to objects only when matched against entity names: trim, lowercase, collapse internal whitespace runs to `-`, strip surrounding punctuation. `Relay App` becomes `relay-app`.
- The object is stored verbatim (trimmed). Normalization happens at write time for subjects and at match time for objects.
- No stemming. `dream` reports near-duplicate subjects instead.

## 4. Event log format

- Default path `~/.myself/brain.jsonl`; override with `--file` or `MYSELF_FILE` (flag wins). Created, with its directory, on first write. A missing file reads as empty.
- One JSON object per line, UTF-8, `\n` terminated, append-only. Compact JSON, keys in this order (so lines sort by uuid):

```json
{"uuid":"0192d7c4-0002-7000-8000-000000000002","subject":"relay","relation":"uses","object":"bun for the backend, biome for linting","time":"2026-10-29T16:00:05Z"}
```

- **Forgetting is a record:** `subject` is the uuid of the forgotten record, `relation` is `forget`, `object` is the optional reason (empty string if none). A record is live unless some `forget` record points at it. Original lines are never rewritten. Forgetting an already forgotten or unknown uuid is a no-op for readers.
- `remember --replace` writes `forget` records for every live record with the same subject and relation, then the new record.
- **Order is uuid order, not file order.** Readers sort by uuid, so a reordered file means the same thing.
- `--at TIME` (RFC 3339, or `YYYY-MM-DD` meaning 00:00:00Z) evaluates the log as of that time: only records with `time <= TIME` count, forgets included.
- **Git merge recipe.** Both sides only appended, so the union is the answer. Because keys are ordered with `uuid` first, a plain sort is a uuid sort:

```sh
git checkout --merge brain.jsonl
{ git show :2:brain.jsonl; git show :3:brain.jsonl; } | sort -u > brain.jsonl
git add brain.jsonl
```

  That is: concatenate both sides, dedupe by `uuid` (identical lines collapse), sort by `uuid`. A `.gitattributes` line `brain.jsonl merge=union` gets the concatenate-and-dedupe part automatically, but not the sort, which readers do anyway.

  `myself sync` automates this: it sets `.gitattributes` to `brain.jsonl merge=myself` and `git config merge.myself.driver "myself merge-driver %O %A %B"`, and the driver does the union, dedupe and uuid sort (lines that are not records are kept, last).

## 5. CLI grammar

Positional arguments, no `< >` (the shell eats them as redirections). Everything after the relation is the object, joined with single spaces, so quotes are optional. Flags may appear anywhere.

```
myself remember <subject> <relation> <object...> [--replace]
myself remember -                  # bulk: one "subject relation object..." per line on stdin
myself forget <uuid> [--reason TEXT]
myself forget <subject> <relation> [<object...>] [--reason TEXT]
myself describe <entity> [--depth N] [--at TIME]
myself ls [<type>] [--at TIME]
myself relations
myself search <text> [--at TIME]
myself path <a> <b> [--max-depth N]
myself log [--since TIME] [-n N]
myself history <subject> [<relation>]
myself prime [--budget CHARS]
myself dream
myself export --obsidian <dir>
myself sync [--export DIR]
myself merge-driver <base> <ours> <theirs>
myself serve [--addr ADDR] [--site DIR]
```

Global flags: `--file PATH`, `--json`, `--at TIME` (only on commands that list it above).

- `remember -` is all-or-nothing: every line is validated first; on any error nothing is written, and the error names the line number. In bulk mode only, if the object (after joining) is wrapped in one matching pair of `'...'` or `"..."`, that pair is stripped, since nothing else has removed it (the shell does that for the single-argument form, which is unchanged).
- `forget <uuid>` is used when exactly one argument is given and it is a UUID. Otherwise the arguments are `<subject> <relation> [<object...>]`: it forgets every live record with that subject and relation, narrowed to those whose normalized object equals the given object if one is given. Exit 2 if nothing matches.
- `describe <entity>`: records where the entity is subject (`out`) or the linked object (`in`). `--depth` defaults to 0 (only that entity); depth N also shows every entity reachable within N hops, breadth-first.
- `ls <type>`: every entity that reaches `<type>` through a chain of live `is` records (transitive subtypes). `ls` with no argument lists every entity that is the object of a live `is` record. **Decision:** the original text said "either direction"; only the subtype direction is listed, since the supertypes of an entity are already visible with `describe`, and mixing both directions makes `ls software` ambiguous.
- `path <a> <b>`: shortest chain of live records connecting `a` to `b`, edges walked in either direction; `--max-depth` defaults to 6.
- `search <text>`: case-insensitive substring match on subject, relation or object of live records.
- `log`: every record, forgets included, in uuid order. `--since` filters by time, `-n N` keeps the last N.
- `history <subject> [<relation>]`: every record, forgotten or not, for that subject (and relation), plus the `forget` records pointing at them, in uuid order.
- `prime`: a compact summary of the brain to paste into an agent context, cut at a line boundary to fit `--budget` characters (default 2000). Entities are ordered `self` first (if it exists), then the entities directly linked to `self` (one hop, breadth-first order), then the rest by live record count (descending, then name). Each is rendered as `describe` at depth 0, except that incoming `is` records are folded into one line `  <- is: a, b, c` (subjects sorted, comma-separated) instead of one `<- x is` line each.
- `dream`: read-only report of near-duplicate subjects (equal after removing `-` and a trailing `s`). Writes nothing.
- `export --obsidian <dir>`: writes one `<entity>.md` per entity (live state only), overwriting existing files of the same name. Note format is shown in section 9.
- `sync [--export DIR]`: backs the store up with git. The store directory (that of the resolved file) must be a git work tree, else it prints setup instructions on stderr and exits 1. It sets the merge driver config and the `.gitattributes` line, commits the store as `myself: N records since last sync`, then `git pull --rebase` (falling back to a merge) and `git push` if a remote exists. `--export DIR` runs `export --obsidian DIR` after pulling. Git missing or failing exits 3.
- `merge-driver <base> <ours> <theirs>`: git merge driver (`%O %A %B`); writes the union of the complete lines of all three, sorted by `uuid`, to `<ours>`. Exit 1 on I/O errors.
- `serve [--addr ADDR] [--site DIR]`: serves the store over HTTP (section 10) on `ADDR` (default `:8080`) until SIGINT or SIGTERM. Exit 1 if `MYSELF_ADMIN_TOKEN_SHA256` is set but not 64 hex characters, 3 if the address cannot be bound or `tokens.json` cannot be read.

## 6. Output

- Default output is terse plain text for LLM token cost: no colors, no box drawing, no headers beyond what is shown in section 9. Ordering is stable: records by uuid, entities by name unless stated otherwise.
- Errors go to stderr as a single line `error: ...`. Warnings go to stderr as `warning: ...`. `remember` (single and bulk) warns `warning: "is" object looks like a sentence; use a short type name and put details in has or does` when the relation is `is` or `are` and the object has more than 3 words; the fact is still written.
- `--json` writes one JSON value to stdout, keys as listed below, same ordering rules. Errors stay plain text on stderr and stdout is empty. `record` below means the five-field object from section 1.

| command | `--json` stdout |
|---------|-----------------|
| `remember` | `{"remembered":[record,...],"forgotten":[record,...]}` (`forgotten` holds the `forget` records written by `--replace`) |
| `forget` | `{"forgotten":[record,...]}` (the `forget` records written) |
| `describe` | `{"entity":"relay","at":null,"entities":[{"name":"relay","depth":0,"out":[record,...],"in":[record,...]},...]}` (`at` is the `--at` string or null; `entities` in breadth-first order, then name) |
| `ls` | `{"type":"software","entities":["project","relay"]}` (`type` is null without an argument) |
| `relations` | `{"relations":["is","are","has","uses","does","prefers","related"]}` |
| `search` | `{"matches":[record,...]}` |
| `path` | `{"from":"relay","to":"software","path":[record,...]}` |
| `log` | `{"records":[record,...]}` |
| `history` | `{"records":[record,...]}` |
| `prime` | `{"budget":2000,"truncated":false,"text":"..."}` |
| `dream` | `{"near_duplicates":[["relay","relays"],...]}` |
| `export` | `{"dir":"./vault","entities":3,"files":["project.md",...]}` |
| `sync` | `{"committed":3,"pulled":true,"pushed":true,"exported":null}` (`exported` is the entity count with `--export`) |

- **Exit codes:**
  - `0` ok (including searches and listings with no results).
  - `1` usage error: bad flags or arguments, unknown or reserved relation, empty object, bad `--at`.
  - `2` not found: `describe` or `history` on an unknown entity or subject, `ls <type>` on an unknown type, `forget` that matches nothing, `path` with no chain.
  - `3` store error: file unreadable or unwritable, lock timeout (5 s).
- **Decision:** the original listed "corrupt line" under exit 3 and also said readers warn without failing. Resolved in favor of not failing: a corrupt line is skipped with a stderr warning and the exit code is unaffected.

## 7. Concurrency and robustness

- Writers take an exclusive `flock` on `<file>.lock` (`brain.jsonl.lock`), append, `fsync`, release. A writer that cannot get the lock within 5 s exits 3.
- Readers take no lock. They ignore a trailing partial line (a crash mid-write) silently, and for any other line that is not a valid record they print `warning: line N: corrupt, skipped` to stderr and carry on.
- A write is a single `write` of whole lines (`remember -` and `--replace` write all their lines in one call) so a reader sees either none or a prefix of complete lines.

## 8. Performance target

Load and index 50k records in under 100 ms on the runner host. If this ever breaks, add a snapshot cache. Not in the MVP.

## 9. Worked example

Starting from no file. Timestamps and uuids are fixed for illustration (real ones come from the clock); `$` lines are input, the rest is stdout unless marked `stderr`.

```
$ myself remember relay is project
remembered 0192d7c4-0001-7000-8000-000000000001
$ myself remember relay uses "bun for the backend, biome for linting, deployed on k3s"
remembered 0192d7c4-0002-7000-8000-000000000002
$ myself remember relay built-with bun
stderr: error: unknown relation "built-with". use one of: is, are, has, uses, does, prefers, related
(exit 1, nothing written)
$ myself remember project are software
remembered 0192d7c4-0003-7000-8000-000000000003
```

The three records are written at 16:00:00Z, 16:00:05Z and 16:00:10Z. Note `are` is stored as `is`.

```
$ myself describe relay
relay
  is project
  uses bun for the backend, biome for linting, deployed on k3s
$ myself describe relay --depth 1
relay
  is project
  uses bun for the backend, biome for linting, deployed on k3s
project
  is software
  <- relay is
$ myself ls software
project
relay
$ myself path relay software
relay is project
project is software
```

`describe` prints `<relation> <object>` for each outgoing record and `<- <subject> <relation>` for each incoming one, entities flush left, records indented two spaces. With `--depth 1`, `project` appears because `relay is project` makes it one hop away; `software` would need `--depth 2`.

```
$ myself forget relay uses --reason "moved off bun"
forgot 0192d7c4-0002-7000-8000-000000000002
$ myself describe relay
relay
  is project
$ myself history relay
0192d7c4-0001-7000-8000-000000000001 2026-10-29T16:00:00Z relay is project
0192d7c4-0002-7000-8000-000000000002 2026-10-29T16:00:05Z relay uses bun for the backend, biome for linting, deployed on k3s
0192d7c4-0004-7000-8000-000000000004 2026-10-29T16:01:00Z forget 0192d7c4-0002-7000-8000-000000000002 moved off bun
```

`forget` printed the uuid of the record it retracted; the `forget` record itself is `0192d7c4-0004-…`, written at 16:01:00Z. `history` and `log` lines are `<uuid> <time> <subject> <relation> <object>`; for a `forget` record that reads `<uuid> <time> forget <target-uuid> <reason>`.

```
$ myself history relay --json
{"records":[{"uuid":"0192d7c4-0001-7000-8000-000000000001","subject":"relay","relation":"is","object":"project","time":"2026-10-29T16:00:00Z"},{"uuid":"0192d7c4-0002-7000-8000-000000000002","subject":"relay","relation":"uses","object":"bun for the backend, biome for linting, deployed on k3s","time":"2026-10-29T16:00:05Z"},{"uuid":"0192d7c4-0004-7000-8000-000000000004","subject":"0192d7c4-0002-7000-8000-000000000002","relation":"forget","object":"moved off bun","time":"2026-10-29T16:01:00Z"}]}
```

```
$ myself export --obsidian ./vault
exported 3 entities to ./vault
```

Live state only, one note per entity, objects equal to an entity become `[[wikilinks]]`, and incoming links are listed under `Linked from`:

```
vault/project.md        vault/relay.md        vault/software.md
# project               # relay                # software

- is [[software]]       - is [[project]]       ## Linked from
                                               - [[project]] is
## Linked from
- [[relay]] is
```

(Columns are only for display here; each file is separate. A note's lines are: `# <entity>`, blank line, one `- <relation> <object>` per outgoing record, and, if there are incoming records, `## Linked from` and one `- [[<subject>]] <relation>` per incoming record. Non-empty sections are separated by one blank line and empty ones are omitted.)

## 10. Server

`myself serve` puts one brain behind HTTP so several agents, harnesses and models share it. It reads and writes the same store as the CLI (`--file`, `MYSELF_FILE`; in the container `MYSELF_FILE=/data/brain.jsonl`), with the same rules: same validation, same error messages, same exit codes, same JSON shapes as section 6. One server process per store.

### Routes

| path | what |
|------|------|
| `/` | the landing page: `site/` embedded in the binary, or `--site DIR`. `GET` and `HEAD` only. |
| `GET /healthz` | `ok`, no auth. |
| `/api/v1/...` | the REST API below. Everything under `/api/` answers JSON. |
| `/mcp`, `/app` | not registered yet (404). Reserved for MCP and the web UI. |

Every request is logged on stderr as one line: `<method> <path> <status> <duration> <token name or ->`. The query string and tokens are never logged.

### Auth

- Every `/api/v1` call sends `Authorization: Bearer <token>`.
- **Admin token:** the token whose sha256 (lowercase hex) equals the env `MYSELF_ADMIN_TOKEN_SHA256`, compared in constant time. The server never sees the plaintext anywhere else. Admin has write scope and is the only caller of import and the token endpoints.
- **Agent tokens:** `mys_` followed by 64 hex characters (32 random bytes), created by the admin with scope `read` or `write`. Only their sha256 is stored, in `tokens.json` next to the store file (mode 0600, written to a temp file and renamed, under a flock on `tokens.json.lock`). `last_used` is updated at most once a minute. Deleting a token revokes it immediately.
- **Scopes:** `read` may call every `GET` endpoint. `write` may also call `remember` and `forget`. Admin-only: `import` and `tokens`.
- If `MYSELF_ADMIN_TOKEN_SHA256` is unset or empty, every `/api/` request answers 503 (`/` and `/healthz` still work). The server never runs open.

### Responses and errors

- Success: status 200 (201 for a created token), `Content-Type: application/json`, the section 6 shape of the command as one compact JSON value (no HTML escaping) and a newline.
- Warnings that the CLI prints on stderr (corrupt lines, the `"is"` sentence warning) are added as a last field `"warnings":["..."]` to an object body, without the `warning: ` prefix. The field is absent when there are none.
- Errors: `{"error":"<message>","exit_code":N}`. The message is what the CLI prints after `error: `.

| status | exit_code | when |
|--------|-----------|------|
| 400 | 1 | usage: bad parameter, bad body, unknown relation, empty object, bad `at` |
| 404 | 2 | not found (as in section 6); also an unknown `/api/` route |
| 500 | 3 | store error: unreadable or unwritable file, lock timeout |
| 401 | 3 | missing, invalid or revoked token (with `WWW-Authenticate: Bearer`) |
| 403 | 3 | the token lacks the scope (`write`, or admin) |
| 503 | 3 | `MYSELF_ADMIN_TOKEN_SHA256` is not set |

Messages for parameters name the CLI flag (`--at: bad time "x": ...`, `--depth must not be negative`, `-n must be at least 1`), so a remote CLI can print them unchanged. A non-integer integer parameter gives `invalid argument "x" for <param>: want an integer`.

### Endpoints

Query parameters map to the CLI flags. A parameter that is present but empty counts as given (`?at=` is a bad time). Path segments are URL-decoded; `{entity}` and `{subject}` may contain `/`.

| method and path | access | CLI equivalent | body |
|-----------------|--------|----------------|------|
| `GET /api/v1/whoami` | read | | `{"name":"reader","scope":"read","admin":false}`; admin is `{"name":"admin","scope":"write","admin":true}` |
| `POST /api/v1/remember` | write | `remember` | request `{"facts":[{"subject":"relay","relation":"is","object":"project"},...],"replace":false}`; reply as `remember --json` |
| `POST /api/v1/forget` | write | `forget` | request `{"uuid":"..."}` or `{"subject":"relay","relation":"uses","object":"bun"}` (`object` optional), plus optional `"reason"`; reply as `forget --json` |
| `GET /api/v1/describe/{entity}?depth=N&at=TIME` | read | `describe` | as `describe --json` |
| `GET /api/v1/ls?type=T&at=TIME` | read | `ls [<type>]` | as `ls --json` |
| `GET /api/v1/relations` | read | `relations` | as `relations --json` |
| `GET /api/v1/search?q=TEXT&at=TIME` | read | `search` | as `search --json` |
| `GET /api/v1/path?from=A&to=B&max_depth=N` | read | `path` | as `path --json` |
| `GET /api/v1/log?since=TIME&n=N` | read | `log` | as `log --json` |
| `GET /api/v1/history/{subject}?relation=R` | read | `history` | as `history --json` |
| `GET /api/v1/prime?budget=N` | read | `prime` | as `prime --json` |
| `GET /api/v1/dream` | read | `dream` | as `dream --json` |
| `GET /api/v1/graph?at=TIME` | read | | see below |
| `GET /api/v1/export.jsonl` | read | | the log, see below |
| `POST /api/v1/import` | admin | | request: JSONL; reply `{"imported":N,"skipped":M}` |
| `GET /api/v1/tokens` | admin | | `[{"id","name","scope","created","last_used"},...]`, oldest first; `last_used` is null until first use |
| `POST /api/v1/tokens` | admin | | request `{"name":"claude-runner","scope":"write"}`; reply 201, the listed fields plus `"token":"mys_..."`, the only time the plaintext is returned |
| `DELETE /api/v1/tokens/{id}` | admin | | `{"deleted":"<id>"}`; 404 `unknown token "<id>"` |

Details:

- **Request bodies** are JSON objects with exactly the listed fields; unknown fields, trailing data or bad JSON are a 400 `bad request body: ...`. Bodies are limited to 1 MiB (import: 64 MiB).
- **remember** is all or nothing, like `remember -`: every fact is validated before anything is written. With one fact the error is the CLI's (`unknown relation "built-with". use one of: ...`); with several it is prefixed `fact N: ` (1-based). An empty `facts` list is `remember needs at least one fact`. Objects are JSON strings, so no quote stripping. `replace` works as `--replace`. The reply has the sentence warning in `warnings` when it applies.
- **forget** takes either `uuid`, or `subject` and `relation` (optionally narrowed by `object`, compared normalized); both forms at once is a 400. None of them is `forget needs <uuid>, or <subject> <relation> [<object...>]`. A non-uuid `uuid` is `"x" is not a uuid`.
- **search** with a missing or empty `q` is `search needs <text>`. **path** without `from` or `to` is `path needs <a> and <b>`. **describe** with an empty entity is `describe needs exactly one <entity>`.
- **graph** is for drawing the graph:

  ```json
  {"nodes":[{"name":"project","types":["software"],"records":2},{"name":"relay","types":["project"],"records":2},{"name":"software","types":[],"records":1}],
   "edges":[{"uuid":"0192d7c4-0001-7000-8000-000000000001","from":"relay","to":"project","relation":"is"},{"uuid":"0192d7c4-0003-7000-8000-000000000003","from":"project","to":"software","relation":"is"}]}
  ```

  `nodes` holds every entity, sorted by name. `types` are the entities its live `is` records point at, sorted. `records` counts its live outgoing and incoming records (the count `prime` orders by). `edges` holds every live record whose object is an entity (section 1), in uuid order, `from` the subject and `to` the object entity, both normalized. Records with free-text objects are not edges; fetch them with `describe`.
- **export.jsonl** returns the valid records of the log, forgets included, as canonical JSONL lines in uuid order (`Content-Type: application/x-ndjson`), for backups. Corrupt lines are left out.
- **import** takes JSONL (the last line may lack its newline; blank lines are skipped) and unions it with the store by uuid: records whose uuid already exists, or repeats within the body, are skipped; the rest are appended in uuid order with one `store.Append` (flock). Every line is validated first and any bad line rejects the whole import with `line N: <reason>`: not a record (section 4 rules), uuid not a uuid, time not RFC 3339, relation not one of the stored relations (`are` is never stored), a `forget` whose subject is not a uuid, an empty subject or object.

### Concurrency

The server caches the loaded records and graph and reloads when the file's size or mtime changes (another process, such as the CLI, appended) or after it wrote. Writes go through `store.Append` with the flock of section 7. Writes through the server are also serialized in the process, so `replace`, `forget` and `import` compute from the state they append to.

### Deploy

The image (repo `Dockerfile`) runs `myself serve --addr :8080` as user 10001 with `MYSELF_FILE=/data/brain.jsonl`. `.k3s/values.yaml` mounts a 1 Gi volume at `/data` and sets `MYSELF_ADMIN_TOKEN_SHA256`. Pull request previews use the same values, so they share the admin token and get their own empty volume.
