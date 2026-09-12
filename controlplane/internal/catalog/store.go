package catalog

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu        sync.Mutex
	dataDir   string
	providers []Provider
	agents    []Agent
}

type providersFile struct {
	Providers []Provider `json:"providers"`
}

type agentsFile struct {
	Agents []Agent `json:"agents"`
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}

	s := &Store{dataDir: dataDir}

	providers, err := loadProviders(dataDir)
	if err != nil {
		return nil, err
	}
	s.providers = providers

	agents, err := loadAgents(dataDir)
	if err != nil {
		return nil, err
	}
	s.agents = agents

	return s, nil
}

func loadProviders(dataDir string) ([]Provider, error) {
	path := filepath.Join(dataDir, "providers.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := atomicWrite(path, []byte(`{"providers":[]}`)); err != nil {
			return nil, err
		}
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file providersFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if file.Providers == nil {
		return []Provider{}, nil
	}
	return file.Providers, nil
}

func loadAgents(dataDir string) ([]Agent, error) {
	path := filepath.Join(dataDir, "agents.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := atomicWrite(path, []byte(`{"agents":[]}`)); err != nil {
			return nil, err
		}
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file agentsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if file.Agents == nil {
		return []Agent{}, nil
	}
	return file.Agents, nil
}

func (s *Store) ListProviders() []Provider {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Provider, len(s.providers))
	copy(out, s.providers)
	return out
}

func (s *Store) GetProvider(id string) (Provider, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

func (s *Store) CreateProvider(name, typ, baseURL, apiKey string) (Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(name) == "" {
		return Provider{}, fmt.Errorf("provider name is required")
	}
	if strings.TrimSpace(baseURL) == "" {
		return Provider{}, fmt.Errorf("provider baseURL is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return Provider{}, fmt.Errorf("provider apiKey is required")
	}
	if !isKnownProviderType(typ) {
		return Provider{}, fmt.Errorf("unknown provider type %q", typ)
	}

	id, err := newID("prov_")
	if err != nil {
		return Provider{}, err
	}

	now := time.Now().UTC()
	p := Provider{
		ID:        id,
		Name:      name,
		Type:      typ,
		BaseURL:   strings.TrimRight(baseURL, "/"),
		APIKey:    apiKey,
		Models:    []ModelInfo{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.providers = append(s.providers, p)
	if err := s.saveProvidersLocked(); err != nil {
		s.providers = s.providers[:len(s.providers)-1]
		return Provider{}, err
	}
	return p, nil
}

func (s *Store) UpdateProvider(id string, name, baseURL, apiKey *string) (Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := providerIndex(s.providers, id)
	if idx < 0 {
		return Provider{}, fmt.Errorf("provider %q not found", id)
	}

	p := s.providers[idx]
	if name != nil {
		p.Name = *name
	}
	if baseURL != nil {
		p.BaseURL = strings.TrimRight(*baseURL, "/")
	}
	if apiKey != nil {
		p.APIKey = *apiKey
	}
	p.UpdatedAt = time.Now().UTC()
	s.providers[idx] = p

	if err := s.saveProvidersLocked(); err != nil {
		return Provider{}, err
	}
	return p, nil
}

func (s *Store) DeleteProvider(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := providerIndex(s.providers, id)
	if idx < 0 {
		return fmt.Errorf("provider %q not found", id)
	}

	s.providers = append(s.providers[:idx], s.providers[idx+1:]...)
	return s.saveProvidersLocked()
}

func (s *Store) ReplaceProviderModels(id string, models []ModelInfo, updatedAt time.Time) (Provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := providerIndex(s.providers, id)
	if idx < 0 {
		return Provider{}, fmt.Errorf("provider %q not found", id)
	}

	p := s.providers[idx]
	p.Models = append([]ModelInfo(nil), models...)
	t := updatedAt.UTC()
	p.ModelsUpdatedAt = &t
	p.UpdatedAt = time.Now().UTC()
	s.providers[idx] = p

	if err := s.saveProvidersLocked(); err != nil {
		return Provider{}, err
	}
	return p, nil
}

func (s *Store) saveProvidersLocked() error {
	path := filepath.Join(s.dataDir, "providers.json")
	data, err := json.Marshal(providersFile{Providers: s.providers})
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

func providerIndex(providers []Provider, id string) int {
	for i, p := range providers {
		if p.ID == id {
			return i
		}
	}
	return -1
}

func isKnownProviderType(typ string) bool {
	return typ == TypeOpenAICompatible
}

func newID(prefix string) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	success = true
	return nil
}
