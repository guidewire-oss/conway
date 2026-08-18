package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"math"
)

func randRead(b []byte) (int, error) { return rand.Read(b) }

func randPw() string {
	b := make([]byte, 9)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// newID returns a short, URL-safe random identifier (for plans, etc.).
func newID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// newJoinCode returns a short, human-typable game join code (no ambiguous chars).
func newJoinCode() string {
	const al = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // omits 0/O/1/I
	b := make([]byte, 6)
	rand.Read(b)
	for i := range b {
		b[i] = al[int(b[i])%len(al)]
	}
	return string(b)
}

// readAll reads at most max bytes, rejecting anything longer rather than
// truncating it. Reading exactly max via io.LimitReader is the trap: an
// oversized body comes back as a short read that is indistinguishable from a
// complete file, so a 30MB spreadsheet would be parsed as a 20MB one. Reading
// one byte past the limit is what makes the overflow observable.
func readAll(r io.Reader, max int64) ([]byte, error) {
	if max < 0 {
		return nil, fmt.Errorf("readAll: negative limit %d", max)
	}
	// max+1 is the whole trick, so it must not wrap. At MaxInt64 there is no
	// larger limit to read, and a negative max would make LimitReader see a
	// non-positive limit and hand back an empty body with no error at all.
	limit := max + 1
	if max == math.MaxInt64 {
		limit = max
	}
	b, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("body exceeds the %d-byte limit", max)
	}
	return b, nil
}
