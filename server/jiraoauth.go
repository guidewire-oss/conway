package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"conway/auth"
	"conway/jira"
)

// jiraOAuthConfig holds the registered OAuth app credentials + the public URL
// used to build the redirect_uri (must match the app's registered callback).
type jiraOAuthConfig struct {
	ClientID     string
	ClientSecret string
	PublicURL    string // e.g. https://conway.example.com
}

// jiraSession is a user's connected Jira OAuth session, held in memory only.
type jiraSession struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	CloudID      string
	SiteURL      string
}

func (s *server) jiraConfigured() bool {
	return s.jiraOAuth != nil && s.jiraOAuth.ClientID != "" && s.jiraOAuth.ClientSecret != ""
}

func (s *server) redirectURI() string {
	return strings.TrimRight(s.jiraOAuth.PublicURL, "/") + "/api/jira/oauth/callback"
}

// --- state signing: the callback carries no auth header, so the OAuth `state`
// binds the flow to the conway user via an HMAC over the store secret. ---

func (s *server) signState(sub string) string {
	exp := time.Now().Add(10 * time.Minute).Unix()
	payload := sub + "|" + strconv.FormatInt(exp, 10)
	mac := hmac.New(sha256.New, s.store.Secret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

func (s *server) verifyState(state string) (string, bool) {
	dot := strings.LastIndex(state, ".")
	if dot < 0 {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(state[:dot])
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, s.store.Secret)
	mac.Write(raw)
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(state[dot+1:])) {
		return "", false
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return "", false
	}
	exp, _ := strconv.ParseInt(parts[1], 10, 64)
	if time.Now().Unix() > exp {
		return "", false
	}
	return parts[0], true
}

// handleJiraStatus reports whether OAuth is configured and whether the caller
// has a live connection (so the UI can show Connect vs. the project picker).
func (s *server) handleJiraStatus(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	out := map[string]any{"configured": s.jiraConfigured(), "connected": false}
	if sess := s.jiraSession(c.Sub); sess != nil {
		out["connected"] = true
		out["site"] = sess.SiteURL
	}
	writeJSON(w, out)
}

// handleJiraOAuthStart returns the Atlassian authorize URL (which delegates to
// the org IdP/Okta). The browser navigates there to grant consent.
func (s *server) handleJiraOAuthStart(w http.ResponseWriter, r *http.Request, c auth.Claims) {
	if !s.jiraConfigured() {
		http.Error(w, "Jira OAuth is not configured on the server", 503)
		return
	}
	url := jira.AuthorizeURL(s.jiraOAuth.ClientID, s.redirectURI(), s.signState(c.Sub))
	writeJSON(w, map[string]string{"url": url})
}

// handleJiraOAuthCallback is the browser redirect target (no bearer header —
// the user is identified by the signed state). It exchanges the code, resolves
// the cloud id, stores the session, and bounces back into the app.
func (s *server) handleJiraOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if !s.jiraConfigured() {
		http.Error(w, "Jira OAuth is not configured", 503)
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		http.Redirect(w, r, "/?jira=denied", http.StatusFound)
		return
	}
	code := r.URL.Query().Get("code")
	sub, ok := s.verifyState(r.URL.Query().Get("state"))
	if code == "" || !ok {
		http.Error(w, "invalid OAuth callback", 400)
		return
	}
	ctx := r.Context()
	tok, err := jira.ExchangeCode(ctx, s.jiraOAuth.ClientID, s.jiraOAuth.ClientSecret, code, s.redirectURI())
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), 502)
		return
	}
	resources, err := jira.AccessibleResources(ctx, tok.AccessToken)
	if err != nil || len(resources) == 0 {
		http.Error(w, "no accessible Jira sites for this account", 502)
		return
	}
	res := pickJiraSite(resources, s.jiraSiteHint)
	s.jiraMu.Lock()
	s.jiraSessions[sub] = &jiraSession{
		AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken,
		Expiry:  time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second),
		CloudID: res.ID, SiteURL: res.URL,
	}
	s.jiraMu.Unlock()
	http.Redirect(w, r, "/?import=1", http.StatusFound)
}

// pickJiraSite prefers a site whose URL contains hint (e.g. your org's Jira
// hostname, via CONWAY_JIRA_SITE_HINT), else the first accessible resource.
func pickJiraSite(rs []jira.Resource, hint string) jira.Resource {
	if hint != "" {
		for _, r := range rs {
			if strings.Contains(r.URL, hint) {
				return r
			}
		}
	}
	return rs[0]
}

func (s *server) jiraSession(sub string) *jiraSession {
	s.jiraMu.Lock()
	defer s.jiraMu.Unlock()
	return s.jiraSessions[sub]
}

// jiraClientFor returns a Jira client for the caller: an OAuth client when the
// user has a live session (refreshing if needed), else a basic-auth client from
// the supplied credentials. Errors if neither is available.
func (s *server) jiraClientFor(sub string, cr jiraCreds) (*jira.Client, error) {
	if sess := s.jiraSession(sub); sess != nil {
		token, cloudID, err := s.jiraAccessToken(sub)
		if err != nil {
			return nil, err
		}
		return jira.NewOAuth(cloudID, token, cr.PodField), nil
	}
	if cr.valid() {
		return jira.New(cr.BaseURL, cr.Email, cr.Token, cr.PodField), nil
	}
	if s.jiraConfigured() {
		return nil, fmt.Errorf("connect Jira first")
	}
	return nil, fmt.Errorf("enter Jira base URL, email and API token")
}

// jiraAccessToken returns a valid access token for the user, refreshing it when
// it is within a minute of expiry.
func (s *server) jiraAccessToken(sub string) (token, cloudID string, err error) {
	s.jiraMu.Lock()
	sess := s.jiraSessions[sub]
	s.jiraMu.Unlock()
	if sess == nil {
		return "", "", fmt.Errorf("connect Jira first")
	}
	if time.Now().Before(sess.Expiry.Add(-time.Minute)) {
		return sess.AccessToken, sess.CloudID, nil
	}
	if sess.RefreshToken == "" {
		return "", "", fmt.Errorf("Jira session expired — reconnect")
	}
	t, err := jira.Refresh(context.Background(), s.jiraOAuth.ClientID, s.jiraOAuth.ClientSecret, sess.RefreshToken)
	if err != nil {
		return "", "", fmt.Errorf("Jira session expired — reconnect")
	}
	s.jiraMu.Lock()
	sess.AccessToken = t.AccessToken
	if t.RefreshToken != "" {
		sess.RefreshToken = t.RefreshToken
	}
	sess.Expiry = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	tok, cid := sess.AccessToken, sess.CloudID
	s.jiraMu.Unlock()
	return tok, cid, nil
}
