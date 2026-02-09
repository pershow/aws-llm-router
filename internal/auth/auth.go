package auth

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"

	"aws-cursor-router/internal/config"
	"golang.org/x/time/rate"
)

type Client struct {
	ID                   string
	Name                 string
	APIKey               string
	MaxRequestsPerMinute int
	MaxConcurrent        int
	AllowedModels        map[string]struct{}
	limiter              *rate.Limiter
	sem                  chan struct{}
}

type Manager struct {
	mu        sync.RWMutex
	byAPIKey  map[string]*Client
	byID      map[string]*Client
	globalSem chan struct{}
}

func NewManager(cfg config.Config) *Manager {
	manager := &Manager{
		byAPIKey: map[string]*Client{},
		byID:     map[string]*Client{},
	}

	if cfg.GlobalMaxConcurrent > 0 {
		manager.globalSem = make(chan struct{}, cfg.GlobalMaxConcurrent)
	}
	return manager
}

func (m *Manager) Authenticate(r *http.Request) (*Client, error) {
	token := extractToken(r)
	if token == "" {
		return nil, errors.New("missing api key")
	}

	m.mu.RLock()
	client, ok := m.byAPIKey[token]
	m.mu.RUnlock()
	if !ok {
		return nil, errors.New("invalid api key")
	}

	return client, nil
}

func (m *Manager) Acquire(ctx context.Context, client *Client) (func(), error) {
	if client == nil {
		return nil, errors.New("client is nil")
	}

	globalAcquired := false
	clientAcquired := false
	release := func() {
		if clientAcquired {
			<-client.sem
		}
		if globalAcquired {
			<-m.globalSem
		}
	}

	if m.globalSem != nil {
		select {
		case m.globalSem <- struct{}{}:
			globalAcquired = true
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if client.sem != nil {
		select {
		case client.sem <- struct{}{}:
			clientAcquired = true
		case <-ctx.Done():
			release()
			return nil, ctx.Err()
		}
	}

	return release, nil
}

func (c *Client) AllowRequest() bool {
	if c.limiter == nil {
		return true
	}
	return c.limiter.Allow()
}

func (c *Client) IsModelAllowed(requestedModel string, bedrockModelID string) bool {
	if len(c.AllowedModels) == 0 {
		return true
	}

	requestedModel = strings.ToLower(strings.TrimSpace(requestedModel))
	bedrockModelID = strings.ToLower(strings.TrimSpace(bedrockModelID))

	if _, ok := c.AllowedModels[requestedModel]; ok {
		return true
	}
	if _, ok := c.AllowedModels[bedrockModelID]; ok {
		return true
	}
	return false
}

func (m *Manager) ReplaceClients(clientCfgs []config.ClientConfig) error {
	byAPIKey := make(map[string]*Client, len(clientCfgs))
	byID := make(map[string]*Client, len(clientCfgs))

	for _, clientCfg := range clientCfgs {
		client, err := buildClient(clientCfg)
		if err != nil {
			return err
		}
		if _, ok := byID[client.ID]; ok {
			return errors.New("duplicate client id: " + client.ID)
		}
		if _, ok := byAPIKey[client.APIKey]; ok {
			return errors.New("duplicate client api key")
		}
		byID[client.ID] = client
		byAPIKey[client.APIKey] = client
	}

	m.mu.Lock()
	m.byID = byID
	m.byAPIKey = byAPIKey
	m.mu.Unlock()
	return nil
}

func (m *Manager) UpsertClient(clientCfg config.ClientConfig) error {
	client, err := buildClient(clientCfg)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.byAPIKey[client.APIKey]; ok && existing.ID != client.ID {
		return errors.New("api key already in use")
	}
	if existing, ok := m.byID[client.ID]; ok && existing.APIKey != client.APIKey {
		delete(m.byAPIKey, existing.APIKey)
	}

	m.byID[client.ID] = client
	m.byAPIKey[client.APIKey] = client
	return nil
}

func (m *Manager) DeleteClient(clientID string) bool {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	client, ok := m.byID[clientID]
	if !ok {
		return false
	}
	delete(m.byID, clientID)
	delete(m.byAPIKey, client.APIKey)
	return true
}

func (m *Manager) ListClients() []config.ClientConfig {
	m.mu.RLock()
	clients := make([]config.ClientConfig, 0, len(m.byID))
	for _, client := range m.byID {
		allowedModels := make([]string, 0, len(client.AllowedModels))
		for model := range client.AllowedModels {
			allowedModels = append(allowedModels, model)
		}
		sort.Strings(allowedModels)

		clients = append(clients, config.ClientConfig{
			ID:                   client.ID,
			Name:                 client.Name,
			APIKey:               client.APIKey,
			MaxRequestsPerMinute: client.MaxRequestsPerMinute,
			MaxConcurrent:        client.MaxConcurrent,
			AllowedModels:        allowedModels,
		})
	}
	m.mu.RUnlock()

	sort.Slice(clients, func(i, j int) bool {
		return clients[i].ID < clients[j].ID
	})
	return clients
}

func extractToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth != "" {
		const prefix = "Bearer "
		if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
			return strings.TrimSpace(auth[len(prefix):])
		}
	}

	if token := strings.TrimSpace(r.Header.Get("x-api-key")); token != "" {
		return token
	}
	if token := strings.TrimSpace(r.Header.Get("api-key")); token != "" {
		return token
	}
	return ""
}

func buildClient(clientCfg config.ClientConfig) (*Client, error) {
	id := strings.TrimSpace(clientCfg.ID)
	if id == "" {
		return nil, errors.New("client id is required")
	}
	apiKey := strings.TrimSpace(clientCfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("client api key is required")
	}

	maxRPM := clientCfg.MaxRequestsPerMinute
	if maxRPM <= 0 {
		maxRPM = 1200
	}
	maxConcurrent := clientCfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 64
	}

	client := &Client{
		ID:                   id,
		Name:                 defaultIfEmpty(strings.TrimSpace(clientCfg.Name), id),
		APIKey:               apiKey,
		MaxRequestsPerMinute: maxRPM,
		MaxConcurrent:        maxConcurrent,
		AllowedModels:        map[string]struct{}{},
	}

	for _, model := range clientCfg.AllowedModels {
		model = strings.ToLower(strings.TrimSpace(model))
		if model == "" {
			continue
		}
		client.AllowedModels[model] = struct{}{}
	}

	limit := rate.Limit(float64(maxRPM) / 60.0)
	burst := max(1, min(maxRPM, maxRPM/5))
	client.limiter = rate.NewLimiter(limit, burst)
	client.sem = make(chan struct{}, maxConcurrent)
	return client, nil
}

func defaultIfEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
