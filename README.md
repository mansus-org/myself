# myself

`myself` is a single-binary CLI that gives an agent a long-term memory: a small knowledge graph of entities and the links between them, kept in one append-only JSONL file. Any harness that can run Bash (Claude Code, Codex, pi) can use it.

```sh
$ myself remember relay is project
$ myself remember relay uses "bun for the backend, biome for linting"
$ myself describe relay
relay
  is project
  uses bun for the backend, biome for linting
$ myself remember i does "plan work from tickets"
$ myself remember i does "plan work and review PRs" --replace
$ myself history i
01a19418-81e9-7770-a08a-3822cf999776 2026-10-31T22:33:16Z self does plan work from tickets
01a19418-81eb-7738-9e18-7b2371c85fec 2026-10-31T22:33:16Z forget 01a19418-81e9-7770-a08a-3822cf999776
01a19418-81eb-7739-8650-215f8da3fb98 2026-10-31T22:33:16Z self does plan work and review PRs
```

![myself demo](demo/demo.svg)

Play the [terminal cast](demo/demo.cast) with `asciinema play demo/demo.cast`.

The contract for every command, output and exit code is [SPEC.md](SPEC.md).

## Why

- **The agent is the LLM, so the store stays dumb.** The model decides what is worth remembering and writes it as a fact. `myself` only stores, indexes and queries. No extraction step, no network, same input gives the same output.
- **CLI over MCP.** Every agent harness already has Bash. A binary plus a skill file needs no server, no config and no protocol support.
- **Triples plus time.** A fact is `subject relation object` with a timestamp. The object is free text, so the how and why live in it. Facts are superseded, not overwritten: `--replace` and `forget` write new records, and the old ones stay in the log.

The idea that facts are superseded rather than overwritten, and that you can ask what was true at a given time, comes from [Graphiti](https://github.com/getzep/graphiti). `myself` is much smaller: no LLM extraction (the calling agent writes the facts), no Neo4j (a JSONL file), no embeddings (substring search), no server. It has one timeline (when a record was written), not Graphiti's separate event and ingestion times.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mansus-org/myself/main/install.sh | sh
```

This installs the latest release to `~/.local/bin` (Linux and macOS, amd64 and arm64) after verifying its sha256 checksum. Set `MYSELF_VERSION=v0.1.0` to pin a release and `MYSELF_INSTALL_DIR` to change the target directory.

With Go: `go install github.com/mansus-org/myself/cmd/myself@latest`.

Prebuilt tarballs are on the [releases page](https://github.com/mansus-org/myself/releases).

## Use with Claude Code

Set up Claude Code so every session starts with the brain in context and uses `myself` through the skill.

1. Install the binary (see [Install](#install)). The brain lives in `~/.myself/brain.jsonl` (override with `MYSELF_FILE` or `--file`). It is created on first write.

2. From a checkout of this repo, install the hooks and the skill:

   ```sh
   mkdir -p ~/.myself/hooks ~/.claude/skills
   cp contrib/hooks/*.sh ~/.myself/hooks/
   chmod +x ~/.myself/hooks/*.sh
   cp -r skill/myself ~/.claude/skills/
   ```

3. Merge this into `~/.claude/settings.json` (keep any hooks and permissions you already have):

   ```json
   {
     "permissions": {
       "allow": ["Bash(myself:*)"]
     },
     "hooks": {
       "SessionStart": [
         {
           "hooks": [
             { "type": "command", "command": "~/.myself/hooks/session-start.sh", "timeout": 10 }
           ]
         }
       ],
       "Stop": [
         {
           "hooks": [
             { "type": "command", "command": "~/.myself/hooks/stop-nudge.sh", "timeout": 10 }
           ]
         }
       ]
     }
   }
   ```

`session-start.sh` prints `myself prime --budget 4000` (set `MYSELF_PRIME_BUDGET` to change it), which Claude Code adds to the session context, and records the session start time under `${TMPDIR:-/tmp}/myself-hooks/`. `stop-nudge.sh` blocks the first stop of a session once, asking whether anything is worth remembering, but only if nothing was written to the brain since the session started and the session did something: a Write, Edit, MultiEdit or NotebookEdit tool use, 3+ Bash tool uses, or uncommitted changes in a git `cwd`. Bash uses whose command starts with `myself` (or the `MYSELF_BIN` path) do not count, so read-only sessions are not nudged. The nudge asks the agent to repeat its final answer afterwards, so headless runs (`claude -p`) still end with the real answer. Set `MYSELF_NUDGE=off` to disable the nudge entirely. A resumed session (`claude --resume`) gets a fresh start time and can be nudged once more. Both scripts are POSIX sh, use `jq` when present, and exit 0 silently when `myself` is missing or fails. The comments at the top of each script list their settings.

Claude Code also has a built-in file-based auto-memory that can take "remember…" requests before `myself` does. On hosts where `myself` should be the only memory, you can turn it off with `"env": { "CLAUDE_CODE_DISABLE_AUTO_MEMORY": "1" }` in the same settings file.

If `~/.local/bin` is not on the PATH the hooks run with, set `MYSELF_BIN` to the full path of the binary.

## Commands

Arguments are positional and everything after the relation is the object, so quotes are optional. `--json` works on every command, `--file PATH` picks the store, and `--at TIME` (RFC 3339 or `YYYY-MM-DD`) shows the state as of that time where listed.

| command | what it does |
|---------|--------------|
| `remember <subject> <relation> <object...> [--replace]` | add a fact; `--replace` forgets live facts with the same subject and relation first |
| `remember -` | bulk: one `subject relation object` per line on stdin, all or nothing |
| `forget <uuid>` / `forget <subject> <relation> [<object...>] [--reason TEXT]` | retract a record, or every matching live record |
| `describe <entity> [--depth N] [--at TIME]` | outgoing and incoming facts, N hops out |
| `ls [<type>] [--at TIME]` | entities of a type, following `is` transitively |
| `relations` | list the allowed relations |
| `search <text> [--at TIME]` | case-insensitive substring match |
| `path <a> <b> [--max-depth N]` | shortest chain of facts linking two entities |
| `log [--since TIME] [-n N]` | every record, forgets included |
| `history <subject> [<relation>]` | every record for a subject, forgotten or not |
| `prime [--budget CHARS]` | compact summary to paste into an agent context |
| `dream` | report near-duplicate subjects (read-only) |
| `export --obsidian <dir>` | write one Markdown note per entity |
| `sync [--export DIR]` | back the store up with git |
| `merge-driver <base> <ours> <theirs>` | git merge driver used by `sync` |
| `version` | print version, commit and build date |

Relations are a closed set: `is`, `are` (stored as `is`), `has`, `uses`, `does`, `prefers`, `related`. Anything else is rejected with a message listing the valid ones. Details are in [SPEC.md](SPEC.md#5-cli-grammar).

## Storage

One JSON object per line, five fields, never rewritten:

```json
{"uuid":"01a19418-81d8-7344-b9f2-f3a54bd13f7d","subject":"relay","relation":"is","object":"project","time":"2026-10-31T22:33:16Z"}
```

The log is append-only. `forget` appends a record whose subject is the uuid of the retracted record, with `forget` as the relation and the optional reason as the object. A record is live unless a forget points at it. Records are ordered by uuid (UUIDv7, so by time), not by file position. Writers take a lock on `<file>.lock`; readers take none and skip corrupt lines with a warning.

Time travel: `myself describe relay --at 2026-10-30` shows relay as it was at that date, and `--at` also works with `ls` and `search`.

### Backup

Keep the store in a private git repository and sync it with one command:

```sh
cd ~/.myself && git init && git remote add origin <private repo url>
myself sync            # commits, pulls (rebase), pushes
```

`myself sync` configures a git merge driver (`myself merge-driver`) so two hosts that both appended resolve to the union of their records, sorted by uuid. It never creates a remote or sets a git identity. To sync every 15 minutes and refresh an Obsidian vault:

```sh
*/15 * * * * myself sync --export ~/vault/myself
```

or a systemd timer whose service runs `myself sync --export %h/vault/myself` with `OnCalendar=*:0/15`. A Claude Code `Stop` hook that runs `myself sync` works too. `myself` must be on the PATH of the timer or hook, since git calls it for merges.

## Obsidian mirror

`myself export --obsidian ./vault` writes one note per entity, live state only, overwriting files of the same name. Objects that match an entity become `[[wikilinks]]` and incoming links are listed under `Linked from`:

```
$ myself export --obsidian vault
exported 6 entities to vault
$ cat vault/relay.md
# relay

- is [[project]]
- does a Slack for agents
- uses bun for the backend, biome for linting
```

A note for `project` lists `- [[relay]] is` under `## Linked from`. The mirror is one way: edits in the vault are overwritten on the next export. The store is the source of truth.

## Molting and dreaming

Molting is how the agent changes what it knows about itself. The entity `self` holds the agent's identity and preferences; to change one, `remember i does "..." --replace` forgets the old fact and writes the new one, and `myself history i` shows the whole sequence, as in the example at the top. Nothing is lost, so a past self can be read with `--at`.

`dream` is a read-only housekeeping report. It lists subjects that differ only by `-` or a trailing `s`, so the agent can merge them by hand:

```
$ myself dream
relay relays
```

## Gotcha

`i`, `me` and `myself` as a name all mean the entity `self`. To remember something about this project, use another name, such as `myself-cli`.

## Non-goals

- No server.
- No embeddings (yet). Search is substring match.
- No two-way Obsidian sync.
- No query language. The commands above are the interface.

## Landing page

The site lives in `site/` (plain HTML, CSS and JS, no build step) and deploys to https://myself.mansus.org on merge to `main`: the `Dockerfile` serves `site/` with busybox httpd, `.k3s/values.yaml` holds the cluster settings, `.github/workflows/deploy.yml` builds the image. Pull requests get a preview at `myself-pr-<number>.mansus.org`. The design rationale is in `design/README.md`.

Preview locally: `bunx serve site -l 8080`.

## Development

Go (see `go.mod` for the version) and no other tooling.

```sh
go build ./...                 # build everything
go run ./cmd/myself version    # run the CLI
go test ./...                  # unit and golden CLI tests
go test ./internal/cli -update # regenerate internal/cli/testdata/*.golden
```

CLI tests build the binary once; each row of `cases` in `internal/cli/e2e_test.go` runs a sequence of commands against one temp store. uuids and times are normalized in the golden files (`<uuid-N>`, `<time>`), and a step can pass `<uuid-N>` to refer to an earlier uuid. Add rows for each new command, run with `-update` and review the golden diff.

Build info is injected with `-ldflags`:

```sh
go build -ldflags "-X github.com/mansus-org/myself/internal/cli.Version=v0.1.0 \
  -X github.com/mansus-org/myself/internal/cli.Commit=$(git rev-parse --short HEAD) \
  -X github.com/mansus-org/myself/internal/cli.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" ./cmd/myself
```


## License

See [LICENSE](LICENSE).
