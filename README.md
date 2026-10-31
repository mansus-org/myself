# myself

`myself` is a tiny single-binary CLI (Go) that any agent harness (Claude Code, Codex, pi) calls through plain Bash to save and query entities, types and relationships: a personal knowledge graph in one command. Facts are triples that are superseded, never overwritten; the whole brain is one append-only JSONL file.

Status: early development, no release yet. Every command in SPEC.md is implemented.

The contract for the CLI is in [SPEC.md](SPEC.md).

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/mansus-org/myself/main/install.sh | sh
```

This installs the latest release to `~/.local/bin` (Linux and macOS, amd64 and arm64) after verifying its sha256 checksum. Set `MYSELF_VERSION=v0.1.0` to pin a release and `MYSELF_INSTALL_DIR` to change the target directory.

Or with Go: `go install github.com/mansus-org/myself/cmd/myself@latest`.

## Install on an agent host

Set up Claude Code so every session starts with the brain in context and uses `myself` through the skill.

1. Install the binary (see [Install](#install); until the first release is published use `go install`):

   ```sh
   curl -fsSL https://raw.githubusercontent.com/mansus-org/myself/main/install.sh | sh
   ```

   The brain lives in `~/.myself/brain.jsonl` (override with `MYSELF_FILE` or `--file`). It is created on first write.

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

`session-start.sh` prints `myself prime --budget 2000` (set `MYSELF_PRIME_BUDGET` to change it), which Claude Code adds to the session context, and records the session start time under `${TMPDIR:-/tmp}/myself-hooks/`. `stop-nudge.sh` blocks the first stop of a session once, asking whether anything is worth remembering, but only if nothing was written to the brain since the session started and the session did something (a transcript of 10+ lines, or uncommitted changes in a git `cwd`). Both scripts are POSIX sh, use `jq` when present, and exit 0 silently when `myself` is missing or fails. The comments at the top of each script list their settings.

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

