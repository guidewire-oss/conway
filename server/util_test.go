package main

import (
	"math"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// readAll must reject an oversized body rather than truncate it. The bug this
// guards against is subtle: io.LimitReader(r, max) returns a short read that is
// indistinguishable from a complete file, so an over-limit spreadsheet used to
// be parsed as though it were whole.
var _ = Describe("readAll", func() {
	const limit = 16

	DescribeTable("an over-limit body is rejected, not silently truncated",
		func(size int, wantErr bool) {
			got, err := readAll(strings.NewReader(strings.Repeat("x", size)), limit)
			if wantErr {
				Expect(err).To(HaveOccurred(),
					"%d bytes with a %d-byte limit must error, not truncate", size, limit)
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(len(got)).To(Equal(size), "all bytes must come back")
			}
		},
		Entry("under the limit", limit-1, false),
		Entry("exactly at the limit", limit, false),
		Entry("one byte over", limit+1, true),
		Entry("far over", limit*100, true),
	)

	// The +1 that makes an oversized body observable must not wrap. At
	// MaxInt64 there is no larger limit to read, and a negative limit would
	// make io.LimitReader hand back an empty body with no error — a body
	// silently lost rather than rejected.
	It("survives extreme limits", func() {
		body := strings.Repeat("x", 32)
		got, err := readAll(strings.NewReader(body), math.MaxInt64)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(got)).To(Equal(len(body)))
		_, err = readAll(strings.NewReader(body), -1)
		Expect(err).To(HaveOccurred(), "a negative limit must error, not silently lose the body")
	})
})
