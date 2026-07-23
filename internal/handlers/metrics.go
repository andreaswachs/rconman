package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/your-org/rconman/internal/config"
	"github.com/your-org/rconman/internal/store"
)

// MetricsHandler exposes Prometheus-format metrics for KEDA scaling.
type MetricsHandler struct {
	store  store.Store
	config *config.Config
}

func NewMetricsHandler(st store.Store, cfg *config.Config) *MetricsHandler {
	return &MetricsHandler{store: st, config: cfg}
}

// ServeHTTP renders Prometheus text-format metrics.
// Unauthenticated — intended for in-cluster scraping by KEDA/Prometheus.
func (h *MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	desiredStates, err := h.store.GetAllDesiredStates(r.Context())
	if err != nil {
		desiredStates = make(map[string]int)
	}

	var sb strings.Builder

	sb.WriteString("# HELP rconman_server_desired_state Desired state of a configured Minecraft server (1=running, 0=stopped)\n")
	sb.WriteString("# TYPE rconman_server_desired_state gauge\n")
	for _, srv := range h.config.Minecraft.Servers {
		state, ok := desiredStates[srv.ID]
		if !ok {
			state = 1 // default: running
		}
		sb.WriteString(fmt.Sprintf("rconman_server_desired_state{server_id=%q} %d\n", srv.ID, state))
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Write([]byte(sb.String()))
}

var _ http.Handler = (*MetricsHandler)(nil)
