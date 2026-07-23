package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/your-org/rconman/internal/auth"
	"github.com/your-org/rconman/internal/store"
)

// PowerHandler handles server power state toggling (desired state for KEDA).
type PowerHandler struct {
	store store.Store
}

func NewPowerHandler(st store.Store) *PowerHandler {
	return &PowerHandler{store: st}
}

type PowerRequest struct {
	State string `json:"state"` // "running" or "stopped"
}

type PowerResponse struct {
	ServerID     string `json:"server_id"`
	DesiredState string `json:"desired_state"`
}

// SetPower sets the desired state for a server (admin only).
func (h *PowerHandler) SetPower(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.GetSessionFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if session.Role != "admin" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	serverID := chi.URLParam(r, "id")

	isHTMX := r.Header.Get("HX-Request") != ""
	var req PowerRequest
	if isHTMX {
		r.ParseForm()
		req.State = r.FormValue("state")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
			return
		}
	}

	var state int
	switch req.State {
	case "running":
		state = 1
	case "stopped":
		state = 0
	default:
		if isHTMX {
			http.Error(w, "state must be 'running' or 'stopped'", http.StatusBadRequest)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "state must be 'running' or 'stopped'"})
		}
		return
	}

	if err := h.store.SetDesiredState(r.Context(), serverID, state); err != nil {
		slog.Error("failed to set desired state", "server", serverID, "err", err)
		if isHTMX {
			http.Error(w, "failed to set desired state", http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to set desired state"})
		}
		return
	}

	slog.Info("desired state set", "server", serverID, "state", req.State, "user", session.Email)

	if isHTMX {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		desiredStr := "stopped"
		if state == 1 {
			desiredStr = "running"
		}
		// Import views inline — we need to render the PowerPartial
		// But power.go doesn't import views. Use a simpler approach: redirect to the partial.
		// Actually, just return the partial HTML directly.
		powerPartialHTML(w, serverID, desiredStr)
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PowerResponse{
			ServerID:     serverID,
			DesiredState: req.State,
		})
	}
}

// powerPartialHTML writes the power toggle partial directly.
func powerPartialHTML(w http.ResponseWriter, serverID, desiredState string) {
	if desiredState == "running" {
		w.Write([]byte(`<div class="flex items-center gap-2">
			<span class="text-xs text-green-400 font-semibold">Desired: Running</span>
			<button hx-post="/api/servers/` + serverID + `/power" hx-vals='{"state": "stopped"}' hx-target="#power-toggle" hx-swap="innerHTML" class="btn btn-sm btn-outline btn-error">Stop Server</button>
		</div>`))
	} else {
		w.Write([]byte(`<div class="flex items-center gap-2">
			<span class="text-xs text-red-400 font-semibold">Desired: Stopped</span>
			<button hx-post="/api/servers/` + serverID + `/power" hx-vals='{"state": "running"}' hx-target="#power-toggle" hx-swap="innerHTML" class="btn btn-sm btn-outline btn-success">Start Server</button>
		</div>`))
	}
}

// GetPower returns the desired state for a server.
func (h *PowerHandler) GetPower(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")

	state, err := h.store.GetDesiredState(r.Context(), serverID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to get desired state"})
		return
	}

	desiredStr := "stopped"
	if state == 1 {
		desiredStr = "running"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(PowerResponse{
		ServerID:     serverID,
		DesiredState: desiredStr,
	})
}
