package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/your-org/rconman/internal/auth"
	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/rcon"
	"github.com/your-org/rconman/internal/store"
	"github.com/your-org/rconman/internal/views"
)

// CommandHandler handles RCON command execution.
type CommandHandler struct {
	rcons  map[string]rcon.Client
	store  store.Store
	config *config.Config
}

// NewCommandHandler creates a command handler.
func NewCommandHandler(
	rcons map[string]rcon.Client,
	s store.Store,
	cfg *config.Config,
) *CommandHandler {
	return &CommandHandler{
		rcons:  rcons,
		store:  s,
		config: cfg,
	}
}

// ExecuteRequest represents a command execution request.
type ExecuteRequest struct {
	Command string `json:"command"`
}

// ExecuteResponse represents the response from command execution.
type ExecuteResponse struct {
	Status     string `json:"status"`
	Response   string `json:"response,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// Execute handles command execution. Supports both JSON API and HTMX form data.
func (h *CommandHandler) Execute(w http.ResponseWriter, r *http.Request) {
	isHTMX := r.Header.Get("HX-Request") != ""

	serverID := chi.URLParam(r, "id")
	client, ok := h.rcons[serverID]
	if !ok {
		slog.Debug("execute request for unknown server", "server", serverID)
		if isHTMX {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			views.CommandResponsePartial("error", "", "unknown server", 0).Render(r.Context(), w)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ExecuteResponse{Status: "error", Error: "unknown server"})
		}
		return
	}

	session, ok := auth.GetSessionFromContext(r)
	if !ok {
		slog.Debug("execute request without valid session", "server", serverID)
		if isHTMX {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			views.CommandResponsePartial("error", "", "unauthorized", 0).Render(r.Context(), w)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(ExecuteResponse{Status: "error", Error: "unauthorized"})
		}
		return
	}

	// Parse command from request
	var command string
	if isHTMX {
		r.ParseForm()
		tmpl := r.FormValue("command_template")
		if tmpl != "" {
			command = tmpl
			for key, values := range r.Form {
				if key == "command_template" || len(values) == 0 {
					continue
				}
				command = strings.ReplaceAll(command, "{{"+key+"}}", values[0])
			}
		} else {
			command = r.FormValue("command")
		}
	} else {
		var req ExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			slog.Debug("execute request with invalid body", "server", serverID, "user", session.Email, "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ExecuteResponse{Status: "error", Error: "invalid request body"})
			return
		}
		command = req.Command
	}

	// Validate command
	if command == "" {
		slog.Debug("execute request with empty command", "server", serverID, "user", session.Email)
		if isHTMX {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			views.CommandResponsePartial("error", "", "command cannot be empty", 0).Render(r.Context(), w)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ExecuteResponse{Status: "error", Error: "command cannot be empty"})
		}
		return
	}

	if strings.Contains(command, "\x00") {
		slog.Debug("execute request with null bytes", "server", serverID, "user", session.Email)
		if isHTMX {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			views.CommandResponsePartial("error", "", "command contains null bytes", 0).Render(r.Context(), w)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ExecuteResponse{Status: "error", Error: "command contains null bytes"})
		}
		return
	}

	if len(command) > 4096 {
		slog.Debug("execute request exceeds size limit", "server", serverID, "user", session.Email, "len", len(command))
		if isHTMX {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			views.CommandResponsePartial("error", "", "command exceeds maximum length of 4096 bytes", 0).Render(r.Context(), w)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ExecuteResponse{Status: "error", Error: "command exceeds maximum length of 4096 bytes"})
		}
		return
	}

	slog.Debug("executing RCON command", "server", serverID, "user", session.Email, "command", command)

	// Execute command via RCON
	start := time.Now()
	response, err := client.Send(r.Context(), command)
	durationMS := time.Since(start).Milliseconds()

	if err != nil {
		response = ""
		slog.Info("command execution failed",
			"server", serverID, "user", session.Email, "command", command,
			"duration_ms", durationMS, "err", err)
	} else {
		slog.Info("command executed",
			"server", serverID, "user", session.Email, "command", command,
			"duration_ms", durationMS)
	}

	// Record in store
	if h.store != nil {
		if err := h.store.RecordCommand(r.Context(), session.Email, serverID, command, response, durationMS); err != nil {
			slog.Error("failed to record command", "server", serverID, "user", session.Email, "err", err)
		}
	}

	// Return response
	if isHTMX {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			views.CommandResponsePartial("error", "", err.Error(), durationMS).Render(r.Context(), w)
		} else {
			w.WriteHeader(http.StatusOK)
			views.CommandResponsePartial("executed", response, "", durationMS).Render(r.Context(), w)
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		execResp := ExecuteResponse{Status: "executed", Response: response, DurationMS: durationMS}
		if err != nil {
			execResp.Status = "error"
			execResp.Error = err.Error()
			execResp.Response = ""
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		json.NewEncoder(w).Encode(execResp)
	}
}

// LogsResponse represents the response from logs retrieval.
type LogsResponse struct {
	Logs  []store.CommandLog `json:"logs"`
	Error string             `json:"error,omitempty"`
}

// GetLogs returns recent command logs.
func (h *CommandHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	// Check if store is available
	if h.store == nil {
		slog.Error("get logs requested but store is not configured")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(LogsResponse{
			Logs:  []store.CommandLog{},
			Error: "store not configured",
		})
		return
	}

	if session, ok := auth.GetSessionFromContext(r); ok {
		slog.Debug("get logs requested", "user", session.Email)
	}

	logs, err := h.store.GetLogs(r.Context(), 50)
	if err != nil {
		slog.Error("failed to retrieve logs from store", "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(LogsResponse{
			Logs:  []store.CommandLog{},
			Error: "failed to get logs",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LogsResponse{
		Logs: logs,
	})
}

// authMiddleware is the interface AuthHandler requires from the auth middleware.
type authMiddleware interface {
	AuthCodeURL(w http.ResponseWriter, r *http.Request) string
	HandleCallback(w http.ResponseWriter, r *http.Request, code, state string) (string, string, error)
	CreateSession(w http.ResponseWriter, r *http.Request, email, role string) error
	ClearSession(w http.ResponseWriter, r *http.Request) error
}

// AuthHandler handles authentication flows.
type AuthHandler struct {
	config     *config.Config
	middleware authMiddleware
}

// NewAuthHandler creates an auth handler.
func NewAuthHandler(cfg *config.Config, m authMiddleware) *AuthHandler {
	return &AuthHandler{config: cfg, middleware: m}
}

// LoginRequest represents OAuth2 login request
type LoginRequest struct {
	Nonce string `json:"nonce"`
}

// Login redirects to OIDC provider.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") == "access_denied" {
		slog.Debug("login page: access denied error shown")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		views.LoginErrorPage("You are not authorised to access this application.").Render(r.Context(), w)
		return
	}

	// Generate authorization URL
	authURL := h.middleware.AuthCodeURL(w, r)
	if authURL == "" {
		slog.Error("failed to generate auth URL")
		http.Error(w, "failed to generate auth URL", http.StatusInternalServerError)
		return
	}

	slog.Debug("redirecting to OIDC provider for login")
	// Redirect to OIDC provider
	http.Redirect(w, r, authURL, http.StatusFound)
}

// CallbackResponse represents the callback response
type CallbackResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

// Callback handles OIDC callback.
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// Get authorization code from query params
	code := r.URL.Query().Get("code")
	if code == "" {
		slog.Debug("auth callback missing authorization code")
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	state := r.URL.Query().Get("state")
	slog.Debug("handling OIDC auth callback")

	// Exchange code for token
	email, role, err := h.middleware.HandleCallback(w, r, code, state)
	if err != nil {
		if errors.Is(err, auth.ErrLoginDenied) {
			slog.Debug("auth callback: login denied, redirecting")
			http.Redirect(w, r, "/auth/login?error=access_denied", http.StatusFound)
			return
		}
		slog.Error("auth callback failed", "err", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(CallbackResponse{
			Status: "error",
			Error:  "authentication failed",
		})
		return
	}

	// Create session
	if err := h.middleware.CreateSession(w, r, email, role); err != nil {
		slog.Error("failed to create session", "user", email, "err", err)
		http.Error(w, fmt.Sprintf("failed to create session: %v", err), http.StatusInternalServerError)
		return
	}

	slog.Info("user logged in", "user", email, "role", role)
	// Redirect to home page
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout clears session.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session, hasSession := auth.GetSessionFromContext(r)
	if err := h.middleware.ClearSession(w, r); err != nil {
		slog.Error("failed to clear session on logout", "err", err)
		http.Error(w, "failed to logout", http.StatusInternalServerError)
		return
	}

	if hasSession {
		slog.Info("user logged out", "user", session.Email)
	}
	// Optionally redirect to OIDC provider logout, or just home
	http.Redirect(w, r, "/", http.StatusFound)
}
