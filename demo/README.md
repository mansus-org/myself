# Demo

`demo.cast` is an [asciinema](https://asciinema.org) v2 recording of a real `myself` session against a throwaway store. `demo.svg` is the same recording rendered as an animated SVG, shown in the main README.

The cast is generated, not recorded by hand. `gen.go` "types" each command with per-character delays, runs it for real against a temp `MYSELF_FILE`, and writes the real output (uuids and times included) with timestamps.

## Regenerate

```sh
go build -o /tmp/myself ./cmd/myself
go run demo/gen.go -bin /tmp/myself -out demo/demo.cast
bunx svg-term-cli --in demo/demo.cast --out demo/demo.svg --window --width 90 --height 26
```

Play the cast locally with `asciinema play demo/demo.cast`.

To change the script, edit the `commands` list in `gen.go`.
