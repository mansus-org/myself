package model

import (
	"fmt"
	"strings"
)

// RelationForget marks a retraction record (SPEC section 4). It is stored but
// never accepted from a user.
const RelationForget = "forget"

// relations is the closed enum of SPEC section 2, in display order. It is the
// only place the list lives in code.
var relations = []string{"is", "are", "has", "uses", "does", "prefers", "related"}

// Relations returns the accepted relation names in SPEC order. The caller
// gets a copy.
func Relations() []string {
	return append([]string(nil), relations...)
}

// ParseRelation validates a user-supplied relation, case-insensitively, and
// returns the form to store: `are` becomes `is`. The error text has no
// "error: " prefix; the CLI adds it.
func ParseRelation(s string) (string, error) {
	r := strings.ToLower(strings.TrimSpace(s))
	list := strings.Join(relations, ", ")
	if r == RelationForget {
		return "", fmt.Errorf("relation %q is reserved, use the forget command. use one of: %s", RelationForget, list)
	}
	for _, name := range relations {
		if r == name {
			if r == "are" {
				return "is", nil
			}
			return r, nil
		}
	}
	return "", fmt.Errorf("unknown relation %q. use one of: %s", s, list)
}
