package export

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

const (
	netlifyAPIBase       = "https://api.netlify.com/api/v1"
	netlifyPollInterval  = 2 * time.Second
	netlifyPollTimeout   = 2 * time.Minute
	netlifyRemoteKind    = "netlify"
	netlifyRemoteID      = "rmt_netlify"
)

// Netlify publishes the workspace as a static site via the Netlify Deploy API.
type Netlify struct {
	Token      string
	AccountID  string
	HTTPClient *http.Client
	APIBase    string
	MaxBytes   int
	PollEvery  time.Duration
	PollLimit  time.Duration
	// Now is optional; tests inject a fixed clock.
	Now func() time.Time
}

func (n Netlify) Method() Method {
	return Method{
		ID:      MethodNetlify,
		Label:   "Netlify",
		Enabled: true,
	}
}

func (n Netlify) Export(ctx context.Context, req Request) (Result, error) {
	token := strings.TrimSpace(n.Token)
	if token == "" {
		token = netlifyAPIKey(req.Integrations)
	}
	if token == "" {
		return Result{}, ErrMissingCredentials
	}
	accountID := strings.TrimSpace(n.AccountID)
	if accountID == "" {
		accountID = netlifyAccountID(req.Integrations)
	}
	client := n.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	base := strings.TrimRight(n.APIBase, "/")
	if base == "" {
		base = netlifyAPIBase
	}
	pollEvery := n.PollEvery
	if pollEvery <= 0 {
		pollEvery = netlifyPollInterval
	}
	pollLimit := n.PollLimit
	if pollLimit <= 0 {
		pollLimit = netlifyPollTimeout
	}

	siteID, siteURL, remotesPatch, err := n.ensureSite(ctx, client, base, token, accountID, req.Project)
	if err != nil {
		return Result{}, err
	}

	zipBody, err := buildWorkspaceZip(ctx, req.FS, n.MaxBytes)
	if err != nil {
		return Result{}, err
	}

	deploy, err := n.createDeploy(ctx, client, base, token, siteID, zipBody)
	if err != nil {
		return Result{}, err
	}
	deploy, err = n.waitDeploy(ctx, client, base, token, deploy.ID, pollEvery, pollLimit)
	if err != nil {
		return Result{}, err
	}

	if siteURL == "" {
		siteURL = firstNonEmpty(deploy.SSLURL, deploy.URL)
	}
	deployURL := firstNonEmpty(deploy.DeploySSLURL, deploy.SSLURL, deploy.URL)
	links := make([]Link, 0, 2)
	if siteURL != "" {
		links = append(links, Link{ID: "site", Label: "Site", URL: siteURL})
	}
	if deployURL != "" {
		links = append(links, Link{ID: "deploy", Label: "This deploy", URL: deployURL})
	}
	if len(links) == 0 {
		return Result{}, fmt.Errorf("%w: deploy succeeded but returned no URLs", ErrPublishFailed)
	}
	if siteURL != "" && remotesPatch == nil {
		// Refresh stored site URL when Netlify reports a newer canonical URL.
		remotesPatch = netlifyRemotesPatch(siteID, siteURL)
	}
	return Result{
		MediaType:    "application/json",
		Links:        links,
		Message:      "Published to Netlify",
		RemotesPatch: remotesPatch,
	}, nil
}

func (n Netlify) ensureSite(
	ctx context.Context,
	client *http.Client,
	base, token, accountID string,
	project catalog.Project,
) (siteID, siteURL string, remotesPatch json.RawMessage, err error) {
	if existing := findNetlifyRemote(project.Remotes); existing != nil {
		siteID = strings.TrimSpace(existing.Path)
		siteURL = strings.TrimSpace(existing.URLOrBucket)
		if siteID != "" {
			return siteID, siteURL, nil, nil
		}
	}
	name := netlifySiteName(project.Name)
	created, createErr := n.createSite(ctx, client, base, token, accountID, name)
	if createErr != nil {
		return "", "", nil, createErr
	}
	siteID = created.ID
	siteURL = firstNonEmpty(created.SSLURL, created.URL, "https://"+created.DefaultDomain)
	return siteID, siteURL, netlifyRemotesPatch(siteID, siteURL), nil
}

type netlifySite struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	SSLURL        string `json:"ssl_url"`
	DefaultDomain string `json:"default_domain"`
}

type netlifyDeploy struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	URL          string `json:"url"`
	SSLURL       string `json:"ssl_url"`
	DeploySSLURL string `json:"deploy_ssl_url"`
	ErrorMessage string `json:"error_message"`
}

func (n Netlify) createSite(
	ctx context.Context,
	client *http.Client,
	base, token, accountID, name string,
) (netlifySite, error) {
	path := "/sites"
	if accountID != "" {
		path = "/" + urlPathEscape(accountID) + "/sites"
	}
	body, _ := json.Marshal(map[string]string{"name": name})
	var site netlifySite
	if err := n.doJSON(ctx, client, http.MethodPost, base+path, token, "application/json", bytes.NewReader(body), &site); err != nil {
		return netlifySite{}, fmt.Errorf("%w: create site: %v", ErrPublishFailed, err)
	}
	if site.ID == "" {
		return netlifySite{}, fmt.Errorf("%w: create site returned no id", ErrPublishFailed)
	}
	return site, nil
}

func (n Netlify) createDeploy(
	ctx context.Context,
	client *http.Client,
	base, token, siteID string,
	zipBody []byte,
) (netlifyDeploy, error) {
	var deploy netlifyDeploy
	url := base + "/sites/" + urlPathEscape(siteID) + "/deploys"
	if err := n.doJSON(ctx, client, http.MethodPost, url, token, "application/zip", bytes.NewReader(zipBody), &deploy); err != nil {
		return netlifyDeploy{}, fmt.Errorf("%w: create deploy: %v", ErrPublishFailed, err)
	}
	if deploy.ID == "" {
		return netlifyDeploy{}, fmt.Errorf("%w: create deploy returned no id", ErrPublishFailed)
	}
	return deploy, nil
}

func (n Netlify) waitDeploy(
	ctx context.Context,
	client *http.Client,
	base, token, deployID string,
	every, limit time.Duration,
) (netlifyDeploy, error) {
	deadline := n.now().Add(limit)
	var last netlifyDeploy
	for {
		if err := ctx.Err(); err != nil {
			return netlifyDeploy{}, err
		}
		url := base + "/deploys/" + urlPathEscape(deployID)
		if err := n.doJSON(ctx, client, http.MethodGet, url, token, "", nil, &last); err != nil {
			return netlifyDeploy{}, fmt.Errorf("%w: poll deploy: %v", ErrPublishFailed, err)
		}
		switch strings.ToLower(strings.TrimSpace(last.State)) {
		case "ready":
			return last, nil
		case "error", "failed":
			msg := strings.TrimSpace(last.ErrorMessage)
			if msg == "" {
				msg = "deploy state " + last.State
			}
			return netlifyDeploy{}, fmt.Errorf("%w: %s", ErrPublishFailed, msg)
		}
		if !n.now().Before(deadline) {
			return netlifyDeploy{}, fmt.Errorf("%w: deploy timed out (last state %q)", ErrPublishFailed, last.State)
		}
		timer := time.NewTimer(every)
		select {
		case <-ctx.Done():
			timer.Stop()
			return netlifyDeploy{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (n Netlify) now() time.Time {
	if n.Now != nil {
		return n.Now()
	}
	return time.Now()
}

func (n Netlify) doJSON(
	ctx context.Context,
	client *http.Client,
	method, url, token, contentType string,
	body io.Reader,
	out any,
) error {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("netlify %s: %s", resp.Status, truncate(msg, 400))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode netlify response: %w", err)
	}
	return nil
}

type netlifyIntegrations struct {
	Netlify *struct {
		APIKey    string `json:"apiKey"`
		AccountID string `json:"accountId"`
	} `json:"netlify"`
}

func netlifyAPIKey(raw json.RawMessage) string {
	var bag netlifyIntegrations
	if err := json.Unmarshal(rawOrEmptyObject(raw), &bag); err != nil || bag.Netlify == nil {
		return ""
	}
	return strings.TrimSpace(bag.Netlify.APIKey)
}

func netlifyAccountID(raw json.RawMessage) string {
	var bag netlifyIntegrations
	if err := json.Unmarshal(rawOrEmptyObject(raw), &bag); err != nil || bag.Netlify == nil {
		return ""
	}
	return strings.TrimSpace(bag.Netlify.AccountID)
}

func rawOrEmptyObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func findNetlifyRemote(raw json.RawMessage) *catalog.Remote {
	if len(raw) == 0 {
		return nil
	}
	var remotes []catalog.Remote
	if err := json.Unmarshal(raw, &remotes); err != nil {
		return nil
	}
	for i := range remotes {
		if strings.EqualFold(strings.TrimSpace(remotes[i].Kind), netlifyRemoteKind) {
			return &remotes[i]
		}
	}
	return nil
}

func netlifyRemotesPatch(siteID, siteURL string) json.RawMessage {
	enabled := true
	raw, err := json.Marshal([]catalog.Remote{{
		ID:          netlifyRemoteID,
		Kind:        netlifyRemoteKind,
		Path:        siteID,
		URLOrBucket: siteURL,
		Enabled:     &enabled,
	}})
	if err != nil {
		return nil
	}
	return raw
}

func netlifySiteName(projectName string) string {
	slug := slugify(projectName)
	if slug == "" {
		slug = "project"
	}
	if len(slug) > 40 {
		slug = slug[:40]
		slug = strings.Trim(slug, "-")
	}
	return slug + "-" + randomHex(3)
}

func slugify(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '_' || r == '-' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func randomHex(nBytes int) string {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte{0xab, 0xcd, 0xef})[:nBytes*2]
	}
	return hex.EncodeToString(buf)
}

func urlPathEscape(s string) string {
	return strings.ReplaceAll(s, "/", "%2F")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
