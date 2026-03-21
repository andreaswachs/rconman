package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/your-org/rconman/internal/auth"
	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/rcon"
	"github.com/your-org/rconman/internal/server"
	"github.com/your-org/rconman/internal/store"
)

// StaticFS would embed static assets: //go:embed ../../web/static
// Due to Go embed limitations, static files are served from disk in development
// and should be embedded during production builds
var StaticFS embed.FS

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup logging
	var logger *slog.Logger
	if cfg.Log.Format == "json" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	slog.SetDefault(logger)

	// Initialize store
	st, err := store.NewSQLiteStore(cfg.Store.Path)
	if err != nil {
		slog.Error("failed to initialize store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	// Initialize RCON clients (placeholder)
	rcons := make(map[string]rcon.Client)
	for _, srv := range cfg.Minecraft.Servers {
		password, _ := srv.RCON.Password.Resolve()
		// TODO: Initialize real RCON client
		slog.Info("initialized RCON client", "server", srv.ID)
		_ = password
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
