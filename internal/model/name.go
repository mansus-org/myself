package model

import (
	"strings"
	"unicode"
)

// SelfName is the entity that `i`, `me` and `myself` resolve to.
const SelfName = "self"

// NormalizeName applies SPEC section 3: trim, lowercase, collapse whitespace
// runs to `-`, strip surrounding punctuation. A whole name of `i`, `me` or
// `myself` resolves to `self`. It is used for subjects at write time and for
// objects when matching against entity names. The result is empty if nothing
// but whitespace and punctuation was given.
func NormalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.FieldsFunc(s, unicode.IsSpace), "-")
	s = strings.TrimFunc(s, isPunct)
	switch s {
	case "i", "me", "myself":
		return SelfName
	}
	return s
}

// isPunct covers Unicode punctuation and symbols, and so also `-`.
func isPunct(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}
