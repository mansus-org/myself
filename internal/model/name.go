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
	if r, ok := normalizeASCII(s); ok {
		return r
	}
	return normalizeSlow(s)
}

func normalizeSlow(s string) string {
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

// asciiPunct marks the ASCII bytes that isPunct accepts.
var asciiPunct = func() (t [128]bool) {
	for c := range t {
		t[c] = isPunct(rune(c))
	}
	return t
}()

// normalizeASCII is NormalizeName for pure-ASCII input. It returns ok=false
// for anything else. It allocates only when it has to change the string.
func normalizeASCII(s string) (string, bool) {
	clean := true // no uppercase, no whitespace other than a lone inner run
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			return "", false
		}
		if 'A' <= c && c <= 'Z' || asciiSpace(c) {
			clean = false
		}
	}
	if !clean {
		buf := make([]byte, 0, len(s))
		pendingSep := false
		for i := 0; i < len(s); i++ {
			c := s[i]
			switch {
			case asciiSpace(c):
				pendingSep = len(buf) > 0
			default:
				if pendingSep {
					buf = append(buf, '-')
					pendingSep = false
				}
				if 'A' <= c && c <= 'Z' {
					c += 'a' - 'A'
				}
				buf = append(buf, c)
			}
		}
		s = string(buf)
	}
	for len(s) > 0 && asciiPunct[s[0]] {
		s = s[1:]
	}
	for len(s) > 0 && asciiPunct[s[len(s)-1]] {
		s = s[:len(s)-1]
	}
	switch s {
	case "i", "me", "myself":
		return SelfName, true
	}
	return s, true
}

func asciiSpace(c byte) bool {
	return c == ' ' || '\t' <= c && c <= '\r'
}
