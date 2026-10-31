package cli_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestSpecWorkedExample replays the code blocks of SPEC.md section 9 against
// the real binary and a fresh store. The spec is parsed, not copied: a `$ `
// line is a command, `stderr: ` lines are expected stderr, `(exit N...)` is
// the expected exit code and every other line is expected stdout. The uuids
// and timestamps in the spec are illustrations, so they are mapped to the real
// ones by order of appearance (see aligner).
func TestSpecWorkedExample(t *testing.T) {
	raw, err := os.ReadFile("../../SPEC.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(raw), "\n## 9. Worked example\n")
	if !ok {
		t.Fatal("SPEC.md has no section 9")
	}
	var cmds []specCmd
	var notes map[string]string
	for _, block := range codeBlocks(section) {
		switch {
		case strings.HasPrefix(block, "$ "):
			cmds = append(cmds, parseSpecCmds(t, block)...)
		case strings.Contains(block, "vault/"):
			notes = parseNoteColumns(t, block)
		}
	}
	if len(cmds) != 13 {
		t.Fatalf("parsed %d commands from section 9, want 13 (did the spec change?)", len(cmds))
	}
	if len(notes) != 3 {
		t.Fatalf("parsed %d notes from section 9, want 3", len(notes))
	}

	tmp := t.TempDir()
	file := filepath.Join(tmp, "brain.jsonl")
	al := &aligner{uuids: map[string]string{}, times: map[string]string{}}
	for _, c := range cmds {
		before := lineCount(t, file)
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(binary, c.args...)
		cmd.Dir = tmp
		cmd.Env = append(os.Environ(), "MYSELF_FILE="+file)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				t.Fatal(err)
			}
			code = ee.ExitCode()
		}
		if code != c.exit {
			t.Errorf("$ %s: exit %d, want %d", c.line, code, c.exit)
		}
		al.check(t, c.line+" (stdout)", c.stdout, stdout.String())
		al.check(t, c.line+" (stderr)", c.stderr, stderr.String())
		if c.nothingWritten && lineCount(t, file) != before {
			t.Errorf("$ %s: wrote to the store, spec says nothing written", c.line)
		}
	}
	al.checkTimeOrder(t)

	for name, want := range notes {
		got, err := os.ReadFile(filepath.Join(tmp, "vault", name))
		if err != nil {
			t.Errorf("note %s: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("note %s mismatch\n got:\n%s\nwant:\n%s", name, got, want)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(tmp, "vault")); len(entries) != len(notes) {
		t.Errorf("vault has %d files, spec shows %d notes", len(entries), len(notes))
	}
}

type specCmd struct {
	line           string
	args           []string
	stdout, stderr string
	exit           int
	nothingWritten bool
}

func codeBlocks(s string) []string {
	var out []string
	parts := strings.Split(s, "```")
	for i := 1; i < len(parts); i += 2 {
		out = append(out, strings.TrimPrefix(parts[i], "\n"))
	}
	return out
}

var exitRe = regexp.MustCompile(`^\(exit (\d+)([,)].*)?`)

func parseSpecCmds(t *testing.T, block string) []specCmd {
	var cmds []specCmd
	for _, l := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		switch {
		case strings.HasPrefix(l, "$ myself "):
			cmds = append(cmds, specCmd{line: l[2:], args: shellSplit(t, strings.TrimPrefix(l, "$ myself "))})
		case len(cmds) == 0:
			t.Fatalf("line before any command: %q", l)
		case strings.HasPrefix(l, "stderr: "):
			c := &cmds[len(cmds)-1]
			c.stderr += strings.TrimPrefix(l, "stderr: ") + "\n"
		case exitRe.MatchString(l):
			c := &cmds[len(cmds)-1]
			c.exit, _ = strconv.Atoi(exitRe.FindStringSubmatch(l)[1])
			c.nothingWritten = strings.Contains(l, "nothing written")
		default:
			c := &cmds[len(cmds)-1]
			c.stdout += l + "\n"
		}
	}
	return cmds
}

// shellSplit splits on spaces, honouring double quotes.
func shellSplit(t *testing.T, s string) []string {
	var out []string
	var cur strings.Builder
	in, has := false, false
	for _, r := range s {
		switch {
		case r == '"':
			in, has = !in, true
		case r == ' ' && !in:
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	if in {
		t.Fatalf("unterminated quote in %q", s)
	}
	if has {
		out = append(out, cur.String())
	}
	return out
}

// parseNoteColumns reads the side-by-side display of the three notes: the
// header line names the files, each column starts where its header starts.
func parseNoteColumns(t *testing.T, block string) map[string]string {
	lines := strings.Split(strings.TrimRight(block, "\n"), "\n")
	hdr := regexp.MustCompile(`vault/\S+`)
	locs := hdr.FindAllStringIndex(lines[0], -1)
	cols := make([][]string, len(locs))
	for _, l := range lines[1:] {
		for i, loc := range locs {
			end := len(l)
			if i+1 < len(locs) {
				end = min(locs[i+1][0], len(l))
			}
			cell := ""
			if loc[0] < len(l) {
				cell = strings.TrimSpace(l[loc[0]:end]) // the spec drifts by a column in places
			}
			cols[i] = append(cols[i], cell)
		}
	}
	notes := map[string]string{}
	for i, loc := range locs {
		name := strings.TrimPrefix(lines[0][loc[0]:loc[1]], "vault/")
		notes[name] = strings.TrimRight(strings.Join(cols[i], "\n"), "\n") + "\n"
	}
	return notes
}

func lineCount(t *testing.T, file string) int {
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "\n")
}

var timeValRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`)

// aligner maps the spec's illustrative uuids and times to the real ones.
// Expected and actual text must contain the same number of each in the same
// places; the first time a spec value is seen it is bound to the real value
// there, afterwards it must match the same real value.
type aligner struct {
	uuids, times map[string]string // spec -> real
}

func (a *aligner) check(t *testing.T, what, want, got string) {
	t.Helper()
	wu, gu := uuidRe.FindAllString(want, -1), uuidRe.FindAllString(got, -1)
	wt, gt := timeValRe.FindAllString(want, -1), timeValRe.FindAllString(got, -1)
	if len(wu) != len(gu) || len(wt) != len(gt) {
		t.Errorf("%s: uuid/time count differs\n got:\n%s\nwant:\n%s", what, got, want)
		return
	}
	for i := range wu {
		if prev, ok := a.uuids[wu[i]]; ok && prev != gu[i] {
			t.Errorf("%s: spec uuid %s was %s, now %s", what, wu[i], prev, gu[i])
		}
		a.uuids[wu[i]] = gu[i]
	}
	for i := range wt {
		if prev, ok := a.times[wt[i]]; ok && prev != gt[i] {
			t.Errorf("%s: spec time %s was %s, now %s", what, wt[i], prev, gt[i])
		}
		a.times[wt[i]] = gt[i]
	}
	if !a.sameUnder(want, got) {
		t.Errorf("%s mismatch\n got:\n%s\nwant:\n%s", what, got, want)
	}
}

// sameUnder reports whether want, with spec uuids and times replaced by their
// real counterparts, equals got.
func (a *aligner) sameUnder(want, got string) bool {
	w := uuidRe.ReplaceAllStringFunc(want, func(u string) string { return a.uuids[u] })
	w = timeValRe.ReplaceAllStringFunc(w, func(s string) string { return a.times[s] })
	return w == got
}

// checkTimeOrder: the spec's times increase, so the real ones must not go
// backwards.
func (a *aligner) checkTimeOrder(t *testing.T) {
	t.Helper()
	var spec []string
	for k := range a.times {
		spec = append(spec, k)
	}
	for _, x := range spec {
		for _, y := range spec {
			if x < y && a.times[x] > a.times[y] {
				t.Errorf("time order: spec %s<%s but real %s>%s", x, y, a.times[x], a.times[y])
			}
		}
	}
}
