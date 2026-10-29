package model

import "testing"

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "relay", "relay"},
		{"spec example", "Relay App", "relay-app"},
		{"lowercase", "BUN", "bun"},
		{"trim", "  relay  ", "relay"},
		{"trim tabs and newlines", "\t relay\n", "relay"},
		{"collapse spaces", "relay    app", "relay-app"},
		{"collapse mixed whitespace", "a \t\n b", "a-b"},
		{"strip trailing period", "relay.", "relay"},
		{"strip quotes", `"relay app"`, "relay-app"},
		{"strip brackets", "(relay)", "relay"},
		{"strip leading and trailing mix", "...relay!?", "relay"},
		{"strip spaced punctuation", "( relay )", "relay"},
		{"strip surrounding hyphens", "--relay--", "relay"},
		{"keep inner punctuation", "node.js", "node.js"},
		{"keep inner hyphen", "built-with", "built-with"},
		{"inner comma stays", "a, b", "a,-b"},
		{"strip symbol", "$relay$", "relay"},
		{"unicode lowercase", "ÉCOLE", "école"},
		{"unicode punctuation", "«relay»", "relay"},
		{"no stemming", "Relays", "relays"},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"punctuation only", "?!", ""},
		{"i", "i", "self"},
		{"I upper", "I", "self"},
		{"me", "me", "self"},
		{"Myself", "Myself", "self"},
		{"myself punctuated", " Myself. ", "self"},
		{"me inside name", "me-too", "me-too"},
		{"i inside phrase", "i am", "i-am"},
		{"self stays", "self", "self"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeName(tt.in); got != tt.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeNameIdempotent(t *testing.T) {
	for _, in := range []string{"Relay App", " (Me) ", "a  b", "--x--", "Myself"} {
		once := NormalizeName(in)
		if twice := NormalizeName(once); twice != once {
			t.Errorf("not idempotent: %q -> %q -> %q", in, once, twice)
		}
	}
}
