package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr        string
	ControlDSN      string
	AdminToken      string
	TokenPepper     string
	WebDir          string
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        envOr("HTTP_ADDR", ":8080"),
		ControlDSN:      strings.TrimSpace(os.Getenv("CONTROL_DSN")),
		AdminToken:      strings.TrimSpace(os.Getenv("ADMIN_TOKEN")),
		TokenPepper:     strings.TrimSpace(os.Getenv("TOKEN_PEPPER")),
		WebDir:          envOr("WEB_DIR", "web/dist"),
		ShutdownTimeout: 10 * time.Second,
	}
	if cfg.ControlDSN == "" {
		return Config{}, errors.New("CONTROL_DSN is required")
	}
	if len(cfg.AdminToken) < 20 {
		return Config{}, errors.New("ADMIN_TOKEN must contain at least 20 characters")
	}
	if len(cfg.TokenPepper) < 32 {
		return Config{}, errors.New("TOKEN_PEPPER must contain at least 32 characters")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
