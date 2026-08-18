package main

import (
	"strings"
	"testing"
)

// readAll must reject an oversized body rather than truncate it. The bug this
// guards against is subtle: io.LimitReader(r, max) returns a short read that is
// indistinguishable from a complete file, so an over-limit spreadsheet used to
// be parsed as though it were whole.
func TestReadAllRejectsOversizedBody(t *testing.T) {
	const limit = 16

	for _, tc := range []struct {
		name    string
		size    int
		wantErr bool
	}{
		{"under the limit", limit - 1, false},
		{"exactly at the limit", limit, false},
		{"one byte over", limit + 1, true},
		{"far over", limit * 100, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Repeat("x", tc.size)
			got, err := readAll(strings.NewReader(body), limit)
			switch {
			case tc.wantErr && err == nil:
				t.Fatalf("%d bytes with a %d-byte limit: want an error, got %d bytes back (silent truncation)", tc.size, limit, len(got))
			case !tc.wantErr && err != nil:
				t.Fatalf("%d bytes with a %d-byte limit: unexpected error: %v", tc.size, limit, err)
			case !tc.wantErr && len(got) != tc.size:
				t.Fatalf("%d bytes with a %d-byte limit: got %d bytes back, want all of them", tc.size, limit, len(got))
			}
		})
	}
}
