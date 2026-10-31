package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mansus-org/myself/internal/model"
)

// Read parses a JSONL stream and returns the records sorted by uuid. A final
// line without a trailing newline is ignored silently (a crash mid-write).
// Any other line that is not a valid record is skipped and reported on warn
// as "warning: line N: corrupt, skipped". warn may be nil.
//
// The stream is read whole and split into lines, which are decoded in
// parallel, each into its own slot, so records and warnings come out
// deterministically in file order.
func Read(r io.Reader, warn io.Writer) ([]model.Record, error) {
	if warn == nil {
		warn = io.Discard
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	// Everything after the last newline is a partial tail and is dropped.
	end := bytes.LastIndexByte(raw, '\n') + 1
	data := string(raw[:end])

	// Line i+1 is data[starts[i] : starts[i+1]-1].
	starts := make([]int, 1, bytes.Count(raw[:end], []byte{'\n'})+1)
	for off := 0; off < len(data); {
		off += strings.IndexByte(data[off:], '\n') + 1
		starts = append(starts, off)
	}
	lines := len(starts) - 1

	recs := make([]model.Record, lines)
	state := make([]uint8, lines)
	decode := func(lo, hi int) {
		for i := lo; i < hi; i++ {
			rec, st := decodeLine(data[starts[i] : starts[i+1]-1])
			recs[i], state[i] = rec, st
		}
	}
	workers := runtime.GOMAXPROCS(0)
	if lines < 2048 || workers < 2 {
		decode(0, lines)
	} else {
		var wg sync.WaitGroup
		chunk := (lines + workers - 1) / workers
		for lo := 0; lo < lines; lo += chunk {
			wg.Add(1)
			go func(lo, hi int) {
				defer wg.Done()
				decode(lo, hi)
			}(lo, min(lo+chunk, lines))
		}
		wg.Wait()
	}

	out := recs[:0]
	for i, st := range state {
		switch st {
		case stValid:
			out = append(out, recs[i])
		case stCorrupt:
			fmt.Fprintf(warn, "warning: line %d: corrupt, skipped\n", i+1)
		}
	}
	cmp := func(a, b model.Record) int { return strings.Compare(a.UUID, b.UUID) }
	if !slices.IsSortedFunc(out, cmp) {
		slices.SortStableFunc(out, cmp)
	}
	return out, nil
}

// Line states: blank lines are skipped silently, corrupt ones are reported.
const (
	stBlank = iota
	stValid
	stCorrupt
)

func decodeLine(line string) (model.Record, uint8) {
	if rec, ok := fastDecode(line); ok {
		return rec, stValid
	}
	if len(strings.TrimSpace(line)) == 0 {
		return model.Record{}, stBlank
	}
	var rec model.Record
	dec := json.NewDecoder(strings.NewReader(line))
	dec.DisallowUnknownFields()
	if dec.Decode(&rec) != nil || dec.More() || rec.UUID == "" {
		return model.Record{}, stCorrupt
	}
	return rec, stValid
}

var fastKeys = [5]string{`{"uuid":"`, `","subject":"`, `","relation":"`, `","object":"`, `","time":"`}

// fastDecode handles the canonical form Append writes: the five keys in
// order, no whitespace, string values with no escapes or control bytes, and
// valid UTF-8. Anything else returns ok=false and is decided by
// encoding/json, so the accepted lines are exactly the same.
func fastDecode(line string) (model.Record, bool) {
	const tail = `"}`
	if !strings.HasPrefix(line, fastKeys[0]) || !strings.HasSuffix(line, tail) {
		return model.Record{}, false
	}
	var vals [5]string
	rest := line[len(fastKeys[0]) : len(line)-len(tail)]
	for i := range vals {
		v := rest
		if i < 4 {
			j := strings.Index(rest, fastKeys[i+1])
			if j < 0 {
				return model.Record{}, false
			}
			v, rest = rest[:j], rest[j+len(fastKeys[i+1]):]
		}
		for k := 0; k < len(v); k++ {
			if c := v[k]; c < 0x20 || c == '"' || c == '\\' {
				return model.Record{}, false
			}
		}
		vals[i] = v
	}
	if vals[0] == "" || !utf8.ValidString(line) {
		return model.Record{}, false
	}
	return model.Record{UUID: vals[0], Subject: vals[1], Relation: vals[2], Object: vals[3], Time: vals[4]}, true
}
