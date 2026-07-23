package handlers

import (
	"context"
	"encoding/json"
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

// storedCategories loads templates from the store and groups them by category.
func (h *PartialHandler) storedCategories(serverID string) []config.CommandCategory {
	if h.store == nil {
		return nil
	}
	templates, err := h.store.GetTemplates(context.Background(), serverID)
	if err != nil {
		return nil
	}

	catMap := map[string]*config.CommandCategory{}
	var categories []config.CommandCategory
	for _, t := range templates {
		var params []config.TemplateParam
		json.Unmarshal([]byte(t.Params), &params)

		tmpl := config.CommandTemplate{
			Name:        t.Name,
			Description: t.Description,
			Command:     t.Command,
			Params:      params,
		}

		cat, ok := catMap[t.Category]
		if !ok {
			categories = append(categories, config.CommandCategory{Category: t.Category})
			cat = &categories[len(categories)-1]
			catMap[t.Category] = cat
		}
		cat.Templates = append(cat.Templates, tmpl)
	}
	return categories
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

	// Override commands with stored templates
	server.Commands = h.storedCategories(serverID)

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

	// Override commands with stored templates
	server.Commands = h.storedCategories(serverID)

	catIndex, err := strconv.Atoi(chi.URLParam(r, "catIndex"))
	if err != nil || catIndex < 0 || catIndex >= len(server.Commands) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<p class="text-gray-400 py-4">No commands in this category.</p>`))
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

func (h *PartialHandler) ManageCommandsPartial(w http.ResponseWriter, r *http.Request) {
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
	categories := h.storedCategories(serverID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ManageCommandsPartial(serverID, categories).Render(r.Context(), w)
}

func (h *PartialHandler) TemplateFormPartial(w http.ResponseWriter, r *http.Request) {
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
	templateIDStr := chi.URLParam(r, "templateId")
	var template *store.StoredTemplate
	if templateIDStr != "" {
		templates, _ := h.store.GetTemplates(r.Context(), serverID)
		for i := range templates {
			if strconv.FormatInt(templates[i].ID, 10) == templateIDStr {
				template = &templates[i]
				break
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.TemplateFormPartial(serverID, template).Render(r.Context(), w)
}
