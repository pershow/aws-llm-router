package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"aws-cursor-router/internal/config"
	_ "modernc.org/sqlite"
)

type CallRecord struct {
	RequestID       string
	ClientID        string
	Model           string
	BedrockModelID  string
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	LatencyMs       int64
	StatusCode      int
	ErrorMessage    string
	RequestContent  string
	ResponseContent string
	IsStream        bool
	CreatedAt       time.Time
}

type UsageRow struct {
	ClientID     string `json:"client_id"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	RequestCount int64  `json:"request_count"`
}

type UsageByModelRow struct {
	ClientID     string `json:"client_id"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	RequestCount int64  `json:"request_count"`
}

type CallLogRow struct {
	RequestID       string `json:"request_id"`
	ClientID        string `json:"client_id"`
	Model           string `json:"model"`
	BedrockModelID  string `json:"bedrock_model_id"`
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	TotalTokens     int    `json:"total_tokens"`
	LatencyMs       int64  `json:"latency_ms"`
	StatusCode      int    `json:"status_code"`
	ErrorMessage    string `json:"error_message"`
	RequestContent  string `json:"request_content"`
	ResponseContent string `json:"response_content"`
	IsStream        bool   `json:"is_stream"`
	CreatedAt       string `json:"created_at"`
}

type AWSRuntimeConfig struct {
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
	DefaultModelID  string `json:"default_model_id"`
}

type ModelPricingRow struct {
	ModelID          string  `json:"model_id"`
	InputPricePer1K  float64 `json:"input_price_per_1k"`
	OutputPricePer1K float64 `json:"output_price_per_1k"`
}

type AdminAuthConfig struct {
	AdminToken string `json:"admin_token"`
}

type BillingConfig struct {
	GlobalCostLimitUSD float64 `json:"global_cost_limit_usd"`
}

type Store struct {
	db    *sql.DB
	queue chan CallRecord
	done  chan struct{}
	wg    sync.WaitGroup
}

func New(dbPath string, queueSize int) (*Store, error) {
	if queueSize <= 0 {
		queueSize = 10000
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dsn := fmt.Sprintf(
		"%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)",
		dbPath,
	)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)

	s := &Store{
		db:    db,
		queue: make(chan CallRecord, queueSize),
		done:  make(chan struct{}),
	}

	if err := s.ensureSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	s.wg.Add(1)
	go s.writeLoop()
	return s, nil
}

func (s *Store) Close() error {
	close(s.done)
	s.wg.Wait()
	return s.db.Close()
}

func (s *Store) Enqueue(record CallRecord) bool {
	select {
	case s.queue <- record:
		return true
	default:
		return false
	}
}

func (s *Store) SeedClientsIfEmpty(ctx context.Context, clients []config.ClientConfig) error {
	count, err := s.countRows(ctx, "admin_clients")
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for _, client := range clients {
		if err := s.UpsertClient(ctx, client); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SeedModelMappingsIfEmpty(ctx context.Context, mappings map[string]string) error {
	count, err := s.countRows(ctx, "admin_model_mappings")
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	for alias, modelID := range mappings {
		if err := s.UpsertModelMapping(ctx, alias, modelID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SeedAWSConfigIfEmpty(ctx context.Context, awsCfg AWSRuntimeConfig) error {
	count, err := s.countRows(ctx, "admin_aws_config")
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if strings.TrimSpace(awsCfg.Region) == "" {
		return nil
	}
	return s.UpsertAWSConfig(ctx, awsCfg)
}

func (s *Store) SeedEnabledModelsIfEmpty(ctx context.Context, modelIDs []string) error {
	count, err := s.countRows(ctx, "admin_enabled_models")
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return s.ReplaceEnabledModels(ctx, modelIDs)
}

func (s *Store) SeedAdminTokenIfEmpty(ctx context.Context, adminToken string) error {
	if strings.TrimSpace(adminToken) == "" {
		adminToken = "admin123"
	}
	count, err := s.countRows(ctx, "admin_auth_config")
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return s.UpsertAdminToken(ctx, adminToken)
}

func (s *Store) GetAWSConfig(ctx context.Context) (AWSRuntimeConfig, bool, error) {
	var cfg AWSRuntimeConfig
	row := s.db.QueryRowContext(ctx, `
SELECT region, access_key_id, secret_access_key, session_token, default_model_id
FROM admin_aws_config
WHERE id = 1
`)
	err := row.Scan(
		&cfg.Region,
		&cfg.AccessKeyID,
		&cfg.SecretAccessKey,
		&cfg.SessionToken,
		&cfg.DefaultModelID,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return AWSRuntimeConfig{}, false, nil
		}
		return AWSRuntimeConfig{}, false, err
	}
	normalizeAWSRuntimeConfig(&cfg)
	return cfg, true, nil
}

func (s *Store) UpsertAWSConfig(ctx context.Context, cfg AWSRuntimeConfig) error {
	normalizeAWSRuntimeConfig(&cfg)
	if cfg.Region == "" {
		return fmt.Errorf("region is required")
	}
	if (cfg.AccessKeyID == "") != (cfg.SecretAccessKey == "") {
		return fmt.Errorf("access_key_id and secret_access_key must be set together")
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO admin_aws_config(
id, region, access_key_id, secret_access_key, session_token, default_model_id, updated_at
) VALUES (1, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id)
DO UPDATE SET
region = excluded.region,
access_key_id = excluded.access_key_id,
secret_access_key = excluded.secret_access_key,
session_token = excluded.session_token,
default_model_id = excluded.default_model_id,
updated_at = excluded.updated_at
`,
		cfg.Region,
		cfg.AccessKeyID,
		cfg.SecretAccessKey,
		cfg.SessionToken,
		cfg.DefaultModelID,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) GetAdminToken(ctx context.Context) (string, bool, error) {
	var token string
	row := s.db.QueryRowContext(ctx, `
SELECT admin_token
FROM admin_auth_config
WHERE id = 1
`)
	if err := row.Scan(&token); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false, nil
	}
	return token, true, nil
}

func (s *Store) UpsertAdminToken(ctx context.Context, adminToken string) error {
	adminToken, err := normalizeAdminToken(adminToken)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO admin_auth_config(
id, admin_token, updated_at
) VALUES (1, ?, ?)
ON CONFLICT(id)
DO UPDATE SET
admin_token = excluded.admin_token,
updated_at = excluded.updated_at
`,
		adminToken,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) GetBillingConfig(ctx context.Context) (BillingConfig, bool, error) {
	var cfg BillingConfig
	row := s.db.QueryRowContext(ctx, `
SELECT global_cost_limit_usd
FROM admin_billing_config
WHERE id = 1
`)
	err := row.Scan(&cfg.GlobalCostLimitUSD)
	if err != nil {
		if err == sql.ErrNoRows {
			return BillingConfig{}, false, nil
		}
		return BillingConfig{}, false, err
	}
	if err := normalizeBillingConfig(&cfg); err != nil {
		return BillingConfig{}, false, err
	}
	return cfg, true, nil
}

func (s *Store) UpsertBillingConfig(ctx context.Context, cfg BillingConfig) error {
	if err := normalizeBillingConfig(&cfg); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO admin_billing_config(
id, global_cost_limit_usd, updated_at
) VALUES (1, ?, ?)
ON CONFLICT(id)
DO UPDATE SET
global_cost_limit_usd = excluded.global_cost_limit_usd,
updated_at = excluded.updated_at
`,
		cfg.GlobalCostLimitUSD,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) ListEnabledModels(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT model_id
FROM admin_enabled_models
ORDER BY model_id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]string, 0)
	for rows.Next() {
		var modelID string
		if err := rows.Scan(&modelID); err != nil {
			return nil, err
		}
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		result = append(result, modelID)
	}
	return result, rows.Err()
}

func (s *Store) ReplaceEnabledModels(ctx context.Context, modelIDs []string) error {
	modelIDs = uniqueNonEmpty(modelIDs)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_enabled_models`); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, modelID := range modelIDs {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO admin_enabled_models(model_id, updated_at)
VALUES (?, ?)
`, modelID, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ListModelPricing(ctx context.Context) ([]ModelPricingRow, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT model_id, input_price_per_1k, output_price_per_1k
FROM admin_model_pricing
ORDER BY model_id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]ModelPricingRow, 0)
	for rows.Next() {
		var row ModelPricingRow
		if err := rows.Scan(&row.ModelID, &row.InputPricePer1K, &row.OutputPricePer1K); err != nil {
			return nil, err
		}
		row.ModelID = strings.TrimSpace(row.ModelID)
		if row.ModelID == "" {
			continue
		}
		if row.InputPricePer1K < 0 {
			row.InputPricePer1K = 0
		}
		if row.OutputPricePer1K < 0 {
			row.OutputPricePer1K = 0
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) ReplaceModelPricing(ctx context.Context, pricing []ModelPricingRow) error {
	pricing, err := normalizeModelPricing(pricing)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_model_pricing`); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, item := range pricing {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO admin_model_pricing(model_id, input_price_per_1k, output_price_per_1k, updated_at)
VALUES (?, ?, ?, ?)
`, item.ModelID, item.InputPricePer1K, item.OutputPricePer1K, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ListClients(ctx context.Context) ([]config.ClientConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, api_key, max_requests_per_minute, max_concurrent, allowed_models_json, is_disabled
FROM admin_clients
ORDER BY id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]config.ClientConfig, 0)
	for rows.Next() {
		var (
			client            config.ClientConfig
			allowedModelsJSON string
			disabledFlag      int
		)
		if err := rows.Scan(
			&client.ID,
			&client.Name,
			&client.APIKey,
			&client.MaxRequestsPerMinute,
			&client.MaxConcurrent,
			&allowedModelsJSON,
			&disabledFlag,
		); err != nil {
			return nil, err
		}
		client.Disabled = disabledFlag == 1
		if strings.TrimSpace(allowedModelsJSON) != "" {
			_ = json.Unmarshal([]byte(allowedModelsJSON), &client.AllowedModels)
		}
		normalizeClientConfig(&client)
		result = append(result, client)
	}
	return result, rows.Err()
}

func (s *Store) UpsertClient(ctx context.Context, client config.ClientConfig) error {
	normalizeClientConfig(&client)
	if client.ID == "" {
		return fmt.Errorf("client id is required")
	}
	if client.APIKey == "" {
		return fmt.Errorf("client api key is required")
	}

	allowedModelsJSON := "[]"
	if len(client.AllowedModels) > 0 {
		payload, err := json.Marshal(client.AllowedModels)
		if err != nil {
			return err
		}
		allowedModelsJSON = string(payload)
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO admin_clients(
id, name, api_key, max_requests_per_minute, max_concurrent, allowed_models_json, is_disabled, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id)
DO UPDATE SET
name = excluded.name,
api_key = excluded.api_key,
max_requests_per_minute = excluded.max_requests_per_minute,
max_concurrent = excluded.max_concurrent,
allowed_models_json = excluded.allowed_models_json,
is_disabled = excluded.is_disabled,
updated_at = excluded.updated_at
`,
		client.ID,
		client.Name,
		client.APIKey,
		client.MaxRequestsPerMinute,
		client.MaxConcurrent,
		allowedModelsJSON,
		boolToInt(client.Disabled),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) DeleteClient(ctx context.Context, clientID string) error {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return fmt.Errorf("client_id is required")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_clients WHERE id = ?`, clientID)
	return err
}

func (s *Store) ListModelMappings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT alias, bedrock_model_id
FROM admin_model_mappings
ORDER BY alias ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]string{}
	for rows.Next() {
		var alias, modelID string
		if err := rows.Scan(&alias, &modelID); err != nil {
			return nil, err
		}
		alias = strings.ToLower(strings.TrimSpace(alias))
		modelID = strings.TrimSpace(modelID)
		if alias == "" || modelID == "" {
			continue
		}
		result[alias] = modelID
	}
	return result, rows.Err()
}

func (s *Store) UpsertModelMapping(ctx context.Context, alias, bedrockModelID string) error {
	alias = strings.ToLower(strings.TrimSpace(alias))
	bedrockModelID = strings.TrimSpace(bedrockModelID)
	if alias == "" {
		return fmt.Errorf("alias is required")
	}
	if bedrockModelID == "" {
		return fmt.Errorf("bedrock_model_id is required")
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO admin_model_mappings(alias, bedrock_model_id, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(alias)
DO UPDATE SET
bedrock_model_id = excluded.bedrock_model_id,
updated_at = excluded.updated_at
`,
		alias,
		bedrockModelID,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) DeleteModelMapping(ctx context.Context, alias string) error {
	alias = strings.ToLower(strings.TrimSpace(alias))
	if alias == "" {
		return fmt.Errorf("alias is required")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_model_mappings WHERE alias = ?`, alias)
	return err
}

func (s *Store) GetUsage(ctx context.Context, fromDate, toDate, clientID string) ([]UsageRow, error) {
	base := `
SELECT client_id, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), SUM(request_count)
FROM usage_daily
WHERE usage_date BETWEEN ? AND ?
`
	args := []any{fromDate, toDate}
	if clientID != "" {
		base += "AND client_id = ?\n"
		args = append(args, clientID)
	}
	base += "GROUP BY client_id ORDER BY SUM(total_tokens) DESC"

	rows, err := s.db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]UsageRow, 0)
	for rows.Next() {
		var row UsageRow
		if err := rows.Scan(
			&row.ClientID,
			&row.InputTokens,
			&row.OutputTokens,
			&row.TotalTokens,
			&row.RequestCount,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) GetUsageByModel(ctx context.Context, fromDate, toDate, clientID string) ([]UsageByModelRow, error) {
	base := `
SELECT client_id, model, SUM(input_tokens), SUM(output_tokens), SUM(total_tokens), SUM(request_count)
FROM usage_model_daily
WHERE usage_date BETWEEN ? AND ?
`
	args := []any{fromDate, toDate}
	if clientID != "" {
		base += "AND client_id = ?\n"
		args = append(args, clientID)
	}
	base += "GROUP BY client_id, model ORDER BY SUM(total_tokens) DESC"

	rows, err := s.db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]UsageByModelRow, 0)
	for rows.Next() {
		var row UsageByModelRow
		if err := rows.Scan(
			&row.ClientID,
			&row.Model,
			&row.InputTokens,
			&row.OutputTokens,
			&row.TotalTokens,
			&row.RequestCount,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) GetTotalCost(ctx context.Context) (float64, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(
(
CAST(umd.input_tokens AS REAL) * COALESCE(mp.input_price_per_1k, 0) +
CAST(umd.output_tokens AS REAL) * COALESCE(mp.output_price_per_1k, 0)
) / 1000.0
), 0)
FROM usage_model_daily AS umd
LEFT JOIN admin_model_pricing AS mp ON mp.model_id = umd.model
`)

	var totalCost float64
	if err := row.Scan(&totalCost); err != nil {
		return 0, err
	}
	if math.IsNaN(totalCost) || math.IsInf(totalCost, 0) || totalCost < 0 {
		return 0, nil
	}
	return totalCost, nil
}

func (s *Store) CountCalls(ctx context.Context, clientID string) (int64, error) {
	base := `SELECT COUNT(1) FROM call_logs`
	args := []any{}
	if clientID != "" {
		base += " WHERE client_id = ?"
		args = append(args, clientID)
	}

	var count int64
	if err := s.db.QueryRowContext(ctx, base, args...).Scan(&count); err != nil {
		return 0, err
	}
	if count < 0 {
		return 0, nil
	}
	return count, nil
}

func (s *Store) GetCalls(ctx context.Context, limit int, offset int, clientID string) ([]CallLogRow, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	base := `
SELECT
request_id, client_id, model, bedrock_model_id, input_tokens, output_tokens, total_tokens,
latency_ms, status_code, error_message, request_content, response_content, is_stream, created_at
FROM call_logs
`
	args := []any{}
	if clientID != "" {
		base += "WHERE client_id = ?\n"
		args = append(args, clientID)
	}
	base += "ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, limit)
	args = append(args, offset)

	rows, err := s.db.QueryContext(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]CallLogRow, 0)
	for rows.Next() {
		var row CallLogRow
		var streamFlag int
		if err := rows.Scan(
			&row.RequestID,
			&row.ClientID,
			&row.Model,
			&row.BedrockModelID,
			&row.InputTokens,
			&row.OutputTokens,
			&row.TotalTokens,
			&row.LatencyMs,
			&row.StatusCode,
			&row.ErrorMessage,
			&row.RequestContent,
			&row.ResponseContent,
			&streamFlag,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		row.IsStream = streamFlag == 1
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) writeLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.done:
			for {
				select {
				case record := <-s.queue:
					_ = s.insertRecord(record)
				default:
					return
				}
			}
		case record := <-s.queue:
			_ = s.insertRecord(record)
		}
	}
}

func (s *Store) insertRecord(record CallRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(record.Model) == "" {
		record.Model = "default"
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	streamFlag := 0
	if record.IsStream {
		streamFlag = 1
	}

	createdAt := record.CreatedAt.UTC().Format(time.RFC3339Nano)
	_, err = tx.Exec(`
INSERT INTO call_logs(
request_id, client_id, model, bedrock_model_id, input_tokens, output_tokens, total_tokens,
latency_ms, status_code, error_message, request_content, response_content, is_stream, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		record.RequestID,
		record.ClientID,
		record.Model,
		record.BedrockModelID,
		record.InputTokens,
		record.OutputTokens,
		record.TotalTokens,
		record.LatencyMs,
		record.StatusCode,
		record.ErrorMessage,
		record.RequestContent,
		record.ResponseContent,
		streamFlag,
		createdAt,
	)
	if err != nil {
		return err
	}

	usageDate := record.CreatedAt.UTC().Format("2006-01-02")
	_, err = tx.Exec(`
INSERT INTO usage_daily(
client_id, usage_date, input_tokens, output_tokens, total_tokens, request_count, last_seen_at
) VALUES (?, ?, ?, ?, ?, 1, ?)
ON CONFLICT(client_id, usage_date)
DO UPDATE SET
input_tokens = input_tokens + excluded.input_tokens,
output_tokens = output_tokens + excluded.output_tokens,
total_tokens = total_tokens + excluded.total_tokens,
request_count = request_count + 1,
last_seen_at = excluded.last_seen_at
`,
		record.ClientID,
		usageDate,
		record.InputTokens,
		record.OutputTokens,
		record.TotalTokens,
		createdAt,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
INSERT INTO usage_model_daily(
client_id, model, usage_date, input_tokens, output_tokens, total_tokens, request_count, last_seen_at
) VALUES (?, ?, ?, ?, ?, ?, 1, ?)
ON CONFLICT(client_id, model, usage_date)
DO UPDATE SET
input_tokens = input_tokens + excluded.input_tokens,
output_tokens = output_tokens + excluded.output_tokens,
total_tokens = total_tokens + excluded.total_tokens,
request_count = request_count + 1,
last_seen_at = excluded.last_seen_at
`,
		record.ClientID,
		record.Model,
		usageDate,
		record.InputTokens,
		record.OutputTokens,
		record.TotalTokens,
		createdAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ensureSchema(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS call_logs (
id INTEGER PRIMARY KEY AUTOINCREMENT,
request_id TEXT NOT NULL,
client_id TEXT NOT NULL,
model TEXT NOT NULL,
bedrock_model_id TEXT NOT NULL,
input_tokens INTEGER NOT NULL DEFAULT 0,
output_tokens INTEGER NOT NULL DEFAULT 0,
total_tokens INTEGER NOT NULL DEFAULT 0,
latency_ms INTEGER NOT NULL DEFAULT 0,
status_code INTEGER NOT NULL DEFAULT 0,
error_message TEXT NOT NULL DEFAULT '',
request_content TEXT NOT NULL DEFAULT '',
response_content TEXT NOT NULL DEFAULT '',
is_stream INTEGER NOT NULL DEFAULT 0,
created_at TEXT NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_call_logs_client_created
ON call_logs(client_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_call_logs_created
ON call_logs(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS usage_daily (
client_id TEXT NOT NULL,
usage_date TEXT NOT NULL,
input_tokens INTEGER NOT NULL DEFAULT 0,
output_tokens INTEGER NOT NULL DEFAULT 0,
total_tokens INTEGER NOT NULL DEFAULT 0,
request_count INTEGER NOT NULL DEFAULT 0,
last_seen_at TEXT NOT NULL,
PRIMARY KEY (client_id, usage_date)
)`,
		`CREATE TABLE IF NOT EXISTS usage_model_daily (
client_id TEXT NOT NULL,
model TEXT NOT NULL,
usage_date TEXT NOT NULL,
input_tokens INTEGER NOT NULL DEFAULT 0,
output_tokens INTEGER NOT NULL DEFAULT 0,
total_tokens INTEGER NOT NULL DEFAULT 0,
request_count INTEGER NOT NULL DEFAULT 0,
last_seen_at TEXT NOT NULL,
PRIMARY KEY (client_id, model, usage_date)
)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_model_daily_client_date
ON usage_model_daily(client_id, usage_date)`,
		`CREATE TABLE IF NOT EXISTS admin_clients (
id TEXT PRIMARY KEY,
name TEXT NOT NULL,
api_key TEXT NOT NULL UNIQUE,
max_requests_per_minute INTEGER NOT NULL,
max_concurrent INTEGER NOT NULL,
allowed_models_json TEXT NOT NULL DEFAULT '[]',
is_disabled INTEGER NOT NULL DEFAULT 0,
updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS admin_model_mappings (
alias TEXT PRIMARY KEY,
bedrock_model_id TEXT NOT NULL,
updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS admin_aws_config (
id INTEGER PRIMARY KEY CHECK (id = 1),
region TEXT NOT NULL,
access_key_id TEXT NOT NULL DEFAULT '',
secret_access_key TEXT NOT NULL DEFAULT '',
session_token TEXT NOT NULL DEFAULT '',
default_model_id TEXT NOT NULL DEFAULT '',
updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS admin_enabled_models (
model_id TEXT PRIMARY KEY,
updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS admin_model_pricing (
model_id TEXT PRIMARY KEY,
input_price_per_1k REAL NOT NULL DEFAULT 0,
output_price_per_1k REAL NOT NULL DEFAULT 0,
updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS admin_auth_config (
id INTEGER PRIMARY KEY CHECK (id = 1),
admin_token TEXT NOT NULL,
updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS admin_billing_config (
id INTEGER PRIMARY KEY CHECK (id = 1),
global_cost_limit_usd REAL NOT NULL DEFAULT 0,
updated_at TEXT NOT NULL
)`,
	}

	for _, query := range queries {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("ensure schema failed: %w", err)
		}
	}
	if err := s.migrateAdminClientColumns(ctx); err != nil {
		return err
	}
	if err := s.migrateModelPricingColumns(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrateAdminClientColumns(ctx context.Context) error {
	columns, err := s.tableColumns(ctx, "admin_clients")
	if err != nil {
		return err
	}

	if _, ok := columns["is_disabled"]; !ok {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE admin_clients ADD COLUMN is_disabled INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("migrate admin clients disabled column: %w", err)
		}
	}

	return nil
}

func (s *Store) migrateModelPricingColumns(ctx context.Context) error {
	columns, err := s.tableColumns(ctx, "admin_model_pricing")
	if err != nil {
		return err
	}

	_, hasInputPer1K := columns["input_price_per_1k"]
	_, hasOutputPer1K := columns["output_price_per_1k"]

	if !hasInputPer1K {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE admin_model_pricing ADD COLUMN input_price_per_1k REAL NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("migrate model pricing input column: %w", err)
		}
	}
	if !hasOutputPer1K {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE admin_model_pricing ADD COLUMN output_price_per_1k REAL NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("migrate model pricing output column: %w", err)
		}
	}

	_, hasInputPerMillion := columns["input_price_per_million"]
	if hasInputPerMillion {
		if _, err := s.db.ExecContext(ctx, `
UPDATE admin_model_pricing
SET input_price_per_1k =
CASE
WHEN input_price_per_1k <= 0 THEN input_price_per_million / 1000.0
ELSE input_price_per_1k
END
`); err != nil {
			return fmt.Errorf("migrate model pricing input values: %w", err)
		}
	}

	_, hasOutputPerMillion := columns["output_price_per_million"]
	if hasOutputPerMillion {
		if _, err := s.db.ExecContext(ctx, `
UPDATE admin_model_pricing
SET output_price_per_1k =
CASE
WHEN output_price_per_1k <= 0 THEN output_price_per_million / 1000.0
ELSE output_price_per_1k
END
`); err != nil {
			return fmt.Errorf("migrate model pricing output values: %w", err)
		}
	}

	return nil
}

func (s *Store) tableColumns(ctx context.Context, table string) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := map[string]struct{}{}
	for rows.Next() {
		var (
			cid        int
			name       string
			typ        string
			notNull    int
			defaultV   sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryKey); err != nil {
			return nil, err
		}
		name = strings.TrimSpace(strings.ToLower(name))
		if name == "" {
			continue
		}
		columns[name] = struct{}{}
	}
	return columns, rows.Err()
}

func (s *Store) countRows(ctx context.Context, table string) (int, error) {
	var count int
	row := s.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(1) FROM %s", table))
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func normalizeClientConfig(client *config.ClientConfig) {
	client.ID = strings.TrimSpace(client.ID)
	client.Name = strings.TrimSpace(client.Name)
	client.APIKey = strings.TrimSpace(client.APIKey)
	if client.Name == "" {
		client.Name = client.ID
	}
	if client.MaxRequestsPerMinute <= 0 {
		client.MaxRequestsPerMinute = 1200
	}
	if client.MaxConcurrent <= 0 {
		client.MaxConcurrent = 64
	}
	for i, model := range client.AllowedModels {
		client.AllowedModels[i] = strings.ToLower(strings.TrimSpace(model))
	}
	client.AllowedModels = uniqueNonEmpty(client.AllowedModels)
}

func normalizeAWSRuntimeConfig(cfg *AWSRuntimeConfig) {
	cfg.Region = strings.TrimSpace(cfg.Region)
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.SecretAccessKey = strings.TrimSpace(cfg.SecretAccessKey)
	cfg.SessionToken = strings.TrimSpace(cfg.SessionToken)
	cfg.DefaultModelID = strings.TrimSpace(cfg.DefaultModelID)
}

func normalizeBillingConfig(cfg *BillingConfig) error {
	if math.IsNaN(cfg.GlobalCostLimitUSD) || math.IsInf(cfg.GlobalCostLimitUSD, 0) {
		return fmt.Errorf("invalid global_cost_limit_usd")
	}
	if cfg.GlobalCostLimitUSD < 0 {
		return fmt.Errorf("global_cost_limit_usd must be >= 0")
	}
	return nil
}

func normalizeAdminToken(adminToken string) (string, error) {
	adminToken = strings.TrimSpace(adminToken)
	if adminToken == "" {
		return "", fmt.Errorf("admin_token is required")
	}
	return adminToken, nil
}

func uniqueNonEmpty(items []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := set[item]; ok {
			continue
		}
		set[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeModelPricing(items []ModelPricingRow) ([]ModelPricingRow, error) {
	byModel := map[string]ModelPricingRow{}
	for _, item := range items {
		modelID := strings.TrimSpace(item.ModelID)
		if modelID == "" {
			continue
		}

		inputPrice := item.InputPricePer1K
		outputPrice := item.OutputPricePer1K

		if math.IsNaN(inputPrice) || math.IsInf(inputPrice, 0) {
			return nil, fmt.Errorf("invalid input_price_per_1k for model %q", modelID)
		}
		if math.IsNaN(outputPrice) || math.IsInf(outputPrice, 0) {
			return nil, fmt.Errorf("invalid output_price_per_1k for model %q", modelID)
		}
		if inputPrice < 0 {
			return nil, fmt.Errorf("input_price_per_1k must be >= 0 for model %q", modelID)
		}
		if outputPrice < 0 {
			return nil, fmt.Errorf("output_price_per_1k must be >= 0 for model %q", modelID)
		}

		byModel[modelID] = ModelPricingRow{
			ModelID:          modelID,
			InputPricePer1K:  inputPrice,
			OutputPricePer1K: outputPrice,
		}
	}

	modelIDs := make([]string, 0, len(byModel))
	for modelID := range byModel {
		modelIDs = append(modelIDs, modelID)
	}
	sort.Strings(modelIDs)

	out := make([]ModelPricingRow, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		out = append(out, byModel[modelID])
	}
	return out, nil
}
