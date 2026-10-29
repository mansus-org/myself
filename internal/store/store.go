// Package store reads and appends the JSONL event log (SPEC sections 1, 4, 7).
package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Record is one line of the log. Field order is the on-disk key order.
type Record struct {
	UUID     string `json:"uuid"`
	Subject  string `json:"subject"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
	Time     string `json:"time"`
}

// Read parses a JSONL stream. A final line without a trailing newline is
// ignored silently (a crash mid-write). Any other line that is not a valid
// record is skipped and reported in warnings as "line N: corrupt, skipped".
func Read(r io.Reader) (records []Record, warnings []string, err error) {
	br := bufio.NewReader(r)
	for n := 1; ; n++ {
		line, rerr := br.ReadBytes('\n')
		if rerr != nil {
			if rerr == io.EOF {
				return records, warnings, nil // partial or empty tail
			}
			return records, warnings, rerr
		}
		line = bytes.TrimSuffix(line, []byte("\n"))
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec Record
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if derr := dec.Decode(&rec); derr != nil || dec.More() || rec.UUID == "" {
			warnings = append(warnings, fmt.Sprintf("line %d: corrupt, skipped", n))
			continue
		}
		records = append(records, rec)
	}
}

// ReadFile reads the log at path. A missing file reads as empty.
func ReadFile(path string) ([]Record, []string, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	return Read(f)
}

// Append writes recs to the log at path as whole lines in a single write,
// creating the file and its directory if needed, then fsyncs.
func Append(path string, recs ...Record) error {
	if len(recs) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, rec := range recs {
		b, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
