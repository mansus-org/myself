# myself

`myself` is a tiny single-binary CLI (Go) that any agent harness (Claude Code, Paseo workers, pi) calls through plain Bash to save and query entities, types and relationships: a personal knowledge graph in one command. Facts are triples that are superseded, never overwritten; the whole brain is one append-only JSONL file.

Status: in design, not released.

## Landing page

The site lives in `site/` (plain HTML, CSS and JS, no build step) and deploys to https://myself.mansus.org on merge to `main`: the `Dockerfile` serves `site/` with busybox httpd, `.k3s/values.yaml` holds the cluster settings, `.github/workflows/deploy.yml` builds the image. Pull requests get a preview at `myself-pr-<number>.mansus.org`. The design rationale is in `design/README.md`.

Preview locally: `bunx serve site -l 8080`.
