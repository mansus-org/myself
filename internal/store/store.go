// Package store reads and appends the JSONL event log (SPEC sections 4 and 7).
// It is the only package that touches brain.jsonl.
package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mansus-org/myself/internal/model"
)

// EnvFile is the environment variable that overrides the default path.
const EnvFile = "MYSELF_FILE"

// ErrLockTimeout is returned by Append when the write lock is not acquired
// within the lock timeout. The CLI maps it to exit code 3.
var ErrLockTimeout = errors.New("timed out waiting for the write lock")

// lockTimeout is a variable so tests can shorten it.
var lockTimeout = 5 * time.Second

const lockPoll = 5 * time.Millisecond

// ResolvePath applies the precedence --file > MYSELF_FILE > ~/.myself/brain.jsonl.
// flagSet reports whether --file was given on the command line.
func ResolvePath(flagValue string, flagSet bool) (string, error) {
	if flagSet {
		return flagValue, nil
	}
	if v := os.Getenv(EnvFile); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".myself", "brain.jsonl"), nil
}

// ReadFile reads the log at path without locking. A missing file reads as empty.
func ReadFile(path string, warn io.Writer) ([]model.Record, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f, warn)
}

// Append writes recs to the log at path under an exclusive flock on
// path+".lock", as whole lines in a single write, then fsyncs. The file and
// its directory are created if needed. It returns ErrLockTimeout if the lock
// is not acquired within 5 s.
func Append(path string, recs ...model.Record) error {
	if len(recs) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, rec := range recs {
		b, err := model.Marshal(rec)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	unlock, err := lock(path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()

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

func lock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(lockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { f.Close() }, nil // closing releases the lock
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EINTR {
			f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, ErrLockTimeout
		}
		time.Sleep(lockPoll)
	}
}

// ParseAt parses an --at value: RFC 3339, or YYYY-MM-DD meaning 00:00:00Z
// of that day (SPEC section 4).
func ParseAt(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("bad time %q: use RFC 3339 or YYYY-MM-DD", s)
}

// AsOf returns the records (already uuid-sorted) with time <= at, forget
// records included. Records with an unparseable time are dropped.
func AsOf(recs []model.Record, at time.Time) []model.Record {
	var out []model.Record
	for _, r := range recs {
		t, err := time.Parse(time.RFC3339, r.Time)
		if err == nil && !t.After(at) {
			out = append(out, r)
		}
	}
	return out
}

// Live returns the records that are not forgotten, in input order. Forget
// records themselves are not live. Forgetting an unknown or already
// forgotten uuid is a no-op.
func Live(recs []model.Record) []model.Record {
	forgotten := make(map[string]bool)
	for _, r := range recs {
		if r.IsForget() {
			forgotten[r.Subject] = true
		}
	}
	var out []model.Record
	for _, r := range recs {
		if !r.IsForget() && !forgotten[r.UUID] {
			out = append(out, r)
		}
	}
	return out
}
