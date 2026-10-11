package export

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

const (
	netlifyDeploysPerPage = 100
	// netlifyDeployPageLimit bounds one sync; a longer history is reported as
	// incomplete so older stored deploys are kept.
	netlifyDeployPageLimit = 10
)

var _ PublishManager = Netlify{}

// ListDeploys reports the bound Netlify site's successful production deploys
// and which one the site currently serves.
func (n Netlify) ListDeploys(ctx context.Context, req Request) ([]TargetDeploys, error) {
	siteID := boundNetlifySite(req.Project)
	if siteID == "" {
		return nil, nil
	}
	token, client, base, err := n.conn(req)
	if err != nil {
		return nil, err
	}
	site, err := n.getSite(ctx, client, base, token, siteID)
	if err != nil {
		return nil, err
	}
	liveID := ""
	if site.PublishedDeploy != nil {
		liveID = site.PublishedDeploy.ID
	}
	siteURL := firstNonEmpty(site.SSLURL, site.URL)

	out := TargetDeploys{Target: siteID, Complete: false}
	seenLive := false
	for page := 1; page <= netlifyDeployPageLimit; page++ {
		var batch []netlifyDeploy
		url := fmt.Sprintf("%s/sites/%s/deploys?page=%d&per_page=%d",
			base, urlPathEscape(siteID), page, netlifyDeploysPerPage)
		if err := n.doJSON(ctx, client, http.MethodGet, url, token, "", nil, &batch); err != nil {
			return nil, fmt.Errorf("%w: list deploys: %v", ErrPublishFailed, err)
		}
		for _, d := range batch {
			if in, ok := netlifyDeployInput(d, siteURL, liveID); ok {
				seenLive = seenLive || in.Live
				out.Deploys = append(out.Deploys, in)
			}
		}
		if len(batch) < netlifyDeploysPerPage {
			out.Complete = true
			break
		}
	}
	// A rollback can make an old deploy live; keep it in view even when the
	// list was cut off before reaching it.
	if liveID != "" && !seenLive {
		var d netlifyDeploy
		url := base + "/deploys/" + urlPathEscape(liveID)
		if err := n.doJSON(ctx, client, http.MethodGet, url, token, "", nil, &d); err != nil {
			return nil, fmt.Errorf("%w: get live deploy: %v", ErrPublishFailed, err)
		}
		if in, ok := netlifyDeployInput(d, siteURL, liveID); ok {
			out.Deploys = append(out.Deploys, in)
		}
	}
	return []TargetDeploys{out}, nil
}

// DeleteDeploy deletes one deploy of the bound site. Netlify keeps serving
// the site from its published deploy, so that one is refused.
func (n Netlify) DeleteDeploy(ctx context.Context, req Request, target, deployID string) error {
	target = strings.TrimSpace(target)
	deployID = strings.TrimSpace(deployID)
	if target == "" || deployID == "" {
		return fmt.Errorf("%w: publish has no Netlify site or deploy id", ErrPublishNotManaged)
	}
	token, client, base, err := n.conn(req)
	if err != nil {
		return err
	}
	site, err := n.getSite(ctx, client, base, token, target)
	if err != nil {
		var status *netlifyStatusError
		if errors.As(err, &status) && status.Status == http.StatusNotFound {
			// The whole site is gone, so is the deploy.
			return nil
		}
		return err
	}
	if site.PublishedDeploy != nil && site.PublishedDeploy.ID == deployID {
		return ErrPublishLive
	}
	url := base + "/sites/" + urlPathEscape(target) + "/deploys/" + urlPathEscape(deployID)
	if err := n.doJSON(ctx, client, http.MethodDelete, url, token, "", nil, nil); err != nil {
		var status *netlifyStatusError
		if errors.As(err, &status) && status.Status == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("%w: delete deploy: %v", ErrPublishFailed, err)
	}
	return nil
}

func (n Netlify) getSite(ctx context.Context, client *http.Client, base, token, siteID string) (netlifySite, error) {
	var site netlifySite
	url := base + "/sites/" + urlPathEscape(siteID)
	if err := n.doJSON(ctx, client, http.MethodGet, url, token, "", nil, &site); err != nil {
		return netlifySite{}, fmt.Errorf("%w: get site: %w", ErrPublishFailed, err)
	}
	return site, nil
}

// netlifyDeployInput maps a ready production deploy to a publish entry.
func netlifyDeployInput(d netlifyDeploy, siteURL, liveID string) (catalog.PublishInput, bool) {
	if d.ID == "" || !strings.EqualFold(strings.TrimSpace(d.State), "ready") {
		return catalog.PublishInput{}, false
	}
	// Branch deploys and deploy previews are not publishes of the project.
	if ctx := strings.TrimSpace(d.Context); ctx != "" && ctx != "production" {
		return catalog.PublishInput{}, false
	}
	in := catalog.PublishInput{
		DeployID:  d.ID,
		SiteURL:   firstNonEmpty(siteURL, d.SSLURL, d.URL),
		DeployURL: firstNonEmpty(d.DeploySSLURL, d.DeployURL),
		Live:      d.ID == liveID,
		CreatedAt: d.CreatedAt,
	}
	if in.SiteURL == "" && in.DeployURL == "" {
		return catalog.PublishInput{}, false
	}
	return in, true
}

func boundNetlifySite(project catalog.Project) string {
	if remote := findNetlifyRemote(project.Remotes); remote != nil {
		return strings.TrimSpace(remote.Path)
	}
	return ""
}
