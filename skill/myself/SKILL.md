---
name: myself
description: Your long-term memory across sessions, via the `myself` CLI, a personal knowledge graph of facts about people, projects, tools and the agent itself. Use it, not CLAUDE.md or a memory file, whenever the user says "remember…", "remember that…", "don't forget…", or corrects how you work ("don't do X", "always Y", "I hate X"). Also use when you learn a durable fact or decision, when the user mentions a person, project or tool you might know about, when they ask "what do you know about…", "who is…" or "what changed lately", and at session start and end. Use before answering "I don't know" about any person, project or tool.
---

# myself

`myself` stores facts as `subject relation object` triples in one append-only log (`~/.myself/brain.jsonl`). Facts are superseded, never edited. Call it with Bash. The full contract is SPEC.md in github.com/mansus-org/myself.

If `myself` is not installed (`command -v myself` fails), install it first: `curl -fsSL https://raw.githubusercontent.com/mansus-org/myself/main/install.sh | sh` (puts it in `~/.local/bin`; add that to PATH if needed). To use a shared brain server instead of a local file, set `MYSELF_URL` and `MYSELF_TOKEN`.

## When to use

`myself` is your long-term memory. When the user says "remember…" or corrects you, write it with `myself`, not to CLAUDE.md or a file-based memory directory.

- **Recall** before guessing: a name you don't recognise, "what do you know about X", "how does X relate to Y", "what happened lately".
- **Write** when something durable is learned: a new project, person or tool and how they relate, a decision with lasting effect, a correction to how you work.
- **Session start:** a SessionStart hook may already have injected `myself prime`. If not, run `myself prime`.
- **Session end:** see the rules below. A Stop hook may nudge you once.

## Cheat sheet

```
myself remember <subject> <relation> <object...> [--replace]   # add a fact; --replace supersedes same subject+relation
myself remember -                       # bulk: one "subject relation object..." per stdin line, all-or-nothing
myself forget <uuid> [--reason TEXT]    # retract one record
myself forget <subject> <relation> [<object...>] [--reason TEXT]   # retract matching live records
myself describe <entity> [--depth N] [--at TIME]   # facts out of and into an entity, N hops out
myself ls [<type>] [--at TIME]          # entities that are (transitively) <type>; no arg: all types
myself relations                        # the closed relation list
myself search <text> [--at TIME]        # substring match over live facts
myself path <a> <b> [--max-depth N]     # shortest chain of facts linking two entities
myself log [--since TIME] [-n N]        # every record in order, forgets included
myself history <subject> [<relation>]   # all records ever for a subject, forgotten ones too
myself prime [--budget CHARS]           # compact brain summary for context
myself dream                            # report near-duplicate subjects; writes nothing
myself export --obsidian <dir>          # one markdown note per entity
```

Global flags: `--json`, `--file PATH`. TIME is RFC 3339 or `YYYY-MM-DD`. Exit 0 ok, 1 usage, 2 not found, 3 store error. Everything after the relation is the object; quote it when it has shell characters (`'…'`, or use a heredoc).

## Recall strategy

- Known name: `myself describe relay`; add `--depth 1` to see its neighbours.
- Unknown or partial name: `myself search consenso` (or a word from the topic).
- "How are X and Y related?": `myself path x y`.
- "What changed lately?": `myself log --since 2026-10-24` or `myself log -n 20`.
- "What projects/people do I know?": `myself ls` for the types, then `myself ls project`.
- About yourself or your owner: `myself describe i --depth 1`.
- Exit 2 means unknown, not an error: fall back to `search`.
- **Never answer "I don't know" about a person, project or tool before trying `myself`.**

## Writing conventions

Relations are a closed set. Pick the closest; the object sentence carries the nuance.

| relation | use for | example |
|----------|---------|---------|
| `is` (`are`) | type or category; object is a short entity name | `relay is project`, `project is software` |
| `has` | parts, attributes, locations, links | `consenso has 'repo at github.com/nemoNoboru/consenso'` |
| `uses` | tools, libraries, services it depends on | `relay uses bun`, `relay uses 'biome for linting'` |
| `does` | what it does, its role or job | `bun does 'runs, tests and bundles JavaScript'` |
| `prefers` | preferences, rules, constraints | `felipe prefers 'small PRs, one ticket each'` |
| `related` | any other link between two entities | `relay related consenso` |

- **Type every new entity with `is`** the first time you write it: `myself remember hono is library`. An `is` object is a short type name (`project`, `tool`), never a sentence; details go in `has` or `does`.
- **Object = entity name when you'd want the link** (`relay uses bun` connects relay to bun), **a sentence otherwise** (`relay uses 'bun for the backend, biome for linting'`). Check the spelling of existing names with `describe`/`search` first; don't invent a variant.
- **Versions and details of a link go in `has`**, so the link survives: keep `relay uses postgres` and add `relay has 'postgres 17'`. Don't write a second `uses` whose object repeats an entity name inside a sentence; if the old fact is now wrong, use `--replace` or `forget`.
- **Provenance:** when a ticket or context matters, put it in the object: `relay prefers 'bun over npm (BRAIN-12)'`.
- **3+ facts:** one bulk write.

```sh
myself remember - <<'EOF'
consenso is project
consenso uses hono
hono is library
relay has repo at github.com/nemoNoboru/relay
EOF
```

  Don't quote objects inside `remember -`: each line's object is everything after the relation, as written.

- **Mistakes:** `myself forget <uuid>` (uuid from `describe --json`, `history` or `log`), or `myself remember x has 'new value' --replace` to supersede every live `x has`.
- **Self-model:** `i`, `me`, `myself` all mean the entity `self`.
  - Type: `myself remember i is agent`.
  - Identity: one paragraph under 600 chars, who I am and how I work: `myself remember i does '<paragraph>' --replace`. Always `myself describe i` first and rewrite the whole paragraph, keeping what is still true.
  - Preferences and constraints: `myself remember i prefers '<x>'`; phrase constraints as "never …" or "not …".
  - Owner: `myself remember i related felipe` (and `felipe is person` if new).
  - World facts go on the world entity (`relay uses bun`), not on `self`.

## Session-end rules

Before you finish a session that did real work, ask: what will matter next time?

Remember: new entities and how they relate; decisions with lasting effect (`relay prefers 'move from npm to bun (BRAIN-12)'`); corrections to how you work (molting, below); facts the user states about themselves or the world that matter beyond today.

Don't remember: anything derivable from the repo (code structure, git history), ephemeral task state ("tests are failing", "halfway through X"), anything already in CLAUDE.md, session summaries. Store facts about things, not a diary.

One fact per `remember`, bulk `remember -` for 3+. If nothing is durable, write nothing; when the Stop hook asks, just stop.

## Molting

Molting is updating your own self-model (`self`). Molt when:

1. **The user corrects your behaviour** ("don't do X", "always Y"): add `i prefers`, then `myself describe i` and rewrite the identity paragraph with `myself remember i does '<paragraph>' --replace` so it includes the new rule in one clause. Do both in the same turn. Skip the rewrite only for a rule about one repo or one task; rules about how you commit, write or work in general (even "in all X repos") get the rewrite.
2. **The same lesson shows up a second time**: check `myself history i` for a near-duplicate first.
3. **`dream` or `describe i` shows contradictions** in your self facts: forget the stale one.

Don't molt for one-off task instructions, things CLAUDE.md already says, or guesses about the user's preferences they haven't stated.

Examples:

- *Correction:* "Stop opening PRs stacked on other branches." → `myself remember i prefers 'never stack PRs on other branches; always target main'`, then `myself describe i` and rewrite the identity paragraph with "I always open PRs against main" folded in (`myself remember i does '…' --replace`).
- *Repeated lesson:* you're reminded a second time to run gofmt before pushing. `myself history i` shows `prefers 'run gofmt before committing'` already. → `myself describe i`, then rewrite the identity paragraph to include "I run the repo's formatter and linters before every push" with `myself remember i does '…' --replace`.
- *Non-trigger:* "Use port 8081 for this preview." That's a task instruction, so remember nothing.

## Dream recipe

When asked to dream (consolidate the brain):

1. `myself dream --json` lists near-duplicate subjects, e.g. `{"near_duplicates":[["relay","relays"]]}`.
2. For each group, pick the name to keep (more facts, the one used as an object elsewhere: check `describe`). Move each fact of the other name with `myself remember <kept> <relation> <object>`, then `myself forget <other> <relation>` (or by uuid). Re-point incoming facts too (the `<-` lines of `describe <other>`): write them again with `<kept>` as the object and forget the old ones. Ask the user when the merge is not obvious.
3. Run `myself ls` and `describe` every subject: any entity with no `is` fact is untyped. Type it with its obvious general category (`grafana is tool`, `postgres is database`); types are cheap to change. Ask only when the name is ambiguous. List anything you left untyped in the summary.
4. Finish with a one-paragraph summary of what you merged, typed and left alone.

Richer reports (untyped entities, stale facts, self-model size) are future work in BRAIN-32; until then do those checks by hand.

## Never store

- Secrets, passwords, API keys, tokens, or config files that contain them.
- The user's work email. Other email addresses only when the user gives one as an identity to use (e.g. a commit author): then store it whole, `i prefers 'commit in nemoNoboru repos as felipetavres <felipetavres@gmail.com>'`.
- Other people's private data (health, finances, personal contact details).
- Anything derivable from the repo, or ephemeral task state.
