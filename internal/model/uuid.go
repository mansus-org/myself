package model

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

var uuidGen struct {
	mu  sync.Mutex
	ms  uint64 // unix milliseconds of the last uuid issued
	seq uint16 // 12-bit counter within ms
}

// NewUUID returns a UUIDv7 (RFC 9562). Within a process the results are
// strictly increasing, even in a tight loop or if the clock steps back, so a
// bulk write keeps its order. The 12 bits after the version hold a counter
// (RFC 9562 method 1); the remaining 62 bits are random.
func NewUUID() string {
	return newUUIDAt(time.Now())
}

func newUUIDAt(now time.Time) string {
	var rnd [10]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		panic("model: crypto/rand failed: " + err.Error())
	}

	g := &uuidGen
	g.mu.Lock()
	ms := uint64(now.UnixMilli())
	switch {
	case ms > g.ms:
		g.ms = ms
		// Start below the top of the range so a burst has room to count up.
		g.seq = (uint16(rnd[8])<<8 | uint16(rnd[9])) & 0x7ff
	default:
		g.seq++
		if g.seq > 0xfff { // counter exhausted: borrow the next millisecond
			g.ms++
			g.seq = 0
		}
	}
	ms, seq := g.ms, g.seq
	g.mu.Unlock()

	var b [16]byte
	b[0], b[1], b[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	b[3], b[4], b[5] = byte(ms>>16), byte(ms>>8), byte(ms)
	b[6] = 0x70 | byte(seq>>8)
	b[7] = byte(seq)
	copy(b[8:], rnd[:8])
	b[8] = 0x80 | b[8]&0x3f // variant 10

	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}

// IsUUID reports whether s is a canonical lowercase UUID string. The forget
// command uses it to tell "forget <uuid>" from "forget <subject> <relation>".
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
