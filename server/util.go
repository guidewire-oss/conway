package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
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

func readAll(r io.Reader, max int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, max))
}
