package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/your-org/rconman/internal/auth"
	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/rcon"
	"github.com/your-org/rconman/internal/store"
	"github.com/your-org/rconman/internal/views"
)

type PartialHandler struct {
	config *config.Config
	cache  *StatusCache
	store  store.Store
	rcons  map[string]rcon.Client
}

func NewPartialHandler(
	cfg *config.Config,
	cache *StatusCache,
	st store.Store,
	rcons map[string]rcon.Client,
) *PartialHandler {
	return &PartialHandler{config: cfg, cache: cache, store: st, rcons: rcons}
}

func (h *PartialHandler) findServer(serverID string) *config.ServerDef {
	for i := range h.config.Minecraft.Servers {
		if h.config.Minecraft.Servers[i].ID == serverID {
			return &h.config.Minecraft.Servers[i]
		}
	}
	return nil
}

func (h *PartialHandler) ServerPartial(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.GetSessionFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID := chi.URLParam(r, "id")
	server := h.findServer(serverID)
	if server == nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ServerPartial(session, *server, h.config.Lists).Render(r.Context(), w)
}

func (h *PartialHandler) CategoryPartial(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.GetSessionFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID := chi.URLParam(r, "id")
	server := h.findServer(serverID)
	if server == nil {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	catIndex, err := strconv.Atoi(chi.URLParam(r, "catIndex"))
	if err != nil || catIndex < 0 || catIndex >= len(server.Commands) {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.CategoryPartial(session, *server, catIndex, h.config.Lists).Render(r.Context(), w)
}

func (h *PartialHandler) StatusPartial(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	status := h.cache.Get(serverID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.StatusPartial(status.Online, status.PlayerCount, status.Error).Render(r.Context(), w)
}

func (h *PartialHandler) CustomCommandPartial(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.CustomCommandPartial(session, serverID).Render(r.Context(), w)
}

func (h *PartialHandler) PlayersPartial(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	client, ok := h.rcons[serverID]
	if !ok {
		http.Error(w, "unknown server", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	players, err := client.PlayerList(ctx)
	if err != nil {
		// Return empty list on error — the dropdown just won't have options
		players = []string{}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.PlayerOptionsPartial(players).Render(r.Context(), w)
}

func (h *PartialHandler) LogPartial(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		views.LogPartial([]store.CommandLog{}).Render(r.Context(), w)
		return
	}

	logs, err := h.store.GetLogs(r.Context(), 50)
	if err != nil {
		logs = []store.CommandLog{}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.LogPartial(logs).Render(r.Context(), w)
}

func (h *PartialHandler) PowerPartial(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")

	state, err := h.store.GetDesiredState(r.Context(), serverID)
	if err != nil {
		state = 1
	}

	desiredStr := "stopped"
	if state == 1 {
		desiredStr = "running"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.PowerPartial(serverID, desiredStr).Render(r.Context(), w)
}
