//go:build ignore

// gen records an asciinema v2 cast of a real myself session.
//
//	go run demo/gen.go -bin ./myself -out demo/demo.cast
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var commands = []string{
	`myself remember relay is project`,
	`myself remember relay uses bun`,
	`myself remember felipe related relay`,
	`myself remember bun is tool`,
	`myself describe relay --depth 1`,
	`myself path bun felipe`,
	`myself remember i does "Felipe's coding agent. Answers tersely."`,
	`myself remember i does "Felipe's coding agent. Answers tersely. Never commits with the work email." --replace`,
	`myself history i`,
	`myself export --obsidian /tmp/vault && cat /tmp/vault/relay.md`,
}

const prompt = "\x1b[32m$\x1b[0m "

func main() {
	bin := flag.String("bin", "myself", "path to the myself binary")
	out := flag.String("out", "demo/demo.cast", "cast file to write")
	flag.Parse()

	abs, err := filepath.Abs(*bin)
	check(err)
	work, err := os.MkdirTemp("", "myself-demo")
	check(err)
	defer os.RemoveAll(work)
	binDir := filepath.Join(work, "bin")
	check(os.MkdirAll(binDir, 0o755))
	check(os.Symlink(abs, filepath.Join(binDir, "myself")))
	vault := filepath.Join(work, "vault")

	var env []string
	for _, e := range os.Environ() {
		switch {
		case strings.HasPrefix(e, "MYSELF_URL="), strings.HasPrefix(e, "MYSELF_TOKEN="), strings.HasPrefix(e, "MYSELF_TIMEOUT="), strings.HasPrefix(e, "MYSELF_FILE="):
		default:
			env = append(env, e)
		}
	}
	env = append(env,
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"MYSELF_FILE="+filepath.Join(work, "brain.jsonl"),
		"MYSELF_NUDGE=off",
	)

	rng := rand.New(rand.NewSource(40))
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	check(enc.Encode(map[string]any{
		"version": 2, "width": 90, "height": 26,
		"timestamp": time.Now().Unix(),
		"env":       map[string]string{"SHELL": "/bin/sh", "TERM": "xterm-256color"},
	}))

	var t float64
	emit := func(s string) {
		check(enc.Encode([]any{round(t), "o", s}))
	}

	t = 0.5
	for _, c := range commands {
		emit(prompt)
		t += 0.4
		for _, r := range c {
			emit(string(r))
			t += 0.04 + rng.Float64()*0.07
		}
		t += 0.35
		emit("\r\n")

		cmd := exec.Command("sh", "-c", strings.ReplaceAll(c, "/tmp/vault", vault))
		cmd.Env = env
		o, _ := cmd.CombinedOutput()
		res := strings.ReplaceAll(string(o), vault, "/tmp/vault")
		t += 0.15
		for _, line := range strings.SplitAfter(res, "\n") {
			if line == "" {
				continue
			}
			emit(strings.ReplaceAll(line, "\n", "\r\n"))
			t += 0.06
		}
		t += 1.8
	}
	emit(prompt)
	t += 2
	emit("")

	check(os.WriteFile(*out, buf.Bytes(), 0o644))
	fmt.Printf("wrote %s (%.0fs)\n", *out, t)
}

func round(f float64) float64 { return float64(int(f*1000+0.5)) / 1000 }

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
