package cli_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "regenerate testdata/*.golden")

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

// Add a row per case. Each runs in a fresh temp store (MYSELF_FILE) and is
// compared with testdata/<name>.golden (stdout, stderr and exit code).
var cases = []struct {
	name  string
	args  []string
	stdin string
}{
	{"version", []string{"version"}, ""},
	{"version_json", []string{"version", "--json"}, ""},
	{"unknown_command", []string{"frobnicate"}, ""},
	{"bad_flag", []string{"version", "--nope"}, ""},
}

func TestGolden(t *testing.T) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := exec.Command(binary, tc.args...)
			cmd.Stdin = strings.NewReader(tc.stdin)
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			cmd.Env = append(os.Environ(), "MYSELF_FILE="+filepath.Join(t.TempDir(), "brain.jsonl"))
			code := 0
			if err := cmd.Run(); err != nil {
				var ee *exec.ExitError
				if !errors.As(err, &ee) {
					t.Fatal(err)
				}
				code = ee.ExitCode()
			}
			got := fmt.Sprintf("--- stdout\n%s--- stderr\n%s--- exit\n%d\n", stdout.String(), stderr.String(), code)
			golden := filepath.Join("testdata", tc.name+".golden")
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/cli -update)", err)
			}
			if got != string(want) {
				t.Errorf("mismatch for %v\n got:\n%s\nwant:\n%s", tc.args, got, want)
			}
		})
	}
}
