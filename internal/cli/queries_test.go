package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mansus-org/myself/internal/querylog"
)

// `myself queries` on a local store reads what the last `myself serve` recorded
// next to it — which on a brain that was never served is nothing, and is still a
// plain exit 0.
func TestQueriesLocal(t *testing.T) {
	t.Setenv(EnvURL, "")
	t.Setenv(EnvToken, "")
	file := filepath.Join(t.TempDir(), "brain.jsonl")

	var out, errb bytes.Buffer
	if code := Execute([]string{"queries", "--file", file}, &out, &errb); code != 0 || out.String() != "" || errb.String() != "" {
		t.Fatalf("an empty log: exit %d, stdout %q, stderr %q", code, out.String(), errb.String())
	}

	q := querylog.Open(querylog.Path(file), nil)
	for _, e := range []querylog.Entry{
		{Time: "2026-01-01T00:00:00Z", Verb: "search", Text: "bun", Hits: 1, Facts: []string{"relay uses bun"}},
		{Time: "2026-01-01T00:01:00Z", Verb: "recall", Text: "what does relay run on", Hits: 1, Model: "m", Took: 12},
	} {
		if err := q.Add(e); err != nil {
			t.Fatal(err)
		}
	}

	out.Reset()
	errb.Reset()
	if code := Execute([]string{"queries", "--file", file}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	got := out.String()
	if !strings.Contains(got, `recall "what does relay run on" 1 fact in 12ms with m`) {
		t.Fatalf("newest first, with the model: %q", got)
	}
	if strings.Index(got, "recall") > strings.Index(got, "search") {
		t.Fatalf("newest first: %q", got)
	}

	out.Reset()
	if code := Execute([]string{"queries", "--file", file, "--verb", "search", "--json"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), `"verb":"search"`) || strings.Contains(out.String(), "recall") {
		t.Fatalf("--verb filters: %s", out.String())
	}

	out.Reset()
	errb.Reset()
	if code := Execute([]string{"queries", "--file", file, "--verb", "dream"}, &out, &errb); code != ExitUsage ||
		!strings.Contains(errb.String(), "unknown verb") {
		t.Fatalf("a bad verb is a usage error: exit %d, stderr %q", code, errb.String())
	}
}
