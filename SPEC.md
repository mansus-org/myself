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
```

Global flags: `--file PATH`, `--json`, `--at TIME` (only on commands that list it above).

- `remember -` is all-or-nothing: every line is validated first; on any error nothing is written, and the error names the line number.
- `forget <uuid>` is used when exactly one argument is given and it is a UUID. Otherwise the arguments are `<subject> <relation> [<object...>]`: it forgets every live record with that subject and relation, narrowed to those whose normalized object equals the given object if one is given. Exit 2 if nothing matches.
- `describe <entity>`: records where the entity is subject (`out`) or the linked object (`in`). `--depth` defaults to 0 (only that entity); depth N also shows every entity reachable within N hops, breadth-first.
- `ls <type>`: every entity that reaches `<type>` through a chain of live `is` records (transitive subtypes). `ls` with no argument lists every entity that is the object of a live `is` record. **Decision:** the original text said "either direction"; only the subtype direction is listed, since the supertypes of an entity are already visible with `describe`, and mixing both directions makes `ls software` ambiguous.
- `path <a> <b>`: shortest chain of live records connecting `a` to `b`, edges walked in either direction; `--max-depth` defaults to 6.
- `search <text>`: case-insensitive substring match on subject, relation or object of live records.
- `log`: every record, forgets included, in uuid order. `--since` filters by time, `-n N` keeps the last N.
- `history <subject> [<relation>]`: every record, forgotten or not, for that subject (and relation), plus the `forget` records pointing at them, in uuid order.
- `prime`: a compact summary of the brain to paste into an agent context: entities ordered by live record count (descending, then name), each rendered as `describe` at depth 0, cut at a line boundary to fit `--budget` characters (default 2000).
- `dream`: read-only report of near-duplicate subjects (equal after removing `-` and a trailing `s`). Writes nothing.
- `export --obsidian <dir>`: writes one `<entity>.md` per entity (live state only), overwriting existing files of the same name. Note format is shown in section 9.
- `sync [--export DIR]`: backs the store up with git. The store directory (that of the resolved file) must be a git work tree, else it prints setup instructions on stderr and exits 1. It sets the merge driver config and the `.gitattributes` line, commits the store as `myself: N records since last sync`, then `git pull --rebase` (falling back to a merge) and `git push` if a remote exists. `--export DIR` runs `export --obsidian DIR` after pulling. Git missing or failing exits 3.
- `merge-driver <base> <ours> <theirs>`: git merge driver (`%O %A %B`); writes the union of the complete lines of all three, sorted by `uuid`, to `<ours>`. Exit 1 on I/O errors.

## 6. Output

- Default output is terse plain text for LLM token cost: no colors, no box drawing, no headers beyond what is shown in section 9. Ordering is stable: records by uuid, entities by name unless stated otherwise.
- Errors go to stderr as a single line `error: ...`. Warnings go to stderr as `warning: ...`.
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
