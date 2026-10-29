package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mansus-org/myself/internal/model"
)

const line1 = `{"uuid":"0192d7c4-0001-7000-8000-000000000001","subject":"relay","relation":"is","object":"project","time":"2026-10-29T16:00:00Z"}`
const line2 = `{"uuid":"0192d7c4-0002-7000-8000-000000000002","subject":"relay","relation":"uses","object":"bun","time":"2026-10-29T16:00:05Z"}`

const helperEnv = "STORE_TEST_HELPER"

// TestMain lets the test binary double as an append worker for the
// multi-process test: STORE_TEST_HELPER=<path>:<id>:<count>.
func TestMain(m *testing.M) {
	if spec := os.Getenv(helperEnv); spec != "" {
		var path string
		var id, count int
		i := strings.LastIndex(spec, ":")
		j := strings.LastIndex(spec[:i], ":")
		path = spec[:j]
		fmt.Sscanf(spec[j+1:], "%d:%d", &id, &count)
		for k := 0; k < count; k++ {
			// Two lines per call, to exercise the single multi-line write.
			a := model.Record{UUID: fmt.Sprintf("w%03d-%04d-a", id, k), Subject: "s", Relation: "is", Object: strings.Repeat("x", 3000), Time: "2026-10-29T16:00:00Z"}
			b := a
			b.UUID = fmt.Sprintf("w%03d-%04d-b", id, k)
			if err := Append(path, a, b); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func read(t *testing.T, in string) ([]model.Record, string) {
	t.Helper()
	var w bytes.Buffer
	recs, err := Read(strings.NewReader(in), &w)
	if err != nil {
		t.Fatal(err)
	}
	return recs, w.String()
}

func TestReadValid(t *testing.T) {
	recs, warn := read(t, line1+"\n"+line2+"\n")
	if warn != "" || len(recs) != 2 || recs[0].Subject != "relay" || recs[1].Relation != "uses" {
		t.Fatalf("recs=%+v warn=%q", recs, warn)
	}
}

func TestReadTrailingPartialLineIgnored(t *testing.T) {
	recs, warn := read(t, line1+"\n"+line2[:40])
	if warn != "" || len(recs) != 1 {
		t.Fatalf("recs=%+v warn=%q", recs, warn)
	}
}

func TestReadCorruptLineWarns(t *testing.T) {
	recs, warn := read(t, line1+"\nnot json\n{\"uuid\":\"\"}\n"+line2+"\n")
	if len(recs) != 2 {
		t.Fatalf("recs=%+v", recs)
	}
	want := "warning: line 2: corrupt, skipped\nwarning: line 3: corrupt, skipped\n"
	if warn != want {
		t.Fatalf("warn=%q want %q", warn, want)
	}
}

func TestReadSortedByUUID(t *testing.T) {
	recs, _ := read(t, line2+"\n"+line1+"\n")
	if recs[0].UUID > recs[1].UUID || recs[0].Relation != "is" {
		t.Fatalf("recs=%+v", recs)
	}
}

func TestReadLongLine(t *testing.T) {
	obj := strings.Repeat("a", 3<<20)
	line := fmt.Sprintf(`{"uuid":"0192d7c4-0003-7000-8000-000000000003","subject":"s","relation":"is","object":%q,"time":"2026-10-29T16:00:00Z"}`, obj)
	recs, warn := read(t, line1+"\n"+line+"\n"+line2+"\n")
	if warn != "" || len(recs) != 3 || recs[2].Object != obj {
		t.Fatalf("warn=%q n=%d", warn, len(recs))
	}
}

func TestReadFileMissingIsEmpty(t *testing.T) {
	recs, err := ReadFile(filepath.Join(t.TempDir(), "nope.jsonl"), nil)
	if err != nil || len(recs) != 0 {
		t.Fatalf("recs=%v err=%v", recs, err)
	}
}

func TestAppendRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "brain.jsonl")
	a := model.Record{UUID: "0192d7c4-0001-7000-8000-000000000001", Subject: "relay", Relation: "is", Object: "project", Time: "2026-10-29T16:00:00Z"}
	b := model.Record{UUID: "0192d7c4-0002-7000-8000-000000000002", Subject: "relay", Relation: "uses", Object: "bun", Time: "2026-10-29T16:00:05Z"}
	if err := Append(path, a); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, b); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != line1+"\n"+line2+"\n" {
		t.Fatalf("raw=%q", raw)
	}
	recs, err := ReadFile(path, nil)
	if err != nil || !reflect.DeepEqual(recs, []model.Record{a, b}) {
		t.Fatalf("recs=%v err=%v", recs, err)
	}
}

func TestAppendLockTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.jsonl")
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	old := lockTimeout
	lockTimeout = 100 * time.Millisecond
	defer func() { lockTimeout = old }()

	start := time.Now()
	err = Append(path, model.Record{UUID: "u"})
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("err=%v", err)
	}
	if d := time.Since(start); d < 100*time.Millisecond {
		t.Fatalf("returned after %v", d)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("file written despite timeout")
	}
}

func TestAppendMultiProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "brain.jsonl")
	const procs, count = 8, 40
	var cmds []*exec.Cmd
	var outs []*bytes.Buffer
	for id := 0; id < procs; id++ {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s:%d:%d", helperEnv, path, id, count))
		out := &bytes.Buffer{}
		cmd.Stderr = out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
		outs = append(outs, out)
	}
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("worker %d: %v: %s", i, err, outs[i])
		}
	}
	var warn bytes.Buffer
	recs, err := ReadFile(path, &warn)
	if err != nil || warn.Len() != 0 {
		t.Fatalf("err=%v warn=%q", err, warn.String())
	}
	if len(recs) != procs*count*2 {
		t.Fatalf("got %d records, want %d", len(recs), procs*count*2)
	}
	seen := map[string]bool{}
	for _, r := range recs {
		if seen[r.UUID] || r.Object != strings.Repeat("x", 3000) {
			t.Fatalf("duplicate or damaged record %q", r.UUID)
		}
		seen[r.UUID] = true
	}
	// Each call's two lines must be adjacent: one write, no interleaving.
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	for i := 0; i < len(lines); i += 2 {
		if !strings.Contains(lines[i], "-a\"") || !strings.Contains(lines[i+1], "-b\"") ||
			lines[i][9:18] != lines[i+1][9:18] {
			t.Fatalf("lines %d,%d interleaved", i, i+1)
		}
	}
}

func TestParseAt(t *testing.T) {
	for in, want := range map[string]string{
		"2026-10-29":                "2026-10-29T00:00:00Z",
		"2026-10-29T16:00:05Z":      "2026-10-29T16:00:05Z",
		"2026-10-29T18:00:05+02:00": "2026-10-29T16:00:05Z",
	} {
		got, err := ParseAt(in)
		if err != nil || model.FormatTime(got) != want {
			t.Errorf("ParseAt(%q) = %v, %v; want %s", in, got, err, want)
		}
	}
	for _, in := range []string{"", "yesterday", "2026-13-01", "2026-10-29 16:00"} {
		if _, err := ParseAt(in); err == nil {
			t.Errorf("ParseAt(%q): want error", in)
		}
	}
}

func TestAsOfAndLive(t *testing.T) {
	const u1 = "0192d7c4-0001-7000-8000-000000000001"
	const u2 = "0192d7c4-0002-7000-8000-000000000002"
	recs := []model.Record{
		{UUID: u1, Subject: "relay", Relation: "is", Object: "project", Time: "2026-10-29T16:00:00Z"},
		{UUID: u2, Subject: "relay", Relation: "uses", Object: "bun", Time: "2026-10-29T16:00:05Z"},
		{UUID: "0192d7c4-0004-7000-8000-000000000004", Subject: u2, Relation: "forget", Object: "moved", Time: "2026-10-30T09:00:00Z"},
		{UUID: "0192d7c4-0005-7000-8000-000000000005", Subject: "unknown-uuid", Relation: "forget", Object: "", Time: "2026-10-30T09:00:00Z"},
	}
	uuids := func(rs []model.Record) []string {
		var out []string
		for _, r := range rs {
			out = append(out, r.UUID[len(r.UUID)-1:])
		}
		return out
	}
	cases := []struct {
		at   string
		asOf []string
		live []string
	}{
		{"2026-10-29", nil, nil},
		{"2026-10-29T16:00:00Z", []string{"1"}, []string{"1"}},
		{"2026-10-29T16:00:05Z", []string{"1", "2"}, []string{"1", "2"}},
		{"2026-10-30", []string{"1", "2"}, []string{"1", "2"}},
		{"2026-10-30T09:00:00Z", []string{"1", "2", "4", "5"}, []string{"1"}},
	}
	for _, c := range cases {
		at, err := ParseAt(c.at)
		if err != nil {
			t.Fatal(err)
		}
		got := AsOf(recs, at)
		if !reflect.DeepEqual(uuids(got), c.asOf) {
			t.Errorf("AsOf(%s) = %v, want %v", c.at, uuids(got), c.asOf)
		}
		if live := Live(got); !reflect.DeepEqual(uuids(live), c.live) {
			t.Errorf("Live(AsOf(%s)) = %v, want %v", c.at, uuids(live), c.live)
		}
	}
	if got := uuids(Live(recs)); !reflect.DeepEqual(got, []string{"1"}) {
		t.Errorf("Live(all) = %v", got)
	}
}

func TestResolvePath(t *testing.T) {
	t.Setenv(EnvFile, "/env/b.jsonl")
	if p, _ := ResolvePath("/flag/b.jsonl", true); p != "/flag/b.jsonl" {
		t.Errorf("flag: %s", p)
	}
	if p, _ := ResolvePath("", false); p != "/env/b.jsonl" {
		t.Errorf("env: %s", p)
	}
	t.Setenv(EnvFile, "")
	t.Setenv("HOME", "/home/x")
	if p, _ := ResolvePath("", false); p != "/home/x/.myself/brain.jsonl" {
		t.Errorf("default: %s", p)
	}
}
