package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth 2.0 (3LO) for Atlassian Cloud. Login is delegated to the org's IdP
// (Okta) in the browser; the server only exchanges the resulting code for
// tokens and calls api.atlassian.com on the user's behalf.

const (
	authBase     = "https://auth.atlassian.com"
	oauthScopes  = "read:jira-work read:jira-user offline_access"
	resourcesURL = "https://api.atlassian.com/oauth/token/accessible-resources"
)

// AuthorizeURL is where the browser is sent to start the consent flow.
func AuthorizeURL(clientID, redirectURI, state string) string {
	v := url.Values{}
	v.Set("audience", "api.atlassian.com")
	v.Set("client_id", clientID)
	v.Set("scope", oauthScopes)
	v.Set("redirect_uri", redirectURI)
	v.Set("state", state)
	v.Set("response_type", "code")
	v.Set("prompt", "consent")
	return authBase + "/authorize?" + v.Encode()
}

// Token is the OAuth token response.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

func postToken(ctx context.Context, body map[string]string) (*Token, error) {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authBase+"/oauth/token", strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		return nil, fmt.Errorf("oauth token %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var t Token
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ExchangeCode swaps an authorization code for tokens.
func ExchangeCode(ctx context.Context, clientID, secret, code, redirectURI string) (*Token, error) {
	return postToken(ctx, map[string]string{
		"grant_type": "authorization_code", "client_id": clientID, "client_secret": secret,
		"code": code, "redirect_uri": redirectURI,
	})
}

// Refresh exchanges a refresh token for a fresh access token (rotating refresh).
func Refresh(ctx context.Context, clientID, secret, refreshToken string) (*Token, error) {
	return postToken(ctx, map[string]string{
		"grant_type": "refresh_token", "client_id": clientID, "client_secret": secret,
		"refresh_token": refreshToken,
	})
}

// Resource is one Jira site the token can access (id is the cloud id).
type Resource struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
}

// AccessibleResources lists the Jira sites granted to the access token.
func AccessibleResources(ctx context.Context, accessToken string) ([]Resource, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resourcesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		return nil, fmt.Errorf("accessible-resources %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out []Resource
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
