package model

import (
	"sort"
	"testing"
	"time"
)

func TestNewUUIDFormat(t *testing.T) {
	u := NewUUID()
	if !IsUUID(u) {
		t.Fatalf("not canonical: %q", u)
	}
	if u[14] != '7' {
		t.Errorf("version nibble = %c, want 7 in %q", u[14], u)
	}
	if !contains("89ab", u[19]) {
		t.Errorf("variant nibble = %c, want 8-b in %q", u[19], u)
	}
}

func contains(set string, c byte) bool {
	for i := 0; i < len(set); i++ {
		if set[i] == c {
			return true
		}
	}
	return false
}

func TestNewUUIDMonotonicTightLoop(t *testing.T) {
	const n = 100000 // far more than 4096 per ms, exercises counter overflow
	ids := make([]string, n)
	for i := range ids {
		ids[i] = NewUUID()
	}
	for i := 1; i < n; i++ {
		if ids[i-1] >= ids[i] {
			t.Fatalf("not strictly increasing at %d: %s then %s", i, ids[i-1], ids[i])
		}
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatal("generation order differs from sorted order")
	}
}

func TestNewUUIDClockStepsBack(t *testing.T) {
	now := time.Now()
	a := newUUIDAt(now)
	b := newUUIDAt(now.Add(-time.Hour))
	if a >= b {
		t.Errorf("clock regression broke order: %s then %s", a, b)
	}
}

func TestNewUUIDEmbedsTime(t *testing.T) {
	at := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	u := newUUIDAt(at)
	// 48-bit ms timestamp is the first 12 hex digits.
	want := at.UnixMilli()
	var got int64
	for _, c := range u[:8] + u[9:13] {
		got = got<<4 | int64(hexVal(byte(c)))
	}
	if got != want {
		t.Errorf("timestamp = %d, want %d (%s)", got, want, u)
	}
}

func hexVal(c byte) byte {
	if c >= 'a' {
		return c - 'a' + 10
	}
	return c - '0'
}

func TestIsUUID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"0192d7c4-0002-7000-8000-000000000002", true},
		{"0192D7C4-0002-7000-8000-000000000002", false},
		{"0192d7c4000270008000000000000002", false},
		{"0192d7c4-0002-7000-8000-00000000000g", false},
		{"relay", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsUUID(tt.in); got != tt.want {
			t.Errorf("IsUUID(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
