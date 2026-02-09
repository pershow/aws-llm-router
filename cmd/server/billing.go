package main

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"aws-cursor-router/internal/store"
)

type billingState struct {
	mu           sync.RWMutex
	cfg          store.BillingConfig
	totalCost    float64
	priceByModel map[string]store.ModelPricingRow
}

func (a *App) reloadBillingState(ctx context.Context) error {
	cfg, exists, err := a.store.GetBillingConfig(ctx)
	if err != nil {
		return err
	}
	if !exists {
		cfg = store.BillingConfig{}
	}

	pricingRows, err := a.store.ListModelPricing(ctx)
	if err != nil {
		return err
	}
	priceByModel := buildModelPricingMap(pricingRows)

	usageRows, err := a.store.GetUsageByModel(ctx, "1970-01-01", "9999-12-31", "")
	if err != nil {
		return err
	}
	totalCost := 0.0
	for _, row := range usageRows {
		totalCost += calculateCostByTokens(row.Model, row.InputTokens, row.OutputTokens, priceByModel)
	}

	a.billingState.mu.Lock()
	a.billingState.cfg = cfg
	a.billingState.totalCost = totalCost
	a.billingState.priceByModel = priceByModel
	a.billingState.mu.Unlock()
	return nil
}

func (a *App) getBillingSnapshot() (store.BillingConfig, float64) {
	a.billingState.mu.RLock()
	cfg := a.billingState.cfg
	totalCost := a.billingState.totalCost
	a.billingState.mu.RUnlock()
	return cfg, totalCost
}

func (a *App) checkGlobalCostLimit() error {
	cfg, totalCost := a.getBillingSnapshot()
	limit := cfg.GlobalCostLimitUSD
	if limit <= 0 {
		return nil
	}
	if totalCost >= limit {
		return fmt.Errorf(
			"global cost limit exceeded: total=$%.6f, limit=$%.6f",
			roundCost(totalCost),
			roundCost(limit),
		)
	}
	return nil
}

func (a *App) addCostFromUsage(modelID string, inputTokens, outputTokens int64) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if inputTokens == 0 && outputTokens == 0 {
		return
	}

	a.billingState.mu.Lock()
	defer a.billingState.mu.Unlock()

	pricing, ok := a.billingState.priceByModel[modelID]
	if !ok {
		return
	}

	delta := ((float64(inputTokens) * pricing.InputPricePer1K) +
		(float64(outputTokens) * pricing.OutputPricePer1K)) / 1_000.0
	if math.IsNaN(delta) || math.IsInf(delta, 0) || delta <= 0 {
		return
	}

	a.billingState.totalCost += delta
}
