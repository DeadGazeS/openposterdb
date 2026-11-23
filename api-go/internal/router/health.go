package router

import (
	"context"
	"net/http"
	"time"

	"openposterdb/internal/httpx"
)

// readyPingTimeout bounds the database check of /api/ready.
const readyPingTimeout = 2 * time.Second

// registerHealthRoutes wires the probes:
//   - /api/health: the process is up and serving (no dependencies checked) —
//     the container healthcheck / liveness probe;
//   - /api/ready: the server can take traffic — the database answers a ping
//     within readyPingTimeout, and it isn't shutting down (503 while
//     draining, so a load balancer stops sending requests).
func (r *Router) registerHealthRoutes() {
	r.mux.HandleFunc("/api/health", func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.mux.HandleFunc("/api/ready", func(w http.ResponseWriter, req *http.Request) {
		if r.state.Draining.Load() {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "draining"})
			return
		}
		ctx, cancel := context.WithTimeout(req.Context(), readyPingTimeout)
		defer cancel()
		if r.state.DB == nil || r.state.DB.PingContext(ctx) != nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "database unavailable"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
}
