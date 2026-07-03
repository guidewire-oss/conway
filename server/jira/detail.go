package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// DetailedIssue carries the richer fields the enrichment docs need (hygiene,
// epic_meta, epics/*) beyond what plain aggregation uses.
type DetailedIssue struct {
	Key        string
	Summary    string
	Pod        string
	IssueType  string
	StatusName string
	StatusCat  string // statusCategory key: new | indeterminate | done
	Assignee   string
	Points     *float64
	Labels     []string
	ParentKey  string
	DescLen    int
	Created    time.Time
	Updated    time.Time
	Resolved   *time.Time
	DueDate    string // YYYY-MM-DD (Jira date-only)
	Blocks     []string
	BlockedBy  []string
}

// Basic projects a detailed issue down to the aggregation shape.
func (d DetailedIssue) Basic() Issue {
	return Issue{Key: d.Key, Pod: d.Pod, IssueType: d.IssueType, StatusCat: d.StatusCat, ParentKey: d.ParentKey,
		Created: d.Created, Resolved: d.Resolved, Blocks: d.Blocks, BlockedBy: d.BlockedBy}
}

// IsOpen reports whether the issue is not in the done status category.
func (d DetailedIssue) IsOpen() bool { return d.StatusCat != "done" }

// InProgress reports whether the issue is actively in flight (WIP).
func (d DetailedIssue) InProgress() bool { return d.StatusCat == "indeterminate" }

const outcomeMinDescLen = 40 // a description this long counts as a stated outcome

// SearchDetailed runs a JQL query returning the rich field set. pointsField
// defaults to customfield_10014 (story points in this instance). onProgress (if
// non-nil) is called after each page with the running issue count.
func (c *Client) SearchDetailed(ctx context.Context, jql, pointsField string, onProgress func(int)) ([]DetailedIssue, error) {
	if pointsField == "" {
		pointsField = "customfield_10014"
	}
	c.resolveEpicLinkField(ctx)
	fieldList := []string{
		"summary", "created", "updated", "resolutiondate", "duedate", "issuetype",
		"status", "assignee", "labels", "parent", "description", "issuelinks",
		c.PodField, pointsField,
	}
	if c.EpicLinkField != "" {
		fieldList = append(fieldList, c.EpicLinkField)
	}
	fields := strings.Join(fieldList, ",")
	var out []DetailedIssue
	nextToken := ""
	for {
		u := fmt.Sprintf("/rest/api/3/search/jql?jql=%s&fields=%s&maxResults=100",
			url.QueryEscape(jql), url.QueryEscape(fields))
		if nextToken != "" {
			u += "&nextPageToken=" + url.QueryEscape(nextToken)
		}
		var page struct {
			Issues        []json.RawMessage `json:"issues"`
			NextPageToken string            `json:"nextPageToken"`
			IsLast        bool              `json:"isLast"`
		}
		if err := c.get(ctx, u, &page); err != nil {
			return nil, err
		}
		for _, raw := range page.Issues {
			out = append(out, c.parseDetailed(raw, pointsField))
		}
		if onProgress != nil {
			onProgress(len(out))
		}
		if page.NextPageToken == "" || page.IsLast {
			break
		}
		nextToken = page.NextPageToken
		if len(out) > 100000 {
			break
		}
	}
	return out, nil
}

func (c *Client) parseDetailed(raw json.RawMessage, pointsField string) DetailedIssue {
	var e struct {
		Key    string                     `json:"key"`
		Fields map[string]json.RawMessage `json:"fields"`
	}
	json.Unmarshal(raw, &e)
	f := e.Fields
	d := DetailedIssue{Key: e.Key}

	str := func(b json.RawMessage) string {
		var s string
		json.Unmarshal(b, &s)
		return s
	}
	d.Summary = str(f["summary"])
	d.DueDate = str(f["duedate"])
	if t, ok := parseJiraTime(str(f["created"])); ok {
		d.Created = t
	}
	if t, ok := parseJiraTime(str(f["updated"])); ok {
		d.Updated = t
	}
	if t, ok := parseJiraTime(str(f["resolutiondate"])); ok {
		d.Resolved = &t
	}
	var it struct{ Name string }
	json.Unmarshal(f["issuetype"], &it)
	d.IssueType = it.Name
	var st struct {
		Name           string `json:"name"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	}
	json.Unmarshal(f["status"], &st)
	d.StatusName, d.StatusCat = st.Name, st.StatusCategory.Key
	if a := f["assignee"]; len(a) > 0 && string(a) != "null" {
		var as struct {
			DisplayName string `json:"displayName"`
		}
		json.Unmarshal(a, &as)
		d.Assignee = as.DisplayName
	}
	json.Unmarshal(f["labels"], &d.Labels)
	if p := f["parent"]; len(p) > 0 && string(p) != "null" {
		var par struct {
			Key string `json:"key"`
		}
		json.Unmarshal(p, &par)
		d.ParentKey = par.Key
	}
	if d.ParentKey == "" && c.EpicLinkField != "" {
		// classic/company-managed projects link stories to their epic via the
		// legacy "Epic Link" field (a plain issue-key string), not "parent".
		if el := f[c.EpicLinkField]; len(el) > 0 && string(el) != "null" {
			d.ParentKey = str(el)
		}
	}
	if pf := f[c.PodField]; len(pf) > 0 && string(pf) != "null" {
		var obj struct {
			Value string `json:"value"`
		}
		json.Unmarshal(pf, &obj)
		d.Pod = obj.Value
	}
	if pv := f[pointsField]; len(pv) > 0 && string(pv) != "null" {
		var n float64
		if json.Unmarshal(pv, &n) == nil {
			d.Points = &n
		}
	}
	d.DescLen = adfTextLen(f["description"])
	// issuelinks (blocks/blockedBy) — reuse the same shape as the basic parser
	var lf struct {
		IssueLinks []struct {
			Type struct {
				Inward  string `json:"inward"`
				Outward string `json:"outward"`
			} `json:"type"`
			InwardIssue  *struct{ Key string } `json:"inwardIssue"`
			OutwardIssue *struct{ Key string } `json:"outwardIssue"`
		} `json:"issuelinks"`
	}
	if il := f["issuelinks"]; len(il) > 0 {
		json.Unmarshal([]byte(`{"issuelinks":`+string(il)+`}`), &lf)
	}
	for _, l := range lf.IssueLinks {
		if l.OutwardIssue != nil && strings.Contains(strings.ToLower(l.Type.Outward), "block") {
			d.Blocks = append(d.Blocks, l.OutwardIssue.Key)
		}
		if l.InwardIssue != nil && strings.Contains(strings.ToLower(l.Type.Inward), "block") {
			d.BlockedBy = append(d.BlockedBy, l.InwardIssue.Key)
		}
	}
	return d
}

// PodDevCounts derives the pods seen in the issues (by the pod field) and an
// estimated dev-count per pod = the number of distinct assignees who worked its
// issues. Used to build org structure when no Plan roster is supplied.
func PodDevCounts(issues []DetailedIssue) map[string]int {
	people := map[string]map[string]bool{}
	for _, it := range issues {
		p := podOf(it.Pod)
		if p == "" {
			continue
		}
		if people[p] == nil {
			people[p] = map[string]bool{}
		}
		if it.Assignee != "" {
			people[p][it.Assignee] = true
		}
	}
	out := map[string]int{}
	for p, set := range people {
		out[p] = len(set)
	}
	return out
}

// adfTextLen flattens an Atlassian Document Format description to its text
// length (Jira REST v3 returns rich descriptions as ADF JSON, not plain text).
func adfTextLen(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var node any
	if json.Unmarshal(raw, &node) != nil {
		return 0
	}
	var n int
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if t["type"] == "text" {
				if s, ok := t["text"].(string); ok {
					n += len(strings.TrimSpace(s))
				}
			}
			for _, child := range t {
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(node)
	return n
}
