package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/your-org/rconman/internal/auth"
	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/store"
)

// TemplateHandler handles CRUD for command templates.
type TemplateHandler struct {
	store store.Store
}

func NewTemplateHandler(st store.Store) *TemplateHandler {
	return &TemplateHandler{store: st}
}

type templateRequest struct {
	Category    string                  `json:"category"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Command     string                  `json:"command"`
	Params      []config.TemplateParam  `json:"params"`
}

func (h *TemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	serverID := chi.URLParam(r, "id")

	var req templateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Command == "" {
		writeJSONError(w, http.StatusBadRequest, "name and command are required")
		return
	}

	paramsJSON, _ := json.Marshal(req.Params)
	id, err := h.store.CreateTemplate(r.Context(), store.StoredTemplate{
		ServerID:    serverID,
		Category:    req.Category,
		Name:        req.Name,
		Description: req.Description,
		Command:     req.Command,
		Params:      string(paramsJSON),
	})
	if err != nil {
		slog.Error("failed to create template", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to create template")
		return
	}

	slog.Info("template created", "server", serverID, "id", id, "name", req.Name)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{"id": id})
}

func (h *TemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	idStr := chi.URLParam(r, "templateId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid template id")
		return
	}

	var req templateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Command == "" {
		writeJSONError(w, http.StatusBadRequest, "name and command are required")
		return
	}

	paramsJSON, _ := json.Marshal(req.Params)
	if err := h.store.UpdateTemplate(r.Context(), store.StoredTemplate{
		ID:          id,
		Category:    req.Category,
		Name:        req.Name,
		Description: req.Description,
		Command:     req.Command,
		Params:      string(paramsJSON),
	}); err != nil {
		slog.Error("failed to update template", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to update template")
		return
	}

	slog.Info("template updated", "id", id, "name", req.Name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": id})
}

func (h *TemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	idStr := chi.URLParam(r, "templateId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid template id")
		return
	}

	if err := h.store.DeleteTemplate(r.Context(), id); err != nil {
		slog.Error("failed to delete template", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to delete template")
		return
	}

	slog.Info("template deleted", "id", id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"id": id})
}

func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	templates, err := h.store.GetTemplates(r.Context(), serverID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to get templates")
		return
	}

	// Group by category like config.CommandCategory
	type templateResponse struct {
		ID          int64                   `json:"id"`
		Name        string                  `json:"name"`
		Description string                  `json:"description"`
		Command     string                  `json:"command"`
		Params      []config.TemplateParam  `json:"params"`
	}
	type categoryGroup struct {
		Category  string             `json:"category"`
		Templates []templateResponse `json:"templates"`
	}

	categories := []categoryGroup{}
	catMap := map[string]int{}
	for _, t := range templates {
		var params []config.TemplateParam
		json.Unmarshal([]byte(t.Params), &params)

		tr := templateResponse{
			ID:          t.ID,
			Name:        t.Name,
			Description: t.Description,
			Command:     t.Command,
			Params:      params,
		}

		idx, ok := catMap[t.Category]
		if !ok {
			catMap[t.Category] = len(categories)
			categories = append(categories, categoryGroup{
				Category:  t.Category,
				Templates: []templateResponse{tr},
			})
		} else {
			categories[idx].Templates = append(categories[idx].Templates, tr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(categories)
}

func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	session, ok := auth.GetSessionFromContext(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	if session.Role != "admin" {
		writeJSONError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
