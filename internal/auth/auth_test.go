package auth

import (
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
