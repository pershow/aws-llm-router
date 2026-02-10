package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"aws-cursor-router/internal/config"
	"aws-cursor-router/internal/store"
)

type adminClientResponse struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	APIKey               string   `json:"api_key"`
	MaxRequestsPerMinute int      `json:"max_requests_per_minute"`
	MaxConcurrent        int      `json:"max_concurrent"`
	AllowedModels        []string `json:"allowed_models"`
	Disabled             bool     `json:"disabled"`
}

type adminConfigResponse struct {
	AWS               store.AWSRuntimeConfig  `json:"aws"`
	BedrockReady      bool                    `json:"bedrock_client_ready"`
	AvailableModels   []string                `json:"available_models"`
	EnabledModelIDs   []string                `json:"enabled_model_ids"`
	ModelPricing      []store.ModelPricingRow `json:"model_pricing"`
	PricingUnitTokens int                     `json:"pricing_unit_tokens"`
	Billing           store.BillingConfig     `json:"billing"`
	CurrentTotalCost  float64                 `json:"current_total_cost"`
	Clients           []adminClientResponse   `json:"clients"`
}

type adminModelPricingPayload struct {
	Items []store.ModelPricingRow `json:"items"`
}

type adminBillingPayload struct {
	GlobalCostLimitUSD float64 `json:"global_cost_limit_usd"`
}

type adminTokenPayload struct {
	AdminToken string `json:"admin_token"`
}

type adminUsageClientRow struct {
	ClientID     string  `json:"client_id"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	RequestCount int64   `json:"request_count"`
	CostAmount   float64 `json:"cost_amount"`
}

type adminUsageByModelRow struct {
	ClientID     string  `json:"client_id"`
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	RequestCount int64   `json:"request_count"`
	CostAmount   float64 `json:"cost_amount"`
}

type adminCallRow struct {
	store.CallLogRow
	CostAmount float64 `json:"cost_amount"`
}

func registerAdminRoutes(mux *http.ServeMux, app *App) {
	// 原始后台接口路径
	mux.HandleFunc(adminAPIPath("/config"), app.requireAdmin(app.handleAdminConfig))
	mux.HandleFunc(adminAPIPath("/config/aws"), app.requireAdmin(app.handleAdminAWSConfig))
	mux.HandleFunc(adminAPIPath("/config/models"), app.requireAdmin(app.handleAdminEnabledModels))
	mux.HandleFunc(adminAPIPath("/config/models/refresh"), app.requireAdmin(app.handleAdminRefreshModels))
	mux.HandleFunc(adminAPIPath("/config/model-pricing"), app.requireAdmin(app.handleAdminModelPricing))
	mux.HandleFunc(adminAPIPath("/config/admin-token"), app.requireAdmin(app.handleAdminTokenConfig))
	mux.HandleFunc(adminAPIPath("/config/billing"), app.requireAdmin(app.handleAdminBillingConfig))
	mux.HandleFunc(adminAPIPath("/config/clients"), app.requireAdmin(app.handleAdminClients))
	mux.HandleFunc(adminAPIPath("/usage"), app.requireAdmin(app.handleAdminUsage))
	mux.HandleFunc(adminAPIPath("/calls"), app.requireAdmin(app.handleAdminCalls))
}

func (a *App) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAdminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	payload, err := a.buildAdminConfigResponse(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (a *App) handleAdminAWSConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAdminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload adminAWSPayload
	if err := decodeJSONBody(w, r, a.cfg.MaxBodyBytes, &payload); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	awsCfg := store.AWSRuntimeConfig{
		Region:          strings.TrimSpace(payload.Region),
		AccessKeyID:     strings.TrimSpace(payload.AccessKeyID),
		SecretAccessKey: strings.TrimSpace(payload.SecretAccessKey),
		SessionToken:    strings.TrimSpace(payload.SessionToken),
		DefaultModelID:  strings.TrimSpace(payload.DefaultModelID),
	}
	if err := a.store.UpsertAWSConfig(r.Context(), awsCfg); err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.reloadAWSConfig(r.Context()); err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reloadEnabledModels(r.Context()); err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response, err := a.buildAdminConfigResponse(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleAdminEnabledModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAdminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload adminEnabledModelsPayload
	if err := decodeJSONBody(w, r, a.cfg.MaxBodyBytes, &payload); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	enabledModelIDs := normalizeModelIDs(payload.EnabledModelIDs)
	if len(enabledModelIDs) == 0 {
		writeAdminError(w, http.StatusBadRequest, "at least one model must be enabled")
		return
	}
	availableModels := a.listAvailableModels()
	if len(availableModels) > 0 {
		availableSet := make(map[string]struct{}, len(availableModels))
		for _, modelID := range availableModels {
			availableSet[modelID] = struct{}{}
		}
		for _, modelID := range enabledModelIDs {
			if _, exists := availableSet[modelID]; !exists {
				writeAdminError(w, http.StatusBadRequest, "model is not available in AWS list: "+modelID)
				return
			}
		}
	}

	if err := a.store.ReplaceEnabledModels(r.Context(), enabledModelIDs); err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.reloadEnabledModels(r.Context()); err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled_model_ids": a.listEnabledModels(),
	})
}

func (a *App) handleAdminRefreshModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAdminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	availableModels, err := a.refreshAvailableModels(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reloadEnabledModels(r.Context()); err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"available_models":  availableModels,
		"enabled_model_ids": a.listEnabledModels(),
	})
}

func (a *App) handleAdminModelPricing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAdminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload adminModelPricingPayload
	if err := decodeJSONBody(w, r, a.cfg.MaxBodyBytes, &payload); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	enabledModelIDs := a.listEnabledModels()
	enabledSet := make(map[string]struct{}, len(enabledModelIDs))
	for _, modelID := range enabledModelIDs {
		enabledSet[modelID] = struct{}{}
	}
	for _, item := range payload.Items {
		modelID := strings.TrimSpace(item.ModelID)
		if modelID == "" {
			continue
		}
		if _, ok := enabledSet[modelID]; !ok {
			writeAdminError(w, http.StatusBadRequest, fmt.Sprintf("model is not enabled: %s", modelID))
			return
		}
	}

	if err := a.store.ReplaceModelPricing(r.Context(), payload.Items); err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reloadBillingState(r.Context()); err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pricing, err := a.store.ListModelPricing(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": pricing})
}

func (a *App) handleAdminTokenConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAdminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload adminTokenPayload
	if err := decodeJSONBody(w, r, a.cfg.MaxBodyBytes, &payload); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	adminToken := strings.TrimSpace(payload.AdminToken)
	if adminToken == "" {
		writeAdminError(w, http.StatusBadRequest, "admin_token is required")
		return
	}

	if err := a.store.UpsertAdminToken(r.Context(), adminToken); err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reloadAdminToken(r.Context()); err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAdminBillingConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAdminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload adminBillingPayload
	if err := decodeJSONBody(w, r, a.cfg.MaxBodyBytes, &payload); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	cfg := store.BillingConfig{
		GlobalCostLimitUSD: payload.GlobalCostLimitUSD,
	}
	if err := a.store.UpsertBillingConfig(r.Context(), cfg); err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reloadBillingState(r.Context()); err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}

	billingCfg, totalCost := a.getBillingSnapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"billing":            billingCfg,
		"current_total_cost": roundCost(totalCost),
	})
}

func (a *App) handleAdminClients(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.handleAdminUpsertClient(w, r)
	case http.MethodDelete:
		a.handleAdminDeleteClient(w, r)
	default:
		writeAdminError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleAdminUpsertClient(w http.ResponseWriter, r *http.Request) {
	var payload adminClientPayload
	if err := decodeJSONBody(w, r, a.cfg.MaxBodyBytes, &payload); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	clientCfg := config.ClientConfig{
		ID:                   strings.TrimSpace(payload.ID),
		Name:                 strings.TrimSpace(payload.Name),
		APIKey:               strings.TrimSpace(payload.APIKey),
		MaxRequestsPerMinute: payload.MaxRequestsPerMinute,
		MaxConcurrent:        payload.MaxConcurrent,
		AllowedModels:        normalizeModelIDs(payload.AllowedModels),
		Disabled:             payload.Disabled,
	}
	if err := a.store.UpsertClient(r.Context(), clientCfg); err != nil {
		writeAdminError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.syncAuthFromStore(r.Context()); err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAdminDeleteClient(w http.ResponseWriter, r *http.Request) {
	clientID := strings.TrimSpace(r.URL.Query().Get("id"))
	if clientID == "" {
		writeAdminError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := a.store.DeleteClient(r.Context(), clientID); err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.syncAuthFromStore(r.Context()); err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleAdminUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAdminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now().UTC()
	from, err := parseDate(r.URL.Query().Get("from"), now.AddDate(0, 0, -7))
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid from date")
		return
	}
	to, err := parseDate(r.URL.Query().Get("to"), now)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid to date")
		return
	}
	clientID := strings.TrimSpace(r.URL.Query().Get("client_id"))

	byClient, err := a.store.GetUsage(r.Context(), from, to, clientID)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}
	byModel, err := a.store.GetUsageByModel(r.Context(), from, to, clientID)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}
	modelPricing, err := a.store.ListModelPricing(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}

	priceByModel := buildModelPricingMap(modelPricing)

	usageByModel := make([]adminUsageByModelRow, 0, len(byModel))
	costByClient := make(map[string]float64, len(byClient))
	totalCost := 0.0
	for _, row := range byModel {
		cost := calculateCostByTokens(row.Model, row.InputTokens, row.OutputTokens, priceByModel)
		costByClient[row.ClientID] += cost
		totalCost += cost
		usageByModel = append(usageByModel, adminUsageByModelRow{
			ClientID:     row.ClientID,
			Model:        row.Model,
			InputTokens:  row.InputTokens,
			OutputTokens: row.OutputTokens,
			TotalTokens:  row.TotalTokens,
			RequestCount: row.RequestCount,
			CostAmount:   roundCost(cost),
		})
	}

	usageByClient := make([]adminUsageClientRow, 0, len(byClient))
	for _, row := range byClient {
		usageByClient = append(usageByClient, adminUsageClientRow{
			ClientID:     row.ClientID,
			InputTokens:  row.InputTokens,
			OutputTokens: row.OutputTokens,
			TotalTokens:  row.TotalTokens,
			RequestCount: row.RequestCount,
			CostAmount:   roundCost(costByClient[row.ClientID]),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"by_client":       usageByClient,
		"by_client_model": usageByModel,
		"total_cost":      roundCost(totalCost),
	})
}

func (a *App) handleAdminCalls(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAdminError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"), 100, 500)
	page := parseLimit(r.URL.Query().Get("page"), 1, 1_000_000)
	clientID := strings.TrimSpace(r.URL.Query().Get("client_id"))

	totalCount, err := a.store.CountCalls(r.Context(), clientID)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}

	totalPages := 1
	if totalCount > 0 {
		totalPages = int((totalCount + int64(limit) - 1) / int64(limit))
	}
	if page > totalPages {
		page = totalPages
	}
	if page < 1 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, err := a.store.GetCalls(r.Context(), limit, offset, clientID)
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}
	modelPricing, err := a.store.ListModelPricing(r.Context())
	if err != nil {
		writeAdminError(w, http.StatusInternalServerError, err.Error())
		return
	}
	priceByModel := buildModelPricingMap(modelPricing)

	items := make([]adminCallRow, 0, len(rows))
	totalCost := 0.0
	for _, row := range rows {
		costModelID := strings.TrimSpace(row.BedrockModelID)
		if costModelID == "" {
			costModelID = strings.TrimSpace(row.Model)
		}
		cost := calculateCostByTokens(costModelID, int64(row.InputTokens), int64(row.OutputTokens), priceByModel)
		totalCost += cost
		items = append(items, adminCallRow{
			CallLogRow: row,
			CostAmount: roundCost(cost),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"total_cost":  roundCost(totalCost),
		"page":        page,
		"page_size":   limit,
		"total":       totalCount,
		"total_pages": totalPages,
		"has_prev":    page > 1,
		"has_next":    page < totalPages,
	})
}

func (a *App) buildAdminConfigResponse(ctx context.Context) (adminConfigResponse, error) {
	clients, err := a.store.ListClients(ctx)
	if err != nil {
		return adminConfigResponse{}, err
	}
	modelPricing, err := a.store.ListModelPricing(ctx)
	if err != nil {
		return adminConfigResponse{}, err
	}

	clientPayload := make([]adminClientResponse, 0, len(clients))
	for _, client := range clients {
		clientPayload = append(clientPayload, adminClientResponse{
			ID:                   client.ID,
			Name:                 client.Name,
			APIKey:               client.APIKey,
			MaxRequestsPerMinute: client.MaxRequestsPerMinute,
			MaxConcurrent:        client.MaxConcurrent,
			AllowedModels:        normalizeModelIDs(client.AllowedModels),
			Disabled:             client.Disabled,
		})
	}

	billingCfg, totalCost := a.getBillingSnapshot()

	return adminConfigResponse{
		AWS:               a.getAWSConfig(),
		BedrockReady:      a.proxy.HasClient(),
		AvailableModels:   a.listAvailableModels(),
		EnabledModelIDs:   a.listEnabledModels(),
		ModelPricing:      modelPricing,
		PricingUnitTokens: 1000,
		Billing:           billingCfg,
		CurrentTotalCost:  roundCost(totalCost),
		Clients:           clientPayload,
	}, nil
}

func (a *App) syncAuthFromStore(ctx context.Context) error {
	clients, err := a.store.ListClients(ctx)
	if err != nil {
		return err
	}
	return a.auth.ReplaceClients(clients)
}

func buildModelPricingMap(pricing []store.ModelPricingRow) map[string]store.ModelPricingRow {
	out := make(map[string]store.ModelPricingRow, len(pricing)*2)
	for _, item := range pricing {
		modelID := strings.TrimSpace(item.ModelID)
		if modelID == "" {
			continue
		}
		for _, key := range candidateModelPricingKeys(modelID) {
			out[key] = item
		}
	}
	return out
}

func calculateCostByTokens(modelID string, inputTokens, outputTokens int64, priceByModel map[string]store.ModelPricingRow) float64 {
	pricing, ok := priceByModel[strings.TrimSpace(modelID)]
	if !ok {
		return 0
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}

	const tokenBase = 1_000.0
	inputCost := (float64(inputTokens) / tokenBase) * pricing.InputPricePer1K
	outputCost := (float64(outputTokens) / tokenBase) * pricing.OutputPricePer1K
	return inputCost + outputCost
}

func candidateModelPricingKeys(modelID string) []string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}

	keys := map[string]struct{}{
		modelID: {},
	}
	if strings.HasPrefix(modelID, "us.") {
		trimmed := strings.TrimPrefix(modelID, "us.")
		if strings.TrimSpace(trimmed) != "" {
			keys[trimmed] = struct{}{}
		}
	} else {
		keys["us."+modelID] = struct{}{}
	}

	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	return out
}

func roundCost(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Round(value*1_000_000_000) / 1_000_000_000
}
