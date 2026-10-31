// Package gitsync holds the git side of `myself sync`: the merge driver that
// unions two append-only logs, and thin helpers around the git binary.
package gitsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mansus-org/myself/internal/model"
)

type line struct {
	text string
	uuid string // empty when the line is not a record
}

// Merge returns the union of the complete lines of every input: blank lines
// and a trailing partial line are dropped, identical lines collapse, and
// records are sorted by uuid (SPEC section 4). Lines that do not parse as a
// record are kept after the records, in order of first appearance.
func Merge(inputs ...[]byte) []byte {
	seen := map[string]bool{}
	var recs, other []line
	for _, in := range inputs {
		in = in[:bytes.LastIndexByte(in, '\n')+1]
		for _, text := range strings.Split(string(in), "\n") {
			text = strings.TrimSuffix(text, "\r")
			if strings.TrimSpace(text) == "" || seen[text] {
				continue
			}
			seen[text] = true
			l := line{text: text}
			var rec model.Record
			if json.Unmarshal([]byte(text), &rec) == nil && model.IsUUID(rec.UUID) {
				l.uuid = rec.UUID
				recs = append(recs, l)
			} else {
				other = append(other, l)
			}
		}
	}
	sort.SliceStable(recs, func(i, j int) bool {
		if recs[i].uuid != recs[j].uuid {
			return recs[i].uuid < recs[j].uuid
		}
		return recs[i].text < recs[j].text
	})
	var out bytes.Buffer
	for _, l := range append(recs, other...) {
		out.WriteString(l.text)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// MergeFiles is the git merge driver: it merges base, ours and theirs and
// writes the result to ours atomically.
func MergeFiles(base, ours, theirs string) error {
	var inputs [][]byte
	for _, p := range []string{base, ours, theirs} {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		inputs = append(inputs, b)
	}
	return writeAtomic(ours, Merge(inputs...))
}

func writeAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".merge-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
