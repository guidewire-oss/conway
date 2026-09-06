// Package sheets reads bounded, read-only Google Sheets captures for planning.
package sheets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"
)

const MaxBytes = 8 << 20
const MaxRows = 10000
const MaxColumns = 256

// Provider is an internal seam; request bodies cannot select a provider host.
type Provider interface {
	Fetch(context.Context, string, string) ([][]string, error)
}

var sheetPath = regexp.MustCompile(`^/spreadsheets/d/([A-Za-z0-9_-]+)(?:/(?:edit|view|preview|copy))?/?$`)

// ParseLink accepts only canonical Google Sheets links (specs/023-linked-google-sheets.md:208).
func ParseLink(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != "docs.google.com" || u.User != nil {
		return "", errors.New("use an https://docs.google.com/spreadsheets/d/ID link")
	}
	m := sheetPath.FindStringSubmatch(u.Path)
	if len(m) != 2 || len(m[1]) > 200 {
		return "", errors.New("the link must identify a Google spreadsheet")
	}
	return m[1], nil
}

type GoogleProvider struct {
	client *http.Client
	email  string
}

func (p *GoogleProvider) AccountEmail() string { return p.email }

// NewGoogleProvider loads credentials only from the server-configured file.
// Google's token and Sheets endpoints are fixed; redirects are never followed.
func NewGoogleProvider(path string) (*GoogleProvider, error) {
	f, err := os.Open(path) // #nosec G304 -- path comes only from CONWAY_GOOGLE_CREDENTIALS_FILE server configuration, never an HTTP request.
	if err != nil {
		return nil, errors.New("could not open the configured Sheets service account file")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil || len(b) >= 1<<20 {
		return nil, errors.New("could not read the configured Sheets service account file")
	}
	var credentials struct {
		Type         string `json:"type"`
		Email        string `json:"client_email"`
		PrivateKey   string `json:"private_key"`
		PrivateKeyID string `json:"private_key_id"`
	}
	if err := json.Unmarshal(b, &credentials); err != nil || credentials.Type != "service_account" || credentials.Email == "" || credentials.PrivateKey == "" {
		return nil, errors.New("the configured Sheets file must contain service account credentials")
	}
	cfg := jwt.Config{Email: credentials.Email, PrivateKey: []byte(credentials.PrivateKey), PrivateKeyID: credentials.PrivateKeyID, Scopes: []string{"https://www.googleapis.com/auth/spreadsheets.readonly"}, TokenURL: "https://oauth2.googleapis.com/token"} //nolint:gosec // G101: fixed public OAuth endpoint; private key comes only from server configuration.
	base := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, base)
	client := cfg.Client(ctx)
	client.Timeout = 25 * time.Second
	client.CheckRedirect = base.CheckRedirect
	return &GoogleProvider{client: client, email: cfg.Email}, nil
}

func (p *GoogleProvider) Fetch(ctx context.Context, spreadsheetID, rangeName string) ([][]string, error) {
	endpoint := "https://sheets.googleapis.com/v4/spreadsheets/" + url.PathEscape(spreadsheetID) + "/values/" + url.PathEscape(rangeName)
	endpoint += "?majorDimension=ROWS&valueRenderOption=UNFORMATTED_VALUE&dateTimeRenderOption=SERIAL_NUMBER"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("could not prepare the Sheets request")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errors.New("could not reach Google Sheets; check configuration and retry")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusForbidden, http.StatusNotFound:
			return nil, errors.New("access to Google Sheets was refused; share this spreadsheet with the configured service account and check its link and range")
		case http.StatusTooManyRequests:
			return nil, errors.New("the Google Sheets rate limit was reached; retry at the next check")
		default:
			return nil, fmt.Errorf("the Google Sheets request returned HTTP %d; check the range and server configuration", resp.StatusCode)
		}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil || len(b) > MaxBytes {
		return nil, errors.New("the Google Sheets response exceeds the 8 MB capture limit or could not be read")
	}
	var data struct {
		Values [][]any `json:"values"`
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return nil, errors.New("the Google Sheets response contains unreadable cells")
	}
	if len(data.Values) > MaxRows {
		return nil, fmt.Errorf("the range exceeds the %d-row capture limit", MaxRows)
	}
	rows := make([][]string, len(data.Values))
	for i, row := range data.Values {
		if len(row) > MaxColumns {
			return nil, fmt.Errorf("the range exceeds the %d-column capture limit", MaxColumns)
		}
		rows[i] = make([]string, len(row))
		for j, cell := range row {
			switch v := cell.(type) {
			case string:
				rows[i][j] = v
			case json.Number:
				rows[i][j] = string(v)
			case bool:
				rows[i][j] = fmt.Sprint(v)
			case nil:
			default:
				return nil, errors.New("the Google Sheets response contains an unsupported cell value")
			}
		}
	}
	return rows, nil
}
