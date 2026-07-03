package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to Jira Cloud REST v3. It supports two auth modes: basic auth
// (email + API token) and OAuth 2.0 bearer (via api.atlassian.com/ex/jira/
// {cloudId}). Credentials are held only for the lifetime of a request set.
type Client struct {
	BaseURL       string // site URL (basic) or https://api.atlassian.com/ex/jira/{cloudId} (oauth)
	authz         string // the full Authorization header value
	PodField      string // custom field id holding the pod, default customfield_10026
	EpicLinkField string // custom field id for the legacy "Epic Link" relationship (classic
	// company-managed projects; team-managed projects use "parent" instead and
	// leave this empty). Resolved once via ResolveEpicLinkField before import.
	HTTP *http.Client
}

func defaultPodField(podField string) string {
	if podField == "" {
		return "customfield_10026"
	}
	return podField
}

// New builds a basic-auth client (email + API token).
func New(baseURL, email, token, podField string) *Client {
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		authz:    "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token)),
		PodField: defaultPodField(podField),
		HTTP:     &http.Client{Timeout: 30 * time.Second},
	}
}

// NewOAuth builds an OAuth bearer client targeting a specific Jira cloud id.
func NewOAuth(cloudID, accessToken, podField string) *Client {
	return &Client{
		BaseURL:  "https://api.atlassian.com/ex/jira/" + cloudID,
		authz:    "Bearer " + accessToken,
		PodField: defaultPodField(podField),
		HTTP:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.authz)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("jira auth failed (%d) — check email/token", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		return fmt.Errorf("jira %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Project is the minimal project descriptor for the picker.
type Project struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ListProjects returns the projects the credentials can see (paginated).
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var out []Project
	startAt := 0
	for {
		var page struct {
			Values []Project `json:"values"`
			IsLast bool      `json:"isLast"`
			Total  int       `json:"total"`
		}
		path := fmt.Sprintf("/rest/api/3/project/search?startAt=%d&maxResults=50&orderBy=key", startAt)
		if err := c.get(ctx, path, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Values...)
		if page.IsLast || len(page.Values) == 0 {
			break
		}
		startAt += len(page.Values)
		if startAt > 5000 { // safety bound
			break
		}
	}
	return out, nil
}

// resolveEpicLinkField looks up the site's "Epic Link" custom field id (present
// on classic/company-managed projects; absent on team-managed ones, where
// "parent" already covers epic association). No-op once resolved; a lookup
// failure is swallowed since the field is optional — imports still work off
// "parent" alone.
func (c *Client) resolveEpicLinkField(ctx context.Context) {
	if c.EpicLinkField != "" {
		return
	}
	var fields []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.get(ctx, "/rest/api/3/field", &fields); err != nil {
		return
	}
	for _, f := range fields {
		if f.Name == "Epic Link" {
			c.EpicLinkField = f.ID
			return
		}
	}
}

// raw shapes for issue parsing
type rawIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Created        string                `json:"created"`
		ResolutionDate string                `json:"resolutiondate"`
		IssueType      struct{ Name string } `json:"issuetype"`
		IssueLinks     []struct {
			Type struct {
				Inward  string `json:"inward"`
				Outward string `json:"outward"`
			} `json:"type"`
			InwardIssue  *struct{ Key string } `json:"inwardIssue"`
			OutwardIssue *struct{ Key string } `json:"outwardIssue"`
		} `json:"issuelinks"`
	} `json:"fields"`
}

const jiraTime = "2006-01-02T15:04:05.000-0700"

func parseJiraTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(jiraTime, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// SearchIssues runs a JQL query and returns issues mapped for aggregation. The
// pod is read from the configured custom field (an object with a "value").
func (c *Client) SearchIssues(ctx context.Context, jql string) ([]Issue, error) {
	fields := "created,resolutiondate,issuetype,issuelinks," + c.PodField
	var out []Issue
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
			it, ok := c.parseIssue(raw)
			if ok {
				out = append(out, it)
			}
		}
		if page.NextPageToken == "" || page.IsLast {
			break
		}
		nextToken = page.NextPageToken
		if len(out) > 100000 { // safety bound
			break
		}
	}
	return out, nil
}

func (c *Client) parseIssue(raw json.RawMessage) (Issue, bool) {
	var ri rawIssue
	if err := json.Unmarshal(raw, &ri); err != nil {
		return Issue{}, false
	}
	// pull the pod custom field by its configured id
	var envelope struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	json.Unmarshal(raw, &envelope)
	pod := ""
	if pf := envelope.Fields[c.PodField]; len(pf) > 0 {
		var obj struct {
			Value string `json:"value"`
		}
		if json.Unmarshal(pf, &obj) == nil {
			pod = obj.Value
		}
	}
	it := Issue{Key: ri.Key, Pod: pod, IssueType: ri.Fields.IssueType.Name}
	if t, ok := parseJiraTime(ri.Fields.Created); ok {
		it.Created = t
	}
	if t, ok := parseJiraTime(ri.Fields.ResolutionDate); ok {
		it.Resolved = &t
	}
	for _, l := range ri.Fields.IssueLinks {
		if l.OutwardIssue != nil && strings.Contains(strings.ToLower(l.Type.Outward), "block") {
			it.Blocks = append(it.Blocks, l.OutwardIssue.Key)
		}
		if l.InwardIssue != nil && strings.Contains(strings.ToLower(l.Type.Inward), "block") {
			it.BlockedBy = append(it.BlockedBy, l.InwardIssue.Key)
		}
	}
	return it, true
}
