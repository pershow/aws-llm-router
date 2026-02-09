package store

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"aws-cursor-router/internal/config"
)

func TestStoreConfigAndUsage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "router.db")
	s, err := New(dbPath, 100)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	if err := s.UpsertClient(ctx, config.ClientConfig{
		ID:                   "team-a",
		Name:                 "Team A",
		APIKey:               "key-a",
		MaxRequestsPerMinute: 100,
		MaxConcurrent:        10,
		AllowedModels:        []string{"gpt-4o"},
	}); err != nil {
		t.Fatalf("upsert client failed: %v", err)
	}
	if err := s.UpsertModelMapping(ctx, "gpt-4o", "anthropic.model"); err != nil {
		t.Fatalf("upsert model mapping failed: %v", err)
	}
	if err := s.ReplaceEnabledModels(ctx, []string{"anthropic.model", "anthropic.model", "   "}); err != nil {
		t.Fatalf("replace enabled models failed: %v", err)
	}
	enabledModels, err := s.ListEnabledModels(ctx)
	if err != nil {
		t.Fatalf("list enabled models failed: %v", err)
	}
	if len(enabledModels) != 1 || enabledModels[0] != "anthropic.model" {
		t.Fatalf("unexpected enabled models: %+v", enabledModels)
	}
	if err := s.UpsertAWSConfig(ctx, AWSRuntimeConfig{
		Region:          "us-east-1",
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
		SessionToken:    "st",
		DefaultModelID:  "anthropic.model",
	}); err != nil {
		t.Fatalf("upsert aws config failed: %v", err)
	}

	awsCfg, exists, err := s.GetAWSConfig(ctx)
	if err != nil {
		t.Fatalf("get aws config failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected aws config to exist")
	}
	if awsCfg.Region != "us-east-1" || awsCfg.AccessKeyID != "ak" || awsCfg.SecretAccessKey != "sk" {
		t.Fatalf("unexpected aws config: %+v", awsCfg)
	}

	billingCfg, billingExists, err := s.GetBillingConfig(ctx)
	if err != nil {
		t.Fatalf("get billing config failed: %v", err)
	}
	if billingExists {
		t.Fatalf("expected billing config to be empty, got: %+v", billingCfg)
	}

	if err := s.UpsertBillingConfig(ctx, BillingConfig{GlobalCostLimitUSD: 25.5}); err != nil {
		t.Fatalf("upsert billing config failed: %v", err)
	}
	billingCfg, billingExists, err = s.GetBillingConfig(ctx)
	if err != nil {
		t.Fatalf("get billing config after upsert failed: %v", err)
	}
	if !billingExists {
		t.Fatalf("expected billing config to exist")
	}
	if billingCfg.GlobalCostLimitUSD != 25.5 {
		t.Fatalf("unexpected billing config: %+v", billingCfg)
	}
	if err := s.UpsertBillingConfig(ctx, BillingConfig{GlobalCostLimitUSD: -1}); err == nil {
		t.Fatalf("expected negative billing limit to fail")
	}

	adminToken, exists, err := s.GetAdminToken(ctx)
	if err != nil {
		t.Fatalf("get admin token failed: %v", err)
	}
	if exists || adminToken != "" {
		t.Fatalf("expected empty admin token, got exists=%v token=%q", exists, adminToken)
	}
	if err := s.SeedAdminTokenIfEmpty(ctx, "admin123"); err != nil {
		t.Fatalf("seed admin token failed: %v", err)
	}
	adminToken, exists, err = s.GetAdminToken(ctx)
	if err != nil {
		t.Fatalf("get admin token after seed failed: %v", err)
	}
	if !exists || adminToken != "admin123" {
		t.Fatalf("unexpected admin token after seed: exists=%v token=%q", exists, adminToken)
	}
	if err := s.UpsertAdminToken(ctx, "new-admin-token"); err != nil {
		t.Fatalf("upsert admin token failed: %v", err)
	}
	adminToken, exists, err = s.GetAdminToken(ctx)
	if err != nil {
		t.Fatalf("get admin token after upsert failed: %v", err)
	}
	if !exists || adminToken != "new-admin-token" {
		t.Fatalf("unexpected admin token after upsert: exists=%v token=%q", exists, adminToken)
	}
	if err := s.UpsertAdminToken(ctx, "   "); err == nil {
		t.Fatalf("expected empty admin token to fail")
	}

	if err := s.insertRecord(CallRecord{
		RequestID:      "req-1",
		ClientID:       "team-a",
		Model:          "gpt-4o",
		BedrockModelID: "anthropic.model",
		InputTokens:    10,
		OutputTokens:   5,
		TotalTokens:    15,
		StatusCode:     200,
		CreatedAt:      time.Date(2026, 2, 9, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("insert record failed: %v", err)
	}

	usageRows, err := s.GetUsage(ctx, "2026-02-01", "2026-02-10", "")
	if err != nil {
		t.Fatalf("get usage failed: %v", err)
	}
	if len(usageRows) != 1 || usageRows[0].TotalTokens != 15 {
		t.Fatalf("unexpected usage rows: %+v", usageRows)
	}

	usageModelRows, err := s.GetUsageByModel(ctx, "2026-02-01", "2026-02-10", "")
	if err != nil {
		t.Fatalf("get usage by model failed: %v", err)
	}
	if len(usageModelRows) != 1 || usageModelRows[0].Model != "gpt-4o" {
		t.Fatalf("unexpected usage model rows: %+v", usageModelRows)
	}

	if err := s.ReplaceModelPricing(ctx, []ModelPricingRow{
		{ModelID: "gpt-4o", InputPricePer1K: 2.5, OutputPricePer1K: 10},
		{ModelID: "gpt-4o", InputPricePer1K: 3, OutputPricePer1K: 12},
		{ModelID: "  ", InputPricePer1K: 1, OutputPricePer1K: 1},
	}); err != nil {
		t.Fatalf("replace model pricing failed: %v", err)
	}

	pricingRows, err := s.ListModelPricing(ctx)
	if err != nil {
		t.Fatalf("list model pricing failed: %v", err)
	}
	if len(pricingRows) != 1 {
		t.Fatalf("expected one model pricing row, got: %+v", pricingRows)
	}
	if pricingRows[0].ModelID != "gpt-4o" || pricingRows[0].InputPricePer1K != 3 || pricingRows[0].OutputPricePer1K != 12 {
		t.Fatalf("unexpected model pricing row: %+v", pricingRows[0])
	}

	totalCost, err := s.GetTotalCost(ctx)
	if err != nil {
		t.Fatalf("get total cost failed: %v", err)
	}
	expectedTotalCost := 0.09 // (10*3 + 5*12) / 1_000
	if math.Abs(totalCost-expectedTotalCost) > 1e-12 {
		t.Fatalf("unexpected total cost: got=%f expected=%f", totalCost, expectedTotalCost)
	}
}

func TestStoreCostProratedForSubThousandTokens(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "router.db")
	s, err := New(dbPath, 100)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	if err := s.insertRecord(CallRecord{
		RequestID:      "req-sub-1k",
		ClientID:       "team-a",
		Model:          "anthropic.claude-3-5-sonnet",
		BedrockModelID: "anthropic.claude-3-5-sonnet",
		InputTokens:    1,
		OutputTokens:   0,
		TotalTokens:    1,
		StatusCode:     200,
		CreatedAt:      time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("insert record failed: %v", err)
	}

	if err := s.ReplaceModelPricing(ctx, []ModelPricingRow{
		{
			ModelID:          "anthropic.claude-3-5-sonnet",
			InputPricePer1K:  0.003,
			OutputPricePer1K: 0.015,
		},
	}); err != nil {
		t.Fatalf("replace model pricing failed: %v", err)
	}

	totalCost, err := s.GetTotalCost(ctx)
	if err != nil {
		t.Fatalf("get total cost failed: %v", err)
	}

	// 1 input token should still be prorated: 1/1000 * 0.003 = 0.000003
	expectedTotalCost := 0.000003
	if math.Abs(totalCost-expectedTotalCost) > 1e-12 {
		t.Fatalf("unexpected prorated total cost: got=%f expected=%f", totalCost, expectedTotalCost)
	}
}

func TestStoreCallsPaginationDescending(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "router.db")
	s, err := New(dbPath, 100)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	base := time.Date(2026, 2, 9, 13, 0, 0, 0, time.UTC)

	records := []CallRecord{
		{
			RequestID:      "req-1",
			ClientID:       "team-a",
			Model:          "model-a",
			BedrockModelID: "model-a",
			InputTokens:    1,
			OutputTokens:   1,
			TotalTokens:    2,
			StatusCode:     200,
			CreatedAt:      base.Add(1 * time.Minute),
		},
		{
			RequestID:      "req-2",
			ClientID:       "team-a",
			Model:          "model-a",
			BedrockModelID: "model-a",
			InputTokens:    2,
			OutputTokens:   2,
			TotalTokens:    4,
			StatusCode:     200,
			CreatedAt:      base.Add(2 * time.Minute),
		},
		{
			RequestID:      "req-3",
			ClientID:       "team-a",
			Model:          "model-a",
			BedrockModelID: "model-a",
			InputTokens:    3,
			OutputTokens:   3,
			TotalTokens:    6,
			StatusCode:     200,
			CreatedAt:      base.Add(3 * time.Minute),
		},
	}
	for _, record := range records {
		if err := s.insertRecord(record); err != nil {
			t.Fatalf("insert record failed: %v", err)
		}
	}

	total, err := s.CountCalls(ctx, "team-a")
	if err != nil {
		t.Fatalf("count calls failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("unexpected call count: %d", total)
	}

	page1, err := s.GetCalls(ctx, 2, 0, "team-a")
	if err != nil {
		t.Fatalf("get calls page1 failed: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("unexpected page1 size: %d", len(page1))
	}
	if page1[0].RequestID != "req-3" || page1[1].RequestID != "req-2" {
		t.Fatalf("unexpected page1 order: %+v", page1)
	}

	page2, err := s.GetCalls(ctx, 2, 2, "team-a")
	if err != nil {
		t.Fatalf("get calls page2 failed: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("unexpected page2 size: %d", len(page2))
	}
	if page2[0].RequestID != "req-1" {
		t.Fatalf("unexpected page2 order: %+v", page2)
	}
}
