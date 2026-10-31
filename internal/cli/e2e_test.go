package cli_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "regenerate testdata/*.golden")

const uuidPat = `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`

const ldflags = "-X github.com/mansus-org/myself/internal/cli.Version=v0.0.0-test " +
	"-X github.com/mansus-org/myself/internal/cli.Commit=abc1234 " +
	"-X github.com/mansus-org/myself/internal/cli.BuildDate=2026-01-01T00:00:00Z"

var binary string

func TestMain(m *testing.M) {
	flag.Parse()
	dir, err := os.MkdirTemp("", "myself-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "myself")
	build := exec.Command("go", "build", "-ldflags", ldflags, "-o", binary, "github.com/mansus-org/myself/cmd/myself")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// Each case is a sequence of steps run against one fresh temp store
// (MYSELF_FILE) and compared with testdata/<name>.golden: per step the
// command, stdin, stdout, stderr and exit code, then the store file.
//
// uuids and times come from the real clock, so the golden output is
// normalized: each distinct uuid becomes <uuid-N> in order of first
// appearance within the case, every "time":"..." value becomes <time>, and
// the temp store path becomes $STORE. A step's args may use <uuid-N> to refer
// to a uuid printed by an earlier step.
type step struct {
	args  []string
	stdin string
	dump  string // directory (under $TMP) whose files are printed after the step
}

func run(args ...string) step { return step{args: args} }

func pipe(stdin string, args ...string) step { return step{args: args, stdin: stdin} }

// dumping runs the step, then prints every file of dir (under $TMP).
func dumping(dir string, args ...string) step { return step{args: args, dump: dir} }

var cases = []struct {
	name  string
	steps []step
}{
	{"version", []step{run("version")}},
	{"version_json", []step{run("version", "--json")}},
	{"unknown_command", []step{run("frobnicate")}},
	{"bad_flag", []step{run("version", "--nope")}},

	{"remember", []step{
		run("remember", "Relay App", "is", "project"),
		run("remember", "relay", "uses", "bun", "for", "the", "backend,", "biome for linting"),
	}},
	{"remember_are", []step{run("remember", "project", "ARE", "software")}},
	{"remember_bad_relation", []step{run("remember", "relay", "built-with", "bun")}},
	{"remember_reserved", []step{run("remember", "relay", "forget", "bun")}},
	{"remember_empty_object", []step{
		run("remember", "relay", "uses"),
		run("remember", "relay", "uses", "  "),
	}},
	{"remember_usage", []step{run("remember", "relay")}},
	{"remember_json", []step{
		run("--json", "remember", "relay", "is", "project"),
		run("remember", "relay", "nope", "x", "--json"),
	}},
	{"remember_bulk", []step{
		pipe("relay is project\n\n  project are   software  \nrelay uses bun", "remember", "-"),
	}},
	{"remember_bulk_quotes", []step{
		pipe("relay has 'repo at /tmp/x'\nrelay does \"a b\"\nrelay uses 'mismatched\"\nrelay has it's fine\nrelay has ''\n", "remember", "-"),
		pipe("relay has 'repo at /tmp/x'\nrelay does \"a b\"\nrelay uses 'mismatched\"\nrelay has it's fine\n", "remember", "-"),
		run("remember", "relay", "has", "'kept'"),
	}},
	{"remember_is_sentence", []step{
		run("remember", "relay", "is", "project"),
		run("remember", "relay", "is", "a", "micro", "social", "network"),
		run("remember", "project", "ARE", "things", "you", "make", "often"),
		run("remember", "relay", "does", "a", "micro", "social", "network"),
		pipe("qa is a scratch project here\nqa is project\n", "remember", "-"),
	}},
	{"remember_bulk_bad_line", []step{
		pipe("relay is project\nproject is software\nrelay built-with bun\n", "remember", "-"),
		pipe("relay is project\nlonely\n", "remember", "-"),
	}},
	{"remember_replace", []step{
		run("remember", "relay", "uses", "bun"),
		run("remember", "relay", "uses", "deno"),
		run("remember", "relay", "is", "project"),
		run("remember", "relay", "uses", "node", "--replace"),
		run("remember", "--replace", "--json", "relay", "uses", "go"),
		run("remember", "relay", "has", "tests", "--replace", "--json"),
	}},

	{"forget_uuid", []step{
		run("remember", "relay", "uses", "bun"),
		run("forget", "<uuid-1>"),
		run("forget", "<uuid-1>"),
	}},
	{"forget_subject_relation", []step{
		run("remember", "relay", "uses", "bun"),
		run("remember", "relay", "uses", "biome"),
		run("remember", "relay", "is", "project"),
		run("forget", "Relay", "uses"),
		run("forget", "relay", "uses"),
	}},
	{"forget_object", []step{
		run("remember", "relay", "is", "project"),
		run("remember", "relay", "is", "Web App"),
		run("forget", "relay", "are", "web", "app"),
		run("forget", "relay", "is", "nothing"),
	}},
	{"forget_reason", []step{
		run("remember", "relay", "uses", "bun for the backend"),
		run("forget", "relay", "uses", "--reason", "moved off bun"),
	}},
	{"forget_json", []step{
		run("remember", "relay", "uses", "bun"),
		run("forget", "<uuid-1>", "--json", "--reason", "gone"),
		run("forget", "<uuid-1>", "--json"),
	}},
	{"forget_usage", []step{
		run("forget", "relay"),
		run("forget", "relay", "forget"),
		run("forget", "00000000-0000-7000-8000-000000000000"),
	}},

	{"worked_example", []step{
		run("remember", "relay", "is", "project"),
		run("remember", "relay", "uses", "bun for the backend, biome for linting, deployed on k3s"),
		run("remember", "relay", "built-with", "bun"),
		run("remember", "project", "are", "software"),
		run("describe", "relay"),
		run("describe", "relay", "--depth", "1"),
		run("ls", "software"),
		run("path", "relay", "software"),
		run("forget", "relay", "uses", "--reason", "moved off bun"),
		run("describe", "relay"),
		run("history", "relay"),
		run("history", "relay", "--json"),
		dumping("vault", "export", "--obsidian", "./vault"),
	}},
	{"json_no_html_escape", []step{
		run("remember", "a", "uses", "x < y && z > w", "--json"),
		run("remember", "b", "is", "a"),
		run("prime", "--json"),
		run("log", "--json"),
	}},
	{"describe_json", []step{
		run("remember", "relay", "is", "project"),
		run("remember", "project", "is", "software"),
		run("describe", "Relay", "--depth", "2", "--json"),
		run("describe", "software", "--json", "--at", "2999-01-01"),
	}},
	{"describe_at", []step{
		run("remember", "relay", "is", "project"),
		run("describe", "relay", "--at", "2999-01-01"),
		run("describe", "relay", "--at", "2000-01-01"),
		run("describe", "relay", "--at", "2000-01-01", "--json"),
		run("describe", "relay", "--at", "yesterday"),
	}},
	{"describe_errors", []step{
		run("remember", "relay", "is", "project"),
		run("describe"),
		run("describe", "a", "b"),
		run("describe", "relay", "--depth", "-1"),
		run("describe", "nothing", "--json"),
	}},
	{"ls", []step{
		run("remember", "relay", "is", "project"),
		run("remember", "project", "are", "software"),
		run("remember", "tool", "is", "software"),
		run("remember", "relay", "uses", "bun"),
		run("ls"),
		run("ls", "software"),
		run("ls", "relay"),
		run("ls", "nothing"),
		run("ls", "software", "--json"),
		run("ls", "--json"),
		run("ls", "relay", "--json"),
		run("ls", "software", "--at", "2000-01-01"),
		run("ls", "--at", "2000-01-01", "--json"),
		run("ls", "a", "b"),
	}},
	{"relations", []step{
		run("relations"),
		run("relations", "--json"),
		run("relations", "x"),
	}},
	{"search", []step{
		run("remember", "relay", "is", "project"),
		run("remember", "relay", "uses", "Bun for the backend"),
		run("remember", "docs", "uses", "markdown"),
		run("search", "BUN"),
		run("search", "for", "the"),
		run("search", "is"),
		run("search", "relay", "--json"),
		run("search", "zzz"),
		run("search", "zzz", "--json"),
		run("search", "bun", "--at", "2000-01-01"),
		run("search"),
		run("forget", "relay", "uses"),
		run("search", "bun"),
	}},
	{"path", []step{
		run("remember", "relay", "is", "project"),
		run("remember", "project", "is", "software"),
		run("remember", "docs", "has", "software"),
		run("remember", "lonely", "is", "thing"),
		run("path", "relay", "software"),
		run("path", "software", "relay"),
		run("path", "relay", "docs", "--json"),
		run("path", "relay", "docs", "--max-depth", "2"),
		run("path", "relay", "docs", "--max-depth", "3"),
		run("path", "relay", "lonely"),
		run("path", "relay", "nothing"),
		run("path", "relay", "lonely", "--json"),
		run("path", "relay"),
		run("path", "relay", "docs", "--max-depth", "0"),
	}},

	{"log", []step{
		run("log"),
		run("remember", "relay", "is", "project"),
		run("remember", "relay", "uses", "bun for the backend"),
		run("forget", "relay", "uses", "--reason", "moved off bun"),
		run("log"),
		run("log", "-n", "2"),
		run("log", "--since", "2999-01-01"),
		run("log", "--since", "2000-01-01", "--json"),
		run("log", "--since", "yesterday"),
		run("log", "-n", "0"),
		run("log", "extra"),
	}},
	{"history", []step{
		run("remember", "relay", "is", "project"),
		run("remember", "relay", "uses", "bun for the backend, biome for linting, deployed on k3s"),
		run("remember", "project", "are", "software"),
		run("forget", "relay", "uses", "--reason", "moved off bun"),
		run("history", "relay"),
		run("history", "Relay", "uses"),
		run("history", "relay", "are"),
		run("history", "relay", "--json"),
		run("history", "project"),
		run("history", "software"),
		run("history", "relay", "built-with"),
		run("history"),
	}},
	{"prime", []step{
		run("prime"),
		run("remember", "relay", "is", "project"),
		run("remember", "relay", "uses", "bun"),
		run("remember", "project", "are", "software"),
		run("prime"),
		run("prime", "--budget", "40"),
		run("prime", "--budget", "40", "--json"),
		run("prime", "--json"),
		run("prime", "--budget", "0"),
	}},
	{"prime_order", []step{
		pipe("zeta has a\nzeta has b\nzeta has c\nzeta has d\nalpha is tool\nbeta is tool\ngamma is tool\nrelay is project\nself is agent\nself related felipe\nfelipe is person\nself related mansus\nmansus is org\nsolo related self\n", "remember", "-"),
		run("prime"),
		run("prime", "--budget", "60"),
	}},
	{"dream", []step{
		run("dream"),
		run("remember", "relay", "is", "project"),
		run("remember", "relays", "is", "project"),
		run("remember", "web-app", "has", "tests"),
		run("remember", "webapp", "has", "docs"),
		run("remember", "webapps", "has", "docs"),
		run("remember", "bun", "is", "tool"),
		run("dream"),
		run("dream", "--json"),
	}},
	{"export", []step{
		run("export", "--obsidian", "$TMP/vault", "--json"),
		run("remember", "relay", "is", "project"),
		run("remember", "relay", "uses", "bun for the backend, biome for linting, deployed on k3s"),
		run("remember", "project", "are", "software"),
		run("forget", "relay", "uses"),
		run("remember", "relay", "has", "Project"),
		dumping("vault", "export", "--obsidian", "$TMP/vault"),
		run("export", "--obsidian", "$TMP/vault", "--json"),
		run("export"),
	}},
	{"store_errors", []step{
		run("remember", "relay", "is", "project", "--file", "/dev/null/brain.jsonl"),
		run("remember", "relay", "is", "project", "--json", "--file", "/dev/null/brain.jsonl"),
		run("forget", "relay", "is", "--file", "/dev/null/brain.jsonl"),
	}},
}

var (
	uuidRe = regexp.MustCompile(uuidPat)
	// timeRe matches a record time: the JSON "time" value, or the column after
	// the uuid in log, history and search lines.
	timeRe = regexp.MustCompile(`("time":"|` + uuidPat + ` )\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`)
	refRe  = regexp.MustCompile(`<uuid-\d+>`)
)

// normalizer maps real uuids to <uuid-N> and back within one case.
type normalizer struct {
	store string
	tmp   string
	names map[string]string // uuid -> <uuid-N>
	uuids map[string]string // <uuid-N> -> uuid
}

func (n *normalizer) text(s string) string {
	s = strings.ReplaceAll(s, n.store, "$STORE")
	s = strings.ReplaceAll(s, n.tmp, "$TMP")
	s = timeRe.ReplaceAllString(s, "${1}<time>")
	return uuidRe.ReplaceAllStringFunc(s, func(u string) string {
		if name, ok := n.names[u]; ok {
			return name
		}
		name := fmt.Sprintf("<uuid-%d>", len(n.names)+1)
		n.names[u], n.uuids[name] = name, u
		return name
	})
}

func (n *normalizer) arg(s string) string {
	return refRe.ReplaceAllStringFunc(s, func(name string) string {
		if u, ok := n.uuids[name]; ok {
			return u
		}
		return name
	})
}

func TestGolden(t *testing.T) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "brain.jsonl")
			n := &normalizer{store: file, tmp: filepath.Dir(file), names: map[string]string{}, uuids: map[string]string{}}
			var got strings.Builder
			for _, st := range tc.steps {
				args := make([]string, len(st.args))
				for i, a := range st.args {
					args[i] = strings.ReplaceAll(n.arg(a), "$TMP", n.tmp)
				}
				var stdout, stderr bytes.Buffer
				cmd := exec.Command(binary, args...)
				cmd.Dir = n.tmp
				cmd.Stdin = strings.NewReader(st.stdin)
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				cmd.Env = append(os.Environ(), "MYSELF_FILE="+file)
				code := 0
				if err := cmd.Run(); err != nil {
					var ee *exec.ExitError
					if !errors.As(err, &ee) {
						t.Fatal(err)
					}
					code = ee.ExitCode()
				}
				fmt.Fprintf(&got, "$ myself %s\n", shellJoin(st.args))
				if st.stdin != "" {
					fmt.Fprintf(&got, "--- stdin\n%s\n", strings.TrimSuffix(st.stdin, "\n"))
				}
				fmt.Fprintf(&got, "--- stdout\n%s--- stderr\n%s--- exit\n%d\n",
					n.text(stdout.String()), n.text(stderr.String()), code)
				if st.dump != "" {
					got.WriteString(dumpDir(t, n, filepath.Join(n.tmp, st.dump)))
				}
			}
			if b, err := os.ReadFile(file); err == nil {
				fmt.Fprintf(&got, "--- file\n%s", n.text(string(b)))
			} else if !errors.Is(err, fs.ErrNotExist) {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", tc.name+".golden")
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got.String()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/cli -update)", err)
			}
			if got.String() != string(want) {
				t.Errorf("mismatch\n got:\n%s\nwant:\n%s", got.String(), want)
			}
		})
	}
}

// shellJoin renders args for the "$ myself ..." line, quoting the ones a
// shell would split.
func shellJoin(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\n\"'") {
			a = strconv.Quote(a)
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}

// dumpDir prints each file of dir, sorted, as "--- <name>" and its content.
func dumpDir(t *testing.T, n *normalizer, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "--- file %s\n%s", e.Name(), n.text(string(data)))
	}
	return b.String()
}
