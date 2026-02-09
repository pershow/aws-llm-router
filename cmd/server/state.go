package main

import (
	"context"
	"sort"
	"strings"
	"sync"

	"aws-cursor-router/internal/store"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
)

type foundationModelLister interface {
	ListFoundationModels(
		ctx context.Context,
		params *bedrock.ListFoundationModelsInput,
		optFns ...func(*bedrock.Options),
	) (*bedrock.ListFoundationModelsOutput, error)
}

type awsState struct {
	mu              sync.RWMutex
	cfg             store.AWSRuntimeConfig
	controlClient   foundationModelLister
	availableModels []string
}

type modelState struct {
	mu              sync.RWMutex
	enabledModelIDs []string
	enabledSet      map[string]struct{}
}

type adminTokenState struct {
	mu    sync.RWMutex
	token string
}

func (a *App) setAWSRuntimeState(cfg store.AWSRuntimeConfig, controlClient foundationModelLister, availableModels []string) {
	a.awsState.mu.Lock()
	a.awsState.cfg = cfg
	a.awsState.controlClient = controlClient
	a.awsState.availableModels = normalizeModelIDs(availableModels)
	a.awsState.mu.Unlock()
}

func (a *App) setAvailableModels(availableModels []string) {
	a.awsState.mu.Lock()
	a.awsState.availableModels = normalizeModelIDs(availableModels)
	a.awsState.mu.Unlock()
}

func (a *App) getAWSConfig() store.AWSRuntimeConfig {
	a.awsState.mu.RLock()
	cfg := a.awsState.cfg
	a.awsState.mu.RUnlock()
	return cfg
}

func (a *App) listAvailableModels() []string {
	a.awsState.mu.RLock()
	out := append([]string(nil), a.awsState.availableModels...)
	a.awsState.mu.RUnlock()
	return out
}

func (a *App) getControlClient() foundationModelLister {
	a.awsState.mu.RLock()
	client := a.awsState.controlClient
	a.awsState.mu.RUnlock()
	return client
}

func (s *modelState) Replace(modelIDs []string) {
	normalized := normalizeModelIDs(modelIDs)
	set := make(map[string]struct{}, len(normalized))
	for _, modelID := range normalized {
		set[modelID] = struct{}{}
	}

	s.mu.Lock()
	s.enabledModelIDs = normalized
	s.enabledSet = set
	s.mu.Unlock()
}

func (s *modelState) List() []string {
	s.mu.RLock()
	out := append([]string(nil), s.enabledModelIDs...)
	s.mu.RUnlock()
	return out
}

func (s *modelState) IsEnabled(modelID string) bool {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.enabledSet) == 0 {
		return true
	}
	_, ok := s.enabledSet[modelID]
	return ok
}

func (a *App) listEnabledModels() []string {
	return a.modelState.List()
}

func (a *App) isModelEnabled(modelID string) bool {
	return a.modelState.IsEnabled(modelID)
}

func (a *App) listCatalogModels() []string {
	enabled := a.listEnabledModels()
	if len(enabled) > 0 {
		return enabled
	}

	available := a.listAvailableModels()
	if len(available) > 0 {
		return available
	}

	awsCfg := a.getAWSConfig()
	fallback := pickDefaultModelID(a.cfg.DefaultModelID, awsCfg.DefaultModelID)
	if fallback == "" {
		return nil
	}
	return []string{fallback}
}

func (a *App) setAdminToken(adminToken string) {
	a.adminTokenState.mu.Lock()
	a.adminTokenState.token = strings.TrimSpace(adminToken)
	a.adminTokenState.mu.Unlock()
}

func (a *App) getAdminToken() string {
	a.adminTokenState.mu.RLock()
	token := a.adminTokenState.token
	a.adminTokenState.mu.RUnlock()
	return token
}

func (a *App) reloadAdminToken(ctx context.Context) error {
	adminToken, exists, err := a.store.GetAdminToken(ctx)
	if err != nil {
		return err
	}
	if !exists {
		adminToken = "admin123"
		if err := a.store.UpsertAdminToken(ctx, adminToken); err != nil {
			return err
		}
	}
	a.setAdminToken(adminToken)
	return nil
}

func normalizeModelIDs(modelIDs []string) []string {
	set := make(map[string]struct{}, len(modelIDs))
	out := make([]string, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		if _, exists := set[modelID]; exists {
			continue
		}
		set[modelID] = struct{}{}
		out = append(out, modelID)
	}
	sort.Strings(out)
	return out
}
