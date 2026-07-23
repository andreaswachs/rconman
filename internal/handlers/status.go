package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/your-org/rconman/internal/rcon"
)

// ServerStatus represents the cached status of a Minecraft server.
type ServerStatus struct {
	Online      bool      `json:"online"`
	PlayerCount int       `json:"player_count"`
	LastChecked time.Time `json:"last_checked"`
	Error       string    `json:"error,omitempty"`
}

// StatusCache holds per-server status updated by background pollers.
type StatusCache struct {
	mu       sync.RWMutex
	statuses map[string]*ServerStatus
}

func NewStatusCache(serverIDs []string) *StatusCache {
	c := &StatusCache{statuses: make(map[string]*ServerStatus, len(serverIDs))}
	for _, id := range serverIDs {
		c.statuses[id] = &ServerStatus{Online: false}
	}
	return c
}

func (c *StatusCache) Get(serverID string) *ServerStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if s, ok := c.statuses[serverID]; ok {
		cp := *s
		return &cp
	}
	return &ServerStatus{Online: false}
}

func (c *StatusCache) Set(serverID string, status *ServerStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statuses[serverID] = status
}

// StartPollers launches a background poller per server.
func (c *StatusCache) StartPollers(rcons map[string]rcon.Client, intervals map[string]time.Duration) {
	for id, client := range rcons {
		interval := intervals[id]
		if interval == 0 {
			interval = 30 * time.Second
		}
		go c.pollServer(id, client, interval)
	}
}

func (c *StatusCache) pollServer(id string, client rcon.Client, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	check := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		players, err := client.PlayerList(ctx)
		status := &ServerStatus{LastChecked: time.Now()}
		if err != nil {
			status.Online = false
			status.Error = err.Error()
			slog.Warn("status poll failed", "server", id, "err", err)
		} else {
			status.Online = true
			status.PlayerCount = len(players)
		}
		c.Set(id, status)
	}

	check()
	for range ticker.C {
		check()
	}
}

// UpdateStatusHandler updates the StatusHandler to use the cache.
type StatusHandler struct {
	rcons map[string]rcon.Client
	cache *StatusCache
}

func NewStatusHandler(rcons map[string]rcon.Client, cache *StatusCache) *StatusHandler {
	return &StatusHandler{rcons: rcons, cache: cache}
}

// GetStatus returns cached server status.
func (h *StatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.cache.Get(serverID))
}

// PlayersResponse represents the player list response.
type PlayersResponse struct {
	Players []string `json:"players"`
	Error   string   `json:"error,omitempty"`
}

// GetPlayers returns the live player list from RCON.
func (h *StatusHandler) GetPlayers(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "application/json")

	client, ok := h.rcons[serverID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(PlayersResponse{Error: "unknown server"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	players, err := client.PlayerList(ctx)
	if err != nil {
		slog.Debug("player list failed", "server", serverID, "err", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(PlayersResponse{Error: "server unavailable"})
		return
	}

	json.NewEncoder(w).Encode(PlayersResponse{Players: players})
}
