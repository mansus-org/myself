# Demo

`demo.cast` is an [asciinema](https://asciinema.org) v2 recording of a real `myself` session against a throwaway store. `demo.svg` is the same recording rendered as an animated SVG, shown in the main README. The landing page plays a copy of the cast, `site/assets/demo.cast`, with `site/assets/player.js`.

The cast is generated, not recorded by hand. `gen.go` "types" each command with per-character delays, runs it for real against a temp `MYSELF_FILE`, and writes the real output (uuids and times included) with timestamps.

## Regenerate

```sh
go build -o /tmp/myself ./cmd/myself
go run demo/gen.go -bin /tmp/myself -out demo/demo.cast
bunx svg-term-cli --in demo/demo.cast --out demo/demo.svg --window --width 90 --height 26
cp demo/demo.cast site/assets/demo.cast
```

Then paste the finished session into the player window of `site/index.html` (it is what people see without JS or with reduced motion). A test checks the two cast files match.

Play the cast locally with `asciinema play demo/demo.cast`.

To change the script, edit the `commands` list in `gen.go`.
