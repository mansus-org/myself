package model

import (
	"reflect"
	"testing"
)

func TestRelationsList(t *testing.T) {
	want := []string{"is", "are", "has", "uses", "does", "prefers", "related"}
	got := Relations()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Relations() = %v, want %v", got, want)
	}
	got[0] = "mutated"
	if Relations()[0] != "is" {
		t.Error("Relations() exposes internal slice")
	}
}

func TestParseRelation(t *testing.T) {
	const list = "is, are, has, uses, does, prefers, related"
	tests := []struct {
		in      string
		want    string
		wantErr string
	}{
		{"is", "is", ""},
		{"are", "is", ""},
		{"has", "has", ""},
		{"uses", "uses", ""},
		{"does", "does", ""},
		{"prefers", "prefers", ""},
		{"related", "related", ""},
		{"USES", "uses", ""},
		{"Are", "is", ""},
		{"Prefers", "prefers", ""},
		{"built-with", "", `unknown relation "built-with". use one of: ` + list},
		{"Built-With", "", `unknown relation "Built-With". use one of: ` + list},
		{"", "", `unknown relation "". use one of: ` + list},
		{"forget", "", `relation "forget" is reserved, use the forget command. use one of: ` + list},
		{"FORGET", "", `relation "forget" is reserved, use the forget command. use one of: ` + list},
	}
	for _, tt := range tests {
		got, err := ParseRelation(tt.in)
		if tt.wantErr == "" {
			if err != nil || got != tt.want {
				t.Errorf("ParseRelation(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
			}
			continue
		}
		if err == nil || err.Error() != tt.wantErr {
			t.Errorf("ParseRelation(%q) error = %v; want %q", tt.in, err, tt.wantErr)
		}
	}
}
