package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/your-org/rconman/internal/auth"
	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/rcon"
	"github.com/your-org/rconman/internal/server"
	"github.com/your-org/rconman/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup logging
	var logLevel slog.Level
	switch strings.ToLower(cfg.Log.Level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: logLevel}
	var logger *slog.Logger
	if cfg.Log.Format == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, opts))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	slog.SetDefault(logger)
	slog.Debug("log level configured", "level", logLevel.String())

	// Initialize store
	st, err := store.NewSQLiteStore(cfg.Store.Path)
	if err != nil {
		slog.Error("failed to initialize store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	// Start log pruning goroutine
	if cfg.Store.Retention != "" {
		retention, err := time.ParseDuration(cfg.Store.Retention)
		if err != nil {
			slog.Error("failed to parse store retention duration", "err", err)
			os.Exit(1)
		}
		go startLogPruner(st, retention)
	}

	// Seed desired states for configured servers (default: running)
	for _, srv := range cfg.Minecraft.Servers {
		current, err := st.GetDesiredState(context.Background(), srv.ID)
		if err != nil {
			slog.Warn("failed to get desired state, seeding default", "server", srv.ID, "err", err)
			if err := st.SetDesiredState(context.Background(), srv.ID, 1); err != nil {
				slog.Error("failed to seed desired state", "server", srv.ID, "err", err)
			}
			continue
		}
		if current == 0 {
			slog.Info("server desired state is stopped", "server", srv.ID)
		}
	}

	// Initialize RCON clients
	rcons := make(map[string]rcon.Client)
	for _, srv := range cfg.Minecraft.Servers {
		password, _ := srv.RCON.Password.Resolve()
		client, err := rcon.NewRealClient(srv.RCON.Host, srv.RCON.Port, password)
		if err != nil {
			slog.Error("failed to create RCON client", "server", srv.ID, "err", err)
			os.Exit(1)
		}
		rcons[srv.ID] = client
		slog.Info("initialized RCON client", "server", srv.ID, "host", srv.RCON.Host, "port", srv.RCON.Port)
	}

	// Setup auth
	sessionExpiry, _ := cfg.SessionExpiryDuration()
	clientID, err := cfg.Auth.OIDC.ClientID.Resolve()
	if err != nil {
		slog.Error("failed to resolve OIDC client ID", "err", err)
		os.Exit(1)
	}
	clientSecret, err := cfg.Auth.OIDC.ClientSecret.Resolve()
	if err != nil {
		slog.Error("failed to resolve OIDC client secret", "err", err)
		os.Exit(1)
	}
	sessionSecret, err := cfg.Server.SessionSecret.Resolve()
	if err != nil {
		slog.Error("failed to resolve session secret", "err", err)
		os.Exit(1)
	}
	authMiddleware, err := auth.NewMiddleware(
		context.Background(),
		cfg.Auth.OIDC.IssuerURL,
		clientID,
		clientSecret,
		cfg.Server.BaseURL,
		sessionSecret,
		sessionExpiry,
		&auth.RoleConfig{
			ClaimName:      cfg.Auth.Admin.Claim.Name,
			ClaimValue:     cfg.Auth.Admin.Claim.Value,
			EmailAllowlist: cfg.Auth.Admin.EmailAllowlist,
		},
		auth.AllowlistConfig{
			Emails:  cfg.Auth.Allowlist.Emails,
			Domains: cfg.Auth.Allowlist.Domains,
		},
		cfg.Server.InsecureMode,
	)
	if err != nil {
		slog.Error("failed to setup auth", "err", err)
		os.Exit(1)
	}

	// Create HTTP server
	httpServer, err := server.NewServer(cfg, rcons, st, authMiddleware)
	if err != nil {
		slog.Error("failed to create server", "err", err)
		os.Exit(1)
	}

	// Start server in goroutine
	go func() {
		slog.Info("starting HTTP server", "addr", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port))
		if err := httpServer.ListenAndServe(); err != nil {
			slog.Error("HTTP server error", "err", err)
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	slog.Info("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "err", err)
	}

	slog.Info("server stopped")
}

func startLogPruner(st store.Store, retention time.Duration) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	prune := func() {
		if err := st.PruneOlderThan(context.Background(), retention); err != nil {
			slog.Error("log pruning failed", "err", err)
		} else {
			slog.Debug("log pruning completed")
		}
	}

	// Prune shortly after startup (non-blocking)
	prune()

	for range ticker.C {
		prune()
	}
}
