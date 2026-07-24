package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

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

func (h *PartialHandler) ServerSelectorPartial(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.GetSessionFromContext(r); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	servers := h.config.Minecraft.Servers
	selected := r.URL.Query().Get("selected")
	if selected == "" && len(servers) > 0 {
		selected = servers[0].ID
	}

	statuses := make(map[string]bool, len(servers))
	for _, s := range servers {
		statuses[s.ID] = h.cache.Get(s.ID).Online
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ServerSelectorPartial(selected, servers, statuses).Render(r.Context(), w)
}

func (h *PartialHandler) CommandsPartial(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.GetSessionFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID := chi.URLParam(r, "id")
	templates, _ := h.store.GetTemplates(r.Context(), serverID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.CommandsPartial(session, serverID, templates).Render(r.Context(), w)
}

func (h *PartialHandler) CommandRunnerPartial(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.GetSessionFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID := chi.URLParam(r, "id")
	templateIDStr := chi.URLParam(r, "templateId")
	templateID, _ := strconv.ParseInt(templateIDStr, 10, 64)

	templates, _ := h.store.GetTemplates(r.Context(), serverID)
	var tmpl *store.StoredTemplate
	for i := range templates {
		if templates[i].ID == templateID {
			tmpl = &templates[i]
			break
		}
	}
	if tmpl == nil {
		http.Error(w, "template not found", http.StatusNotFound)
		return
	}

	// Parse params from template JSON
	var params []config.TemplateParam
	json.Unmarshal([]byte(tmpl.Params), &params)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.CommandRunnerPartial(session, serverID, tmpl, params).Render(r.Context(), w)
}

func (h *PartialHandler) StatusPartial(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	status := h.cache.Get(serverID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.StatusPartial(status.Online, status.PlayerCount, status.Error).Render(r.Context(), w)
}

func (h *PartialHandler) PlayersPartial(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	status := h.cache.Get(serverID)
	players := status.Players
	if players == nil {
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
	templates, _ := h.store.GetTemplates(r.Context(), serverID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	views.ManageCommandsPartial(session, serverID, templates).Render(r.Context(), w)
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
