package jira

import (
	"context"
	"encoding/json"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"time"
)

// per specs/026-reliable-evidence-foundation.md:150
var _ = Describe("reliable evidence Jira pagination", func() {
	// per specs/026-reliable-evidence-foundation.md:61
	It("retains provider IDs across pages and changed issue keys", func() {
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			Expect(r.URL.Path).To(Equal("/rest/api/3/search/jql"))
			if r.URL.Query().Get("nextPageToken") == "" {
				_, _ = w.Write([]byte(`{"issues":[{"id":"123","key":"PROJ-1","fields":{}}],"nextPageToken":"second","isLast":false}`))
			} else {
				Expect(r.URL.Query().Get("nextPageToken")).To(Equal("second"))
				_, _ = w.Write([]byte(`{"issues":[{"id":"123","key":"NEXT-1","fields":{}}],"isLast":true}`))
			}
		}))
		defer provider.Close()
		client := New(provider.URL, "reader@example.test", "fixture", "team")
		client.EpicLinkField = "epic"
		issues, err := client.SearchDetailed(context.Background(), "project=PROJ", "", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(issues).To(HaveLen(2))
		Expect(issues[0].ID).To(Equal("123"))
		Expect(issues[1].ID).To(Equal("123"))
		Expect(issues[0].Key).To(Equal("PROJ-1"))
		Expect(issues[1].Key).To(Equal("NEXT-1"))
	})

	// per specs/026-reliable-evidence-foundation.md:156
	DescribeTable("rejects incomplete or cycling pagination without returning partial evidence", func(tokens []string) {
		calls := 0
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			defer GinkgoRecover()
			if calls >= len(tokens) {
				http.Error(w, "fixture exhausted", http.StatusBadGateway)
				return
			}
			token := tokens[calls]
			calls++
			Expect(json.NewEncoder(w).Encode(map[string]any{"issues": []map[string]any{{"id": fmt.Sprint(calls), "key": fmt.Sprintf("PROJ-%d", calls), "fields": map[string]any{}}}, "nextPageToken": token, "isLast": false})).To(Succeed())
		}))
		defer provider.Close()
		client := New(provider.URL, "reader@example.test", "fixture", "team")
		client.EpicLinkField = "epic"
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		issues, err := client.SearchDetailed(ctx, "project=PROJ", "", nil)
		Expect(err).To(HaveOccurred())
		Expect(issues).To(BeEmpty())
		Expect(calls).To(Equal(len(tokens)), "failure occurs on the malformed page, before another provider call")
	}, Entry("repeated token", []string{"a", "a"}), Entry("two-token cycle", []string{"a", "b", "a"}), Entry("unfinished page without continuation", []string{""}))
})

// classic/company-managed Jira projects associate a story with its epic via
// the legacy "Epic Link" custom field rather than the native "parent" field
// (that's only populated for sub-tasks). Without this fallback, ParentKey is
// always empty for such projects, so FeverEpics never finds a child in any
// epic and the fever chart renders empty.
var _ = Describe("ParseDetailedFallsBackToEpicLinkField", func() {
	It("behaves", func() {
		c := &Client{PodField: "customfield_10026", EpicLinkField: "customfield_10008"}
		raw := []byte(`{"key":"PROJ-42","fields":{"customfield_10008":"PROJ-1"}}`)
		d := c.parseDetailed(raw, "customfield_10014")
		if d.ParentKey != "PROJ-1" {
			Fail(fmt.Sprintf("ParentKey = %q, want PROJ-1 (from Epic Link field)", d.ParentKey))
		}
	})
})

var _ = Describe("ParseDetailedPrefersParentOverEpicLinkField", func() {
	It("behaves", func() {
		c := &Client{PodField: "customfield_10026", EpicLinkField: "customfield_10008"}
		raw := []byte(`{"key":"PROJ-42","fields":{"parent":{"key":"PROJ-2"},"customfield_10008":"PROJ-1"}}`)
		d := c.parseDetailed(raw, "customfield_10014")
		if d.ParentKey != "PROJ-2" {
			Fail(fmt.Sprintf("ParentKey = %q, want PROJ-2 (native parent wins)", d.ParentKey))
		}
	})
})

var _ = Describe("ParseDetailedNoEpicLinkFieldConfigured", func() {
	It("behaves", func() {
		c := &Client{PodField: "customfield_10026"} // EpicLinkField unresolved/absent
		raw := []byte(`{"key":"PROJ-42","fields":{"customfield_10008":"PROJ-1"}}`)
		d := c.parseDetailed(raw, "customfield_10014")
		if d.ParentKey != "" {
			Fail(fmt.Sprintf("ParentKey = %q, want empty when no Epic Link field is configured", d.ParentKey))
		}
	})
})
