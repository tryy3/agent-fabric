package modelspecs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const maxLogoBytes = 1 << 20

// errLogoNotFound means the source answered 404: a definitive miss, unlike a
// network error or a cancelled request, which must not be cached.
var errLogoNotFound = errors.New("logo not found")

var logoIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type logo struct {
	body        []byte
	contentType string
}

// logoBase is the directory holding logos: the source URL without its last
// path segment (https://models.dev/api.json -> https://models.dev/).
func logoBase(source string) (string, error) {
	u, err := url.Parse(source)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid source url")
	}
	i := strings.LastIndex(u.Path, "/")
	u.Path = u.Path[:i+1]
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

// Logo returns a provider logo, fetched once from the specs source and cached
// until the next sync. ok is false when the source has no logo for the provider.
func (s *Service) Logo(ctx context.Context, id string) (body []byte, contentType string, ok bool) {
	if !logoIDPattern.MatchString(id) {
		return nil, "", false
	}
	s.mu.RLock()
	source := settingsOf(s.row).EffectiveSourceURL()
	_, known := s.providers[id]
	cached, hit := s.logos[id]
	s.mu.RUnlock()
	if !known {
		return nil, "", false
	}
	if hit {
		return cached.body, cached.contentType, cached.body != nil
	}
	base, err := logoBase(source)
	if err != nil {
		return nil, "", false
	}
	definitive := true
	for _, ext := range []string{"svg", "png"} {
		l, ferr := s.fetchLogo(ctx, base+"logos/"+id+"."+ext, ext)
		if ferr == nil {
			body, contentType, ok = l.body, l.contentType, true
			break
		}
		if !errors.Is(ferr, errLogoNotFound) {
			definitive = false
		}
	}
	if ok || definitive {
		s.mu.Lock()
		s.logos[id] = logo{body: body, contentType: contentType} // nil body caches a miss
		s.mu.Unlock()
	}
	return body, contentType, ok
}

func (s *Service) fetchLogo(ctx context.Context, u, ext string) (logo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return logo{}, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return logo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return logo{}, errLogoNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return logo{}, fmt.Errorf("logo HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxLogoBytes+1))
	if err != nil || len(b) > maxLogoBytes || len(b) == 0 {
		return logo{}, fmt.Errorf("bad logo body")
	}
	ct := "image/png"
	if ext == "svg" {
		ct = "image/svg+xml"
	}
	return logo{body: b, contentType: ct}, nil
}
