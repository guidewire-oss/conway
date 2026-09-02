package jira

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
)

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
