package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/yogel/db-access-gateway/internal/api"
	"github.com/yogel/db-access-gateway/internal/auth"
	"github.com/yogel/db-access-gateway/internal/config"
	"github.com/yogel/db-access-gateway/internal/control"
	queryservice "github.com/yogel/db-access-gateway/internal/query"
	"github.com/yogel/db-access-gateway/internal/target"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration invalid", "error", err)
		os.Exit(1)
	}
	store, err := control.Open(cfg.ControlDSN)
	if err != nil {
		logger.Error("control store open failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	startupCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := waitForControlStore(startupCtx, store); err != nil {
		logger.Error("control store unavailable", "error", err)
		os.Exit(1)
	}
	if err := store.Migrate(startupCtx); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	defaultAdminHash, err := auth.HashPassword("admin_123")
	if err != nil {
		logger.Error("default admin password hash failed", "error", err)
		os.Exit(1)
	}
	if err := store.EnsureDefaultAdmin(startupCtx, defaultAdminHash); err != nil {
		logger.Error("default admin initialization failed", "error", err)
		os.Exit(1)
	}
	registry := target.NewRegistry()
	defer registry.Close()
	querySvc := queryservice.NewService(store, registry)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := store.Ping(ctx); err != nil {
			http.Error(w, "not ready", 503)
			return
		}
		_, _ = w.Write([]byte("ready"))
	})
	webAuth := api.NewWebAuth(store, cfg.TokenPepper)
	webAuth.Register(mux)
	api.NewAdminHandler(store, registry, cfg.AdminToken, webAuth).Register(mux)
	api.NewUserHandler(store, webAuth, cfg.TokenPepper).Register(mux)
	mcpHandler := api.NewMCPHandler(store, querySvc, cfg.TokenPepper)
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)
	mux.Handle("/", spaHandler(cfg.WebDir))

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	logger.Info("gateway started", "addr", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func waitForControlStore(ctx context.Context, store *control.Store) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := store.Ping(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func spaHandler(root string) http.Handler {
	absolute, _ := filepath.Abs(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		path := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if path == "." {
			path = "index.html"
		}
		candidate := filepath.Join(absolute, path)
		if !strings.HasPrefix(candidate, absolute+string(os.PathSeparator)) && candidate != absolute {
			http.NotFound(w, r)
			return
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			http.ServeFile(w, r, candidate)
			return
		}
		index := filepath.Join(absolute, "index.html")
		if _, err := os.Stat(index); err != nil {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}
