package server

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/your-org/rconman/internal/auth"
	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/handlers"
	"github.com/your-org/rconman/internal/rcon"
	"github.com/your-org/rconman/internal/store"
	"github.com/your-org/rconman/internal/views"
	"github.com/your-org/rconman/web"
)

// Server wraps the HTTP server and router.
type Server struct {
	http *http.Server
}

// NewServer creates and configures the HTTP server.
func NewServer(
	cfg *config.Config,
	rcons map[string]rcon.Client,
	st store.Store,
	authMiddleware *auth.Middleware,
) (*Server, error) {
	router := chi.NewRouter()

	// Global middleware
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	// Health check
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Public auth routes
	authHandler := handlers.NewAuthHandler(cfg, authMiddleware)
	router.Route("/auth", func(r chi.Router) {
		r.Get("/login", authHandler.Login)
		r.Get("/callback", authHandler.Callback)
		r.Post("/logout", authHandler.Logout)
	})

	// Serve static files from embedded FS
	staticSub, _ := fs.Sub(web.StaticFS, "static")
	router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Prometheus metrics (unauthenticated — for KEDA/Prometheus scraping)
	metricsHandler := handlers.NewMetricsHandler(st, cfg)
	router.Get("/metrics", metricsHandler.ServeHTTP)

	// Create status cache and start background pollers
	serverIDs := make([]string, len(cfg.Minecraft.Servers))
	intervals := make(map[string]time.Duration)
	for i, srv := range cfg.Minecraft.Servers {
		serverIDs[i] = srv.ID
		interval, err := time.ParseDuration(srv.StatusPollInterval)
		if err != nil || interval == 0 {
			interval = 30 * time.Second
		}
		intervals[srv.ID] = interval
	}
	statusCache := handlers.NewStatusCache(serverIDs)
	statusCache.StartPollers(rcons, intervals)

	// Protected routes
	router.Group(func(r chi.Router) {
		r.Use(authMiddleware.RequireAuth)

		commandHandler := handlers.NewCommandHandler(rcons, st, cfg)
		r.Route("/api/commands", func(r chi.Router) {
			r.Post("/{id}", commandHandler.Execute)
		})

		r.Route("/api/logs", func(r chi.Router) {
			r.Get("/", commandHandler.GetLogs)
		})

		statusHandler := handlers.NewStatusHandler(rcons, statusCache)
		powerHandler := handlers.NewPowerHandler(st)
		templateHandler := handlers.NewTemplateHandler(st)
		r.Route("/api/servers", func(r chi.Router) {
			r.Get("/{id}/status", statusHandler.GetStatus)
			r.Get("/{id}/players", statusHandler.GetPlayers)
			r.Get("/{id}/power", powerHandler.GetPower)
			r.Post("/{id}/power", powerHandler.SetPower)
			r.Get("/{id}/templates", templateHandler.List)
		})
		r.Route("/api/templates", func(r chi.Router) {
			r.Post("/{id}", templateHandler.Create)
			r.Put("/{templateId}", templateHandler.Update)
			r.Delete("/{templateId}", templateHandler.Delete)
		})

		// HTMX partials
		partialHandler := handlers.NewPartialHandler(cfg, statusCache, st, rcons)
		r.Route("/partials", func(r chi.Router) {
			r.Get("/server/{id}", partialHandler.ServerPartial)
			r.Get("/server/{id}/commands", partialHandler.CommandsPartial)
			r.Get("/server/{id}/run/{templateId}", partialHandler.CommandRunnerPartial)
			r.Get("/server/{id}/status", partialHandler.StatusPartial)
			r.Get("/server/{id}/players", partialHandler.PlayersPartial)
			r.Get("/server/{id}/power", partialHandler.PowerPartial)
			r.Get("/server/{id}/manage", partialHandler.ManageCommandsPartial)
			r.Get("/server/{id}/template", partialHandler.TemplateFormPartial)
			r.Get("/server/{id}/template/{templateId}", partialHandler.TemplateFormPartial)
			r.Get("/logs", partialHandler.LogPartial)
		})

		// Home page
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			session, ok := auth.GetSessionFromContext(r)
			if !ok {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			views.HomePage(session, cfg.Minecraft.Servers).Render(r.Context(), w)
		})
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &Server{http: httpServer}, nil
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
