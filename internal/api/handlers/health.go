package handlers

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler provides the health check endpoint that reports the status
// of PostgreSQL and NATS connectivity.
type HealthHandler struct {
	pool    *pgxpool.Pool
	natsURL string
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(pool *pgxpool.Pool, natsURL string) *HealthHandler {
	return &HealthHandler{
		pool:    pool,
		natsURL: natsURL,
	}
}

// Handle returns the health status of the service including database and
// NATS connectivity. Always returns 200 — individual services report
// "connected" or "disconnected".
func (h *HealthHandler) Handle(w http.ResponseWriter, r *http.Request) {
	pgStatus := h.checkPostgres(r.Context())
	natsStatus := h.checkNATS()

	RespondJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"pg":     pgStatus,
		"nats":   natsStatus,
	})
}

// checkPostgres pings the database pool and returns a status string.
func (h *HealthHandler) checkPostgres(ctx context.Context) string {
	if h.pool == nil {
		return "disconnected"
	}

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := h.pool.Ping(pingCtx); err != nil {
		return "disconnected"
	}
	return "connected"
}

// checkNATS performs a TCP dial to the NATS URL to verify connectivity.
func (h *HealthHandler) checkNATS() string {
	if h.natsURL == "" {
		return "disconnected"
	}

	parsed, err := url.Parse(h.natsURL)
	if err != nil {
		return "disconnected"
	}

	host := parsed.Host
	if host == "" {
		return "disconnected"
	}

	// Ensure host has a port
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "4222")
	}

	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		return "disconnected"
	}
	conn.Close()
	return "connected"
}
