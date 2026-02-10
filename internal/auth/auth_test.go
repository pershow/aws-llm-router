package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"aws-cursor-router/internal/config"
)

func TestManagerUpsertAndDeleteClient(t *testing.T) {
	manager := NewManager(config.Config{GlobalMaxConcurrent: 16})
	if err := manager.ReplaceClients([]config.ClientConfig{
		{
			ID:                   "team-a",
			Name:                 "Team A",
			APIKey:               "key-a",
			MaxRequestsPerMinute: 100,
			MaxConcurrent:        10,
			AllowedModels:        []string{"gpt-4o"},
		},
	}); err != nil {
		t.Fatalf("replace clients failed: %v", err)
	}

	if err := manager.UpsertClient(config.ClientConfig{
		ID:                   "team-b",
		Name:                 "Team B",
		APIKey:               "key-b",
		MaxRequestsPerMinute: 80,
		MaxConcurrent:        8,
		AllowedModels:        []string{"claude"},
	}); err != nil {
		t.Fatalf("upsert client failed: %v", err)
	}

	clients := manager.ListClients()
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(clients))
	}

	if !manager.DeleteClient("team-a") {
		t.Fatalf("expected delete team-a to succeed")
	}

	clients = manager.ListClients()
	if len(clients) != 1 || clients[0].ID != "team-b" {
		t.Fatalf("unexpected clients after delete: %+v", clients)
	}
}

func TestManagerAuthenticateDisabledClient(t *testing.T) {
	manager := NewManager(config.Config{GlobalMaxConcurrent: 16})
	if err := manager.ReplaceClients([]config.ClientConfig{
		{
			ID:       "team-disabled",
			Name:     "Disabled Team",
			APIKey:   "key-disabled",
			Disabled: true,
		},
	}); err != nil {
		t.Fatalf("replace clients failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer key-disabled")

	if _, err := manager.Authenticate(req); err == nil || err.Error() != "api key is disabled" {
		t.Fatalf("expected disabled api key error, got: %v", err)
	}

	if err := manager.UpsertClient(config.ClientConfig{
		ID:                   "team-disabled",
		Name:                 "Disabled Team",
		APIKey:               "key-disabled",
		MaxRequestsPerMinute: 120,
		MaxConcurrent:        12,
		Disabled:             false,
	}); err != nil {
		t.Fatalf("upsert enabled client failed: %v", err)
	}

	if _, err := manager.Authenticate(req); err != nil {
		t.Fatalf("expected authenticate success after re-enable, got: %v", err)
	}
}
