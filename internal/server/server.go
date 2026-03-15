package server

import (
	"context"
	"fmt"
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

	// Serve static files
	router.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

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

		statusHandler := handlers.NewStatusHandler(rcons)
		r.Route("/api/status", func(r chi.Router) {
			r.Get("/{id}", statusHandler.GetStatus)
		})

		// Home page with authenticated user
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			session, ok := auth.GetSessionFromContext(r)
			if !ok {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			views.HomePage(session, cfg.Minecraft.Servers).Render(r.Context(), w)
		})

		// Server detail page
		r.Get("/servers/{id}", func(w http.ResponseWriter, r *http.Request) {
			session, ok := auth.GetSessionFromContext(r)
			if !ok {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}

			serverID := chi.URLParam(r, "id")
			var server *config.ServerDef
			for i := range cfg.Minecraft.Servers {
				if cfg.Minecraft.Servers[i].ID == serverID {
					server = &cfg.Minecraft.Servers[i]
					break
				}
			}

			if server == nil {
				http.Error(w, "server not found", http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			views.ServerPage(session, *server).Render(r.Context(), w)
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
