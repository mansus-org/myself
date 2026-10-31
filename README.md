# myself

`myself` is a tiny single-binary CLI (Go) that any agent harness (Claude Code, Codex, pi) calls through plain Bash to save and query entities, types and relationships: a personal knowledge graph in one command. Facts are triples that are superseded, never overwritten; the whole brain is one append-only JSONL file.

Status: in design, not released.

The contract for the CLI is in [SPEC.md](SPEC.md).

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

Install: `go install github.com/mansus-org/myself/cmd/myself@latest`.
