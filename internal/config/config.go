package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr            string
	RequestTimeout        time.Duration
	MaxBodyBytes          int64
	AWSRegion             string
	AWSAccessKeyID        string
	AWSSecretAccessKey    string
	AWSSessionToken       string
	DefaultModelID        string
	DefaultMaxOutputToken int32
	GlobalMaxConcurrent   int
	DBPath                string
	LogQueueSize          int
	MaxContentChars       int
}

type ClientConfig struct {
	ID                   string
	Name                 string
	APIKey               string
	MaxRequestsPerMinute int
	MaxConcurrent        int
	AllowedModels        []string
	Disabled             bool
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:            getEnv("LISTEN_ADDR", ":8080"),
		RequestTimeout:        time.Duration(getEnvInt("REQUEST_TIMEOUT_SECONDS", 120)) * time.Second,
		MaxBodyBytes:          int64(getEnvInt("MAX_BODY_BYTES", 0)),
		AWSRegion:             strings.TrimSpace(os.Getenv("AWS_REGION")),
		AWSAccessKeyID:        strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")),
		AWSSecretAccessKey:    strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY")),
		AWSSessionToken:       strings.TrimSpace(os.Getenv("AWS_SESSION_TOKEN")),
		DefaultModelID:        strings.TrimSpace(os.Getenv("DEFAULT_MODEL_ID")),
		DefaultMaxOutputToken: int32(getEnvInt("DEFAULT_MAX_OUTPUT_TOKENS", 0)),
		GlobalMaxConcurrent:   getEnvInt("GLOBAL_MAX_CONCURRENT", 512),
		DBPath:                getEnv("DB_PATH", "./data/router.db"),
		LogQueueSize:          getEnvInt("LOG_QUEUE_SIZE", 10000),
		MaxContentChars:       getEnvInt("MAX_CONTENT_CHARS", 20000),
	}

	if cfg.DefaultMaxOutputToken < 0 {
		return Config{}, errors.New("DEFAULT_MAX_OUTPUT_TOKENS must be >= 0")
	}
	if cfg.MaxBodyBytes < 0 {
		return Config{}, errors.New("MAX_BODY_BYTES must be >= 0")
	}
	if cfg.RequestTimeout <= 0 {
		return Config{}, errors.New("REQUEST_TIMEOUT_SECONDS must be > 0")
	}
	if cfg.LogQueueSize <= 0 {
		return Config{}, errors.New("LOG_QUEUE_SIZE must be > 0")
	}
	if cfg.MaxContentChars <= 0 {
		return Config{}, errors.New("MAX_CONTENT_CHARS must be > 0")
	}

	return cfg, nil
}

func getEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
