package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"aws-cursor-router/internal/auth"
	"aws-cursor-router/internal/bedrockproxy"
	"aws-cursor-router/internal/config"
	"aws-cursor-router/internal/store"
	"github.com/joho/godotenv"
)

//go:embed web/admin/*
var adminUIFiles embed.FS

type App struct {
	cfg    config.Config
	auth   *auth.Manager
	proxy  *bedrockproxy.Service
	store  *store.Store
	logger *log.Logger

	adminStatic http.Handler

	awsState        awsState
	modelState      modelState
	billingState    billingState
	adminTokenState adminTokenState
}

type adminClientPayload struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	APIKey               string   `json:"api_key"`
	MaxRequestsPerMinute int      `json:"max_requests_per_minute"`
	MaxConcurrent        int      `json:"max_concurrent"`
	AllowedModels        []string `json:"allowed_models"`
	Disabled             bool     `json:"disabled"`
}

type adminAWSPayload struct {
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
	DefaultModelID  string `json:"default_model_id"`
}

type adminEnabledModelsPayload struct {
	EnabledModelIDs []string `json:"enabled_model_ids"`
}

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds|log.LUTC)
	logger.Printf("starting aws cursor router on %s", cfg.ListenAddr)

	routerStore, err := store.New(cfg.DBPath, cfg.LogQueueSize)
	if err != nil {
		log.Fatalf("failed to initialize store: %v", err)
	}
	defer func() {
		if err := routerStore.Close(); err != nil {
			logger.Printf("store close error: %v", err)
		}
	}()

	if err := routerStore.SeedAWSConfigIfEmpty(context.Background(), store.AWSRuntimeConfig{
		Region:          cfg.AWSRegion,
		AccessKeyID:     cfg.AWSAccessKeyID,
		SecretAccessKey: cfg.AWSSecretAccessKey,
		SessionToken:    cfg.AWSSessionToken,
		DefaultModelID:  cfg.DefaultModelID,
	}); err != nil {
		log.Fatalf("failed to seed aws config: %v", err)
	}
	if err := routerStore.SeedAdminTokenIfEmpty(context.Background(), "admin123"); err != nil {
		log.Fatalf("failed to seed admin token: %v", err)
	}

	clients, err := routerStore.ListClients(context.Background())
	if err != nil {
		log.Fatalf("failed to load clients from store: %v", err)
	}

	authManager := auth.NewManager(cfg)
	if err := authManager.ReplaceClients(clients); err != nil {
		log.Fatalf("failed to initialize auth clients: %v", err)
	}

	proxy := bedrockproxy.NewService(
		nil,
		cfg.DefaultModelID,
		nil,
		cfg.DefaultMaxOutputToken,
	)

	adminSubFS, err := fs.Sub(adminUIFiles, "web/admin")
	if err != nil {
		log.Fatalf("failed to load admin ui files: %v", err)
	}

	app := &App{
		cfg:    cfg,
		auth:   authManager,
		proxy:  proxy,
		store:  routerStore,
		logger: logger,

		adminStatic: http.StripPrefix(adminStaticPath(), http.FileServer(http.FS(adminSubFS))),
	}

	if err := app.reloadAWSConfig(context.Background()); err != nil {
		logger.Printf("warning: failed to initialize bedrock clients: %v", err)
	}
	if err := app.reloadEnabledModels(context.Background()); err != nil {
		log.Fatalf("failed to initialize enabled models: %v", err)
	}
	if err := app.reloadBillingState(context.Background()); err != nil {
		log.Fatalf("failed to initialize billing state: %v", err)
	}
	if err := app.reloadAdminToken(context.Background()); err != nil {
		log.Fatalf("failed to initialize admin token: %v", err)
	}

	mux := http.NewServeMux()
	registerPublicRoutes(mux, app)
	registerAdminRoutes(mux, app)
	mux.Handle(adminStaticPath(), app.adminStatic)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           loggingMiddleware(logger, mux),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-signalCh:
		logger.Printf("received shutdown signal: %s", sig.String())
	case err := <-errCh:
		logger.Fatalf("server error: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("server shutdown error: %v", err)
	}
}

func pickDefaultModelID(fallback, preferred string) string {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		return preferred
	}
	return strings.TrimSpace(fallback)
}
